import { api, apiBase } from '../core/api.js';
import { enhanceCustomSelects } from '../core/custom-select.js?v=2026062034';
import { countAnnotatedHighlights, renderAnnotatedAnswer, sanitizeAnnotatedAnswer } from '../core/annotated-answer.js';
import { renderMarkdown } from '../core/markdown.js';
import { renderRichText } from '../core/rich-text.js?v=2026070101';

const state = {
  classes: [],
  students: [],
  courses: [],
  selectedClassId: null,
  selectedCourseId: null,
  activeSection: null,
  student: null,
  classroom: null,
  sections: [],
  currentQuestions: [],
  submissionDetail: null,
  sectionDetailMap: new Map(),
  lastQuizResult: null,
  lastQuizResultKey: '',
  aiChatMessages: new Map(),
  aiChatPending: new Map(),
  retryingQuizSectionId: null,
  pollTimer: null,
  completedSectionIds: new Set()
};

const $ = (id) => document.getElementById(id);
const OTHER_OPTION_LABEL = '其它';
const AI_CHAT_GUIDE_TEXT = '请在下方对话框中与AI讨论本次任务的问题，请至少完成一轮对话。';

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// 轻量级提示：标注操作反馈
window.showStudentAlert = function (message, _type) {
  let toast = document.querySelector('.student-alert-toast');
  if (!toast) {
    toast = document.createElement('div');
    toast.className = 'student-alert-toast';
    toast.style.cssText = 'position:fixed;bottom:24px;left:50%;transform:translateX(-50%);padding:12px 24px;border-radius:8px;font-size:14px;z-index:9999;pointer-events:none;transition:opacity 0.3s;background:#fff3cd;color:#856404;border:1px solid #ffc107;box-shadow:0 2px 12px rgba(0,0,0,.15);';
    document.body.appendChild(toast);
  }
  toast.textContent = message;
  toast.style.opacity = '1';
  clearTimeout(toast._timer);
  toast._timer = setTimeout(() => {
    toast.style.opacity = '0';
  }, 2500);
};

function safeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function resolveImageUrl(url) {
  if (!url) return '';
  if (/^https?:\/\//i.test(url)) return url;
  if (url.startsWith('/api/')) {
    return `${window.location.protocol}//${window.location.hostname}:8080${url}`;
  }
  return url;
}

function currentCourse() {
  return state.courses.find((item) => item.id === state.selectedCourseId) || null;
}

function compareTimestampDesc(left, right) {
  const leftText = String(left || '');
  const rightText = String(right || '');
  if (leftText === rightText) return 0;
  return leftText > rightText ? -1 : 1;
}

function sectionIndexById(sectionId) {
  return state.sections.findIndex((item) => item.id === Number(sectionId));
}

function nextIncompleteSectionIndex() {
  return state.sections.findIndex((item) => !state.completedSectionIds.has(item.id));
}

function nextOpenedIncompleteSectionIndex(stageIndex) {
  if (stageIndex < 0) return -1;
  return state.sections.findIndex((item, index) => (
    index <= stageIndex && !state.completedSectionIds.has(item.id)
  ));
}

function getSectionDetail(sectionId) {
  return state.sectionDetailMap.get(Number(sectionId)) || null;
}

function latestQuizAttemptResult(sectionId) {
  const detail = getSectionDetail(sectionId);
  const attempts = detail?.attempts || [];
  const latestAttempt = attempts.length ? attempts[attempts.length - 1] : null;
  if (!latestAttempt) return null;
  const results = (latestAttempt.questions || []).map((question) => ({
    question: question.questionText || '',
    studentAnswer: question.answer || '',
    correctAnswer: question.correctAnswer || '-',
    explanation: question.explanation || '',
    isCorrect: !!question.isCorrect
  }));
  const total = (latestAttempt.questions || []).reduce((sum, question) => sum + Number(question.score || 0), 0);
  return {
    score: latestAttempt.score ?? results.filter((item) => item.isCorrect).length,
    total: total || results.length,
    results
  };
}

async function startQuizRetake(sectionId) {
  state.retryingQuizSectionId = Number(sectionId);
  await renderClassroom(state.student);
}

function sectionTag(section, index) {
  if (section.type === 'prediction') return '热身关卡';
  if (section.type === 'learning') return index === 0 ? '准备关卡' : '思考关卡';
  if (section.type === 'quiz') return '答题关卡';
  if (section.type === 'reflection') return '反思关卡';
  return '当前部分';
}

function renderPlainText(value) {
  return safeHtml(String(value ?? '')).replace(/\n/g, '<br>');
}

function normalizeAIChatTitle(value) {
  return String(value ?? '')
    .replace(/[（(]?\s*预留\s*[）)]?$/g, '')
    .trim() || 'AI 对话题';
}

function aiChatKey(questionId) {
  return String(questionId);
}

function normalizeChatMessages(messages = []) {
  return (Array.isArray(messages) ? messages : [])
    .map((item) => ({
      role: String(item?.role || '').trim(),
      content: String(item?.content || '').trim()
    }))
    .filter((item) => item.content && (item.role === 'user' || item.role === 'assistant'))
    .slice(0, 10);
}

function getStoredAIChatMessages(questionId) {
  return normalizeChatMessages(state.aiChatMessages.get(aiChatKey(questionId)) || []);
}

function setStoredAIChatMessages(questionId, messages) {
  state.aiChatMessages.set(aiChatKey(questionId), normalizeChatMessages(messages));
}

function countAIRounds(messages = []) {
  return normalizeChatMessages(messages).filter((item) => item.role === 'user').length;
}

function renderAIChatThinking() {
  return `
    <div class="ai-chat-message ai-chat-message--assistant ai-chat-message--thinking">
      <div class="ai-chat-message__role">AI 学习助手</div>
      <div class="ai-chat-message__content">
        <span class="ai-chat-thinking">
          思考中
          <span class="ai-chat-thinking__dots" aria-hidden="true">
            <i></i><i></i><i></i>
          </span>
        </span>
      </div>
    </div>
  `;
}

function renderAIChatMessages(messages = [], pending = false) {
  const normalized = normalizeChatMessages(messages);
  if (!normalized.length) {
    return `<div class="ai-chat-empty">${AI_CHAT_GUIDE_TEXT}</div>`;
  }
  const rendered = normalized.map((item) => `
    <div class="ai-chat-message ai-chat-message--${item.role === 'user' ? 'user' : 'assistant'}">
      <div class="ai-chat-message__role">${item.role === 'user' ? '我' : 'AI 学习助手'}</div>
      <div class="ai-chat-message__content">${renderMarkdown(item.content)}</div>
    </div>
  `).join('');
  return pending ? rendered + renderAIChatThinking() : rendered;
}

function renderAIChatTranscript(messages = [], pending = false) {
  const normalized = normalizeChatMessages(messages);
  if (!normalized.length) return '<div class="empty">暂无 AI 对话记录。</div>';
  return `<div class="ai-chat-transcript">${renderAIChatMessages(normalized, pending)}</div>`;
}

function latestQuestionChatMessages(questionId) {
  const stored = getStoredAIChatMessages(questionId);
  if (stored.length) return stored;
  for (const section of state.sectionDetailMap.values()) {
    const attempts = Array.isArray(section?.attempts) ? section.attempts : [];
    for (let ai = attempts.length - 1; ai >= 0; ai--) {
      const questions = Array.isArray(attempts[ai]?.questions) ? attempts[ai].questions : [];
      const matched = questions.find((question) => Number(question.questionId) === Number(questionId));
      if (matched?.chatMessages?.length) {
        return normalizeChatMessages(matched.chatMessages);
      }
    }
  }
  return [];
}

function normalizeBlackboard(value) {
  return String(value ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .map((line) => line.trim())
    .join('\n')
    .trim();
}

function renderBlackboardContent(value) {
  return safeHtml(normalizeBlackboard(value)).replace(/\n/g, '<br>');
}

function applyTipLayout(visible) {
  const blackboard = $('tipBlackboard');
  if (blackboard) {
    blackboard.style.display = visible ? 'block' : 'none';
  }
  document.body.style.paddingTop = visible
    ? (window.innerWidth <= 768 ? '110px' : '90px')
    : '0px';
}

function updateTopBlackboard(blackboard) {
  const content = normalizeBlackboard(blackboard);
  const tipContent = $('tipContent');
  if (tipContent) {
    tipContent.innerHTML = content ? renderBlackboardContent(content) : '';
  }
  applyTipLayout(!!content);
}

window.addEventListener('resize', () => {
  const blackboard = $('tipBlackboard');
  applyTipLayout(!!blackboard && blackboard.style.display !== 'none');
});

function renderCompletedQuestion(question, index) {
  if (question.questionType === 'ai_chat' || question.chatMessages?.length) {
    return `
      <div class="answer-item correct ai-chat-completed">
        <h4>${index + 1}. ${renderRichText(question.questionText || '')}</h4>
        <div class="meta">AI 对话记录</div>
        ${renderAIChatTranscript(question.chatMessages || [])}
      </div>
    `;
  }
  if (question.questionType === 'open_text') {
    return `
      <div class="answer-item correct answer-item--open-text">
        <h4>${index + 1}. ${renderRichText(question.questionText || '')}</h4>
        <div class="annotated-answer-block">
          <strong>你的作答</strong>
          <div class="annotated-answer-content">${renderAnnotatedAnswer(question.answer)}</div>
        </div>
        ${question.explanation ? `<div class="open-text-reference"><strong>参考解析</strong><div>${renderRichExplanationContent(question.explanation)}</div></div>` : ''}
      </div>
    `;
  }
  const isCorrect = !!question.isCorrect;
  const answerText = question.answer ? renderPlainText(question.answer) : '未填写';
  const correctText = question.correctAnswer ? renderPlainText(question.correctAnswer) : '-';
  return `
    <div class="answer-item ${isCorrect ? 'correct' : 'incorrect'}">
      <h4>
        ${index + 1}. ${renderRichText(question.questionText || '')}
        <span class="${isCorrect ? 'correct-mark' : 'incorrect-mark'}">${isCorrect ? '✅ 正确' : '❌ 错误'}</span>
      </h4>
      <p>你的答案：<strong class="${isCorrect ? 'student-answer-text--correct' : 'student-answer-text--wrong'}">${answerText}</strong></p>
      <p>正确答案：<strong class="student-answer-text--correct">${correctText}</strong></p>
      <p class="explanation"><span class="label">解析：</span><span class="content">${renderRichExplanationContent(question.explanation || '')}</span></p>
    </div>
  `;
}

function renderCompletedSection(section, index) {
  const detail = getSectionDetail(section.id);
  const attempts = detail?.attempts || [];
  const firstAttempt = attempts.length ? attempts[0] : null;
  const questions = firstAttempt?.questions || [];
  return `
    <section class="student-quiz-block student-stage-block student-stage-block--done">
      <div class="student-quiz-head">
        <span class="student-quiz-tag">已完成</span>
        <h3>${safeHtml(section.title || `第${index + 1}部分`)}</h3>
      </div>
      ${questions.length
        ? questions.map((question, questionIndex) => renderCompletedQuestion(question, questionIndex)).join('')
        : `
          <div class="waiting-message">
            <div class="emoji">✅</div>
            <p>该部分已完成，等待老师开启下一部分。</p>
          </div>
        `}
    </section>
  `;
}

function renderRetryableQuizSection(section, index) {
  return `
    <section class="student-quiz-block student-stage-block student-stage-block--active student-stage-block--retry">
      <div class="student-quiz-head">
        <span class="student-quiz-tag">可再次作答</span>
        <h3>${safeHtml(section.title || `第${index + 1}部分`)}</h3>
      </div>
      <div id="scoreCompare" class="student-complete-score-compare"></div>
      <div class="item">
        <strong>再答一次说明</strong>
        <div>你可以再次完成小测来复习巩固；系统只记录第一次提交的小测分数。</div>
      </div>
      <div id="quizRetryQuestions" class="list"></div>
      <div id="quizRetrySubmitWrap"></div>
    </section>
  `;
}

function renderWaitingSection(section, index) {
  return `
    <section class="student-quiz-block student-stage-block student-stage-block--waiting">
      <div class="student-quiz-head">
        <span class="student-quiz-tag">等待开启</span>
        <h3>${safeHtml(section.title || `第${index + 1}部分`)}</h3>
      </div>
      <div class="waiting-message">
        <div class="emoji">⏳</div>
        <p>请完成当前开放内容，等待老师开启下一部分。</p>
      </div>
    </section>
  `;
}

function renderPrepSection(student, blackboard) {
  const content = normalizeBlackboard(blackboard);
  return `
    <section class="student-quiz-block student-stage-block student-stage-block--waiting">
      <div class="student-quiz-head">
        <span class="student-quiz-tag">准备环节</span>
        <h3>请等待老师正式开启课堂</h3>
      </div>
      <p class="student-quiz-text">${safeHtml(student?.name || '')}，请先查看课堂提示语，等老师正式开启课堂后再开始答题。</p>
      <div class="student-prep-tip-card">
        <div class="student-prep-tip-head">
          <span class="student-quiz-tag">课堂提示</span>
          <h3>请等待老师正式开启课堂</h3>
        </div>
        <div class="student-prep-tip-content">${content ? renderBlackboardContent(content) : '老师还没有填写课堂提示语，请稍等。'}</div>
      </div>
    </section>
  `;
}

function quizResultStorageKey() {
  if (!state.selectedCourseId || !state.selectedClassId || !state.student) return '';
  return `v2_quiz_result_${state.selectedCourseId}_${state.selectedClassId}_${state.student.id}`;
}

function saveQuizResultCache(value) {
  const key = quizResultStorageKey();
  if (!key) return;
  state.lastQuizResultKey = key;
  window.sessionStorage.setItem(key, JSON.stringify(value || {}));
}

function loadQuizResultCache() {
  const key = quizResultStorageKey();
  if (!key) return null;
  state.lastQuizResultKey = key;
  const raw = window.sessionStorage.getItem(key);
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

async function loadPage() {
  $('apiBase').textContent = apiBase();
  const classData = await api('/classes');
  state.classes = classData.classes || [];
  renderClassSelector();
  await loadClassScopedData();
}

function renderClassSelector() {
  $('classSelect').innerHTML = state.classes.length
    ? state.classes.map((item) => `<option value="${item.id}">${safeHtml(item.name)}</option>`).join('')
    : '<option value="">暂无班级</option>';
  state.selectedClassId = Number($('classSelect').value || state.classes[0]?.id || 0);
}

async function loadClassScopedData() {
  stopPolling();
  updateTopBlackboard('');
  state.selectedClassId = Number($('classSelect').value || 0);
  state.student = null;
  state.students = [];
  state.courses = [];
  state.selectedCourseId = null;
  state.activeSection = null;
  state.classroom = null;
  state.sections = [];
  state.currentQuestions = [];
  state.lastQuizResult = null;
  state.lastQuizResultKey = '';
  state.aiChatMessages = new Map();
  state.completedSectionIds = new Set();
  $('studentNameInput').value = '';
  renderStudentNameList();
  if (!state.selectedClassId) return;

  const [studentData, courseData] = await Promise.all([
    api(`/classes/${state.selectedClassId}/students`),
    api(`/classes/${state.selectedClassId}/courses`)
  ]);
  state.students = studentData.students || [];
  state.courses = courseData.courses || [];
}

function renderStudentNameList() {
  const keyword = $('studentNameInput').value.trim();
  const combobox = $('studentNameCombobox');
  const list = $('studentNameList');
  if (!keyword) {
    list.innerHTML = '';
    combobox.classList.remove('is-open');
    return;
  }
  const matched = state.students
    .filter((item) => item.name.includes(keyword))
    .slice(0, 20);
  list.innerHTML = matched.map((item) => (
    `<button type="button" class="custom-select__option student-name-option" data-name="${safeHtml(item.name)}">
      <strong>${safeHtml(item.name)}</strong>
    </button>`
  )).join('');
  combobox.classList.toggle('is-open', matched.length > 0);
}

function closeStudentNameList() {
  $('studentNameCombobox')?.classList.remove('is-open');
}

function chooseStudentName(name) {
  $('studentNameInput').value = name;
  state.student = null;
  closeStudentNameList();
}

function validateStudent() {
  const name = $('studentNameInput').value.trim();
  if (!state.selectedClassId) {
    throw new Error('请先选择班级');
  }
  if (!name) {
    throw new Error('请输入姓名');
  }
  const student = state.students.find((item) => item.name === name);
  if (!student) {
    throw new Error('班级名单中没有找到该学生，请检查姓名或联系老师');
  }
  state.student = student;
  return student;
}

function resetStudentViewport() {
  if (document.activeElement && typeof document.activeElement.blur === 'function') {
    document.activeElement.blur();
  }
  window.scrollTo({ top: 0, behavior: 'auto' });
  if (document.scrollingElement) {
    document.scrollingElement.scrollTop = 0;
  }
  window.requestAnimationFrame(() => {
    window.scrollTo({ top: 0, behavior: 'auto' });
    if (document.scrollingElement) {
      document.scrollingElement.scrollTop = 0;
    }
  });
}

async function pickActiveCourse() {
  if (!state.courses.length) return null;
  const classroomList = await Promise.all(state.courses.map(async (course) => {
    const data = await api(`/classrooms/${course.id}/${state.selectedClassId}`);
    return { course, classroom: data.classroom || {} };
  }));
  return classroomList.sort((left, right) => {
    const timeCompare = compareTimestampDesc(left.classroom.updatedAt, right.classroom.updatedAt);
    if (timeCompare !== 0) return timeCompare;
    const stageCompare = Number(right.classroom.stageId || 0) - Number(left.classroom.stageId || 0);
    if (stageCompare !== 0) return stageCompare;
    return Number(right.course.id || 0) - Number(left.course.id || 0);
  })[0] || classroomList[0];
}

async function loadSelectedCourseContext() {
  const [classroomData, sectionData] = await Promise.all([
    api(`/classrooms/${state.selectedCourseId}/${state.selectedClassId}`),
    api(`/courses/${state.selectedCourseId}/sections`)
  ]);
  state.classroom = classroomData.classroom;
  state.sections = sectionData.sections || [];
  await syncCompletedSectionsFromHistory();
}

async function syncCompletedSectionsFromHistory() {
  state.completedSectionIds = new Set();
  state.submissionDetail = null;
  state.sectionDetailMap = new Map();
  if (!state.selectedCourseId || !state.selectedClassId || !state.student) return;
  try {
    const data = await api(`/student-history?classId=${state.selectedClassId}&studentId=${state.student.id}`);
    const record = (data.records || []).find((item) => Number(item.courseId) === Number(state.selectedCourseId));
    if (!record) return;
    const completedParts = Math.max(0, Number(record.completedParts || 0));
    if (record.submissionId) {
      try {
        const detail = await api(`/stats/student-detail?submissionId=${record.submissionId}`);
        state.submissionDetail = detail;
        state.sectionDetailMap = new Map((detail.sections || []).map((section) => [Number(section.sectionId), section]));
        const completedIds = (detail.sections || [])
          .filter((section) => section.completed)
          .map((section) => Number(section.sectionId));
        if (completedIds.length) {
          state.completedSectionIds = new Set(completedIds);
          return;
        }
      } catch (detailError) {
        console.warn('加载学生详情失败', detailError);
      }
    }
    if (record.status === 'completed' && state.sections.length > 0) {
      state.completedSectionIds = new Set(state.sections.map((item) => item.id));
      return;
    }
    state.completedSectionIds = new Set(state.sections.slice(0, completedParts).map((item) => item.id));
  } catch (error) {
    console.warn('加载学生进度失败', error);
  }
}

async function enterClassroom() {
  clearLoginMessage();
  let student;
  try {
    student = validateStudent();
  } catch (error) {
    showLoginError(error.message);
    return;
  }
  if (!state.courses.length) {
    showLoginError('该班级还没有绑定课堂，请老师在教师端班级管理中绑定。');
    return;
  }

  try {
    const selected = await pickActiveCourse();
    state.selectedCourseId = selected?.course?.id || state.courses[0].id;
    await loadSelectedCourseContext();
    resetStudentViewport();
    $('loginPage').classList.add('hidden');
    $('surveyPage').classList.remove('hidden');
    await renderClassroom(student);
    resetStudentViewport();
    startPolling();
  } catch (error) {
    showLoginError(error.message);
  }
}

function showLoginError(message) {
  const loginMessage = $('loginMessage');
  if (loginMessage && !$('loginPage').classList.contains('hidden')) {
    loginMessage.textContent = message;
    loginMessage.classList.add('login-message--error');
    return;
  }
  $('classroom').innerHTML = `<div class="panel">${safeHtml(message)}</div>`;
}

function clearLoginMessage() {
  const loginMessage = $('loginMessage');
  if (!loginMessage) return;
  loginMessage.textContent = '';
  loginMessage.classList.remove('login-message--error');
}

function startPolling() {
  stopPolling();
  state.pollTimer = window.setInterval(refreshClassroomState, 5000);
}

function stopPolling() {
  if (state.pollTimer) {
    window.clearInterval(state.pollTimer);
    state.pollTimer = null;
  }
}

async function refreshClassroomState(options = {}) {
  if (!state.selectedCourseId || !state.selectedClassId || !state.student) return;
  try {
    const previousCourseId = state.selectedCourseId;
    const previousStage = Number(state.classroom?.stageId || 0);
    const previousUpdatedAt = state.classroom?.updatedAt || '';
    const preferred = await pickActiveCourse();
    if (preferred?.course?.id && preferred.course.id !== state.selectedCourseId) {
      state.selectedCourseId = preferred.course.id;
    }
    await loadSelectedCourseContext();
    const nextStage = Number(state.classroom?.stageId || 0);
    const nextUpdatedAt = state.classroom?.updatedAt || '';
    const courseChanged = previousCourseId !== state.selectedCourseId;
    if (options.forceRender || courseChanged || previousStage !== nextStage || previousUpdatedAt !== nextUpdatedAt) {
      await renderClassroom(state.student);
    } else {
      renderProgress();
    }
  } catch (error) {
    console.warn('刷新课堂状态失败', error);
  }
}

async function renderClassroom(student) {
  const blackboard = normalizeBlackboard(state.classroom?.blackboard);
  const stageID = Number(state.classroom?.stageId || 0);
  // 准备环节在页面主体内展示黑板，stage > 0 后切换到顶部固定栏
  updateTopBlackboard(stageID > 0 ? blackboard : '');
  const allCompleted = state.sections.length > 0
    && state.sections.every((item) => state.completedSectionIds.has(item.id));
  if (allCompleted) {
    await renderCompletionState(student);
    return;
  }
  const stageIndex = sectionIndexById(stageID);
  const nextIndex = nextIncompleteSectionIndex();
  const activeIndex = nextOpenedIncompleteSectionIndex(stageIndex);
  const hasStarted = stageID > 0;
  state.activeSection = activeIndex >= 0 ? state.sections[activeIndex] || null : null;
  const course = currentCourse();
  renderProgress();
  if (stageID === 0 && nextIndex === 0 && state.completedSectionIds.size === 0) {
    $('classroom').innerHTML = `
      <div class="panel student-stage-banner">
        <h2>${safeHtml(course?.title || '当前课堂')}</h2>
        <p class="meta">${safeHtml(student?.name || '')}，请先查看课堂提示，等待老师正式开启课堂。</p>
      </div>
      ${renderPrepSection(student, blackboard)}
    `;
    return;
  }
  const visibleEndIndex = hasStarted ? stageIndex : nextIndex - 1;
  const visibleSections = visibleEndIndex >= 0 ? state.sections.slice(0, visibleEndIndex + 1) : [];
  let retryableQuizIndex = -1;
  const reflectionCourse = course?.mode === 'reflection' || course?.templateCode === 'reflection';
  if (reflectionCourse) {
    retryableQuizIndex = state.sections.findIndex((section, index) => (
      index <= stageIndex && section.type === 'quiz' && state.completedSectionIds.has(section.id)
    ));
  }
  const sectionHtml = await Promise.all(visibleSections.map(async (section, index) => {
    if (index === retryableQuizIndex) {
      return renderRetryableQuizSection(section, index);
    }
    if (state.completedSectionIds.has(section.id)) {
      return renderCompletedSection(section, index);
    }
    if (index === activeIndex) {
      const data = await api(`/sections/${section.id}/questions`);
      const questions = (data.questions || []).filter((item) => item.enabled);
      state.currentQuestions = questions;
      const questionHtml = questions.length ? questions.map(renderQuestion).join('') : '<div class="empty">当前部分还没有题目。</div>';
      return `
        <section class="student-quiz-block student-stage-block student-stage-block--active">
          <div class="student-quiz-head">
            <span class="student-quiz-tag">${safeHtml(sectionTag(section, index))}</span>
            <h3>${safeHtml(section.title || `第${index + 1}部分`)}</h3>
          </div>
          <div id="scoreCompare"></div>
          <div id="questions" class="list">${questionHtml}</div>
          <div id="sectionSubmitWrap"></div>
        </section>
      `;
    }
    return renderWaitingSection(section, index);
  }));
  $('classroom').innerHTML = `
    <div class="panel student-stage-banner">
      <h2>${safeHtml(course?.title || '当前课堂')}</h2>
      <p class="meta">${safeHtml(student?.name || '')}，右侧内容会跟随老师课堂进度自动切换。</p>
    </div>
    <div class="student-section-stack">
      ${sectionHtml.join('')}
    </div>
  `;
  if (retryableQuizIndex >= 0) {
    const quizSection = state.sections[retryableQuizIndex];
    const data = await api(`/sections/${quizSection.id}/questions`);
    const questions = (data.questions || []).filter((item) => item.enabled);
    const questionsNode = $('quizRetryQuestions');
    if (questionsNode) {
      questionsNode.innerHTML = state.retryingQuizSectionId === quizSection.id
        ? (questions.length ? questions.map(renderQuestion).join('') : '<div class="empty">当前部分还没有题目。</div>')
        : '';
      setupChoiceOptions(questionsNode);
    }
    const sectionSubmitWrap = $('quizRetrySubmitWrap');
    if (sectionSubmitWrap) {
      const resultData = latestQuizAttemptResult(quizSection.id) || loadQuizResultCache();
      if (state.retryingQuizSectionId === quizSection.id) {
        sectionSubmitWrap.innerHTML = `
          <div id="quizRetryResultPage" class="quiz-result-page hidden"></div>
          <button type="button" id="submitQuizRetryBtn">提交再测</button>
          <div id="quizRetrySubmitResult" class="section-submit-result"></div>
        `;
      } else {
        sectionSubmitWrap.innerHTML = `
          <div id="quizRetryResultPage" class="quiz-result-page"></div>
          <div class="student-complete-actions">
            <button type="button" id="startQuizRetryBtn" class="student-history-jump-btn">再测一次</button>
          </div>
          <div id="quizRetrySubmitResult" class="section-submit-result"></div>
        `;
        if (resultData) {
          renderQuizResult(resultData, true, 'quizRetryResultPage', 'quizRetrySubmitResult');
        } else {
          $('quizRetryResultPage').innerHTML = '<div class="empty">暂未找到小测结果，请刷新后重试。</div>';
        }
      }
    }
    $('submitQuizRetryBtn')?.addEventListener('click', () => submitQuizRetry(quizSection, questions));
    $('startQuizRetryBtn')?.addEventListener('click', () => startQuizRetake(quizSection.id));
  }
  const activeSection = activeIndex >= 0 ? state.sections[activeIndex] : null;
  if (!activeSection) {
    state.currentQuestions = [];
    return;
  }
  if (activeSection.type === 'quiz') {
    const resultData = state.retryingQuizSectionId === activeSection.id
      ? null
      : (loadQuizResultCache() || latestQuizAttemptResult(activeSection.id));
    $('sectionSubmitWrap').innerHTML = `
      <div id="quizResultPage" class="quiz-result-page hidden"></div>
      ${resultData ? `
        <div class="student-complete-actions">
          <button type="button" id="startQuizRetryBtn" class="student-history-jump-btn">再测一次</button>
        </div>
      ` : '<button type="button" id="submitSectionBtn">提交小测</button>'}
      <div id="sectionSubmitResult" class="section-submit-result"></div>
    `;
    if (resultData) {
      state.lastQuizResult = resultData;
      const questionsNode = $('questions');
      if (questionsNode) questionsNode.innerHTML = '';
      renderQuizResult(resultData, true);
      const quizResultPage = $('quizResultPage');
      if (quizResultPage) quizResultPage.classList.remove('hidden');
    }
  } else if (activeSection.type === 'reflection') {
    $('sectionSubmitWrap').innerHTML = `
      <div id="reflectionScoreCompare"></div>
      <button type="button" id="submitSectionBtn">完成反思</button>
      <div id="sectionSubmitResult" class="section-submit-result"></div>
    `;
    await renderScoreCompare();
  } else if (activeSection.type === 'learning') {
    $('sectionSubmitWrap').innerHTML = `
      <button type="button" id="submitSectionBtn">提交当前部分</button>
      <div id="sectionSubmitResult" class="section-submit-result"></div>
    `;
    setOpenTextEditorsLocked(false);
  } else {
    $('sectionSubmitWrap').innerHTML = `
      <button type="button" id="submitSectionBtn">提交当前部分</button>
      <div id="sectionSubmitResult" class="section-submit-result"></div>
    `;
  }
  $('submitSectionBtn')?.addEventListener('click', submitCurrentSection);
  $('startQuizRetryBtn')?.addEventListener('click', () => {
    if (activeSection) startQuizRetake(activeSection.id);
  });
  setupChoiceOptions();
  setupAIChatQuestions();
  setupOpenTextEditors();
}

async function renderCompletionState(student) {
  const course = currentCourse();
  const hasScoreCompare = state.sections.some((section) => ['prediction', 'quiz', 'reflection'].includes(section.type));
  state.activeSection = null;
  state.currentQuestions = [];
  $('classroom').innerHTML = `
    <div class="panel student-stage-banner">
      <h2>${safeHtml(course?.title || '当前课堂')}</h2>
      <p class="meta">${safeHtml(student?.name || '')}，你已经完成了本节课的全部任务。</p>
    </div>
    ${hasScoreCompare ? '<div id="scoreCompare" class="student-complete-score-compare"></div>' : ''}
    <div class="panel student-complete-card">
      <div class="waiting-message">
        <div class="emoji">🎉</div>
        <div class="student-complete-badge">学习任务完成</div>
        <p>太棒了！你已经完成所有的问卷！<br>谢谢你的参与！🌟</p>
      </div>
      <div class="student-complete-actions">
        <button type="button" id="historyJumpBtn" class="student-history-jump-btn">查看我的历史课堂答题数据</button>
      </div>
    </div>
  `;
  if (hasScoreCompare) {
    try {
      await renderScoreCompare();
    } catch (error) {
      console.warn('渲染分数对比失败', error);
    }
  }
  const historyBtn = $('historyJumpBtn');
  if (historyBtn) {
    historyBtn.addEventListener('click', openHistoryPage);
  }
  renderProgress();
}

function renderProgress() {
  const stageID = Number(state.classroom?.stageId || 0);
  const stageIndex = sectionIndexById(stageID);
  const nextIndex = nextIncompleteSectionIndex();
  const activeIndex = nextOpenedIncompleteSectionIndex(stageIndex);
  const parts = [
    { id: 0, title: '准备环节' },
    ...state.sections.map((section, index) => ({ id: section.id, title: `第${index + 1}部分` }))
  ];
  const allCompleted = state.sections.length > 0
    && state.sections.every((section) => state.completedSectionIds.has(section.id));
  const prepCompleted = stageID > 0 || state.completedSectionIds.size > 0;
  const completed = parts.filter((part) => {
    if (part.id === 0) {
      return prepCompleted;
    }
    return state.completedSectionIds.has(part.id);
  }).length;
  const total = Math.max(parts.length, 1);
  const percent = Math.round((completed / total) * 100);
  const activeTitle = stageID === 0
    ? '准备环节'
    : (activeIndex >= 0 ? parts[activeIndex + 1]?.title : parts.find((part) => part.id === stageID)?.title) || '当前部分';

  $('studentProgressSummary').textContent = allCompleted
    ? '你已完成本节课全部任务。'
    : (stageID === 0 && !prepCompleted
      ? '正在等待课堂开始'
      : `当前进行中：${activeTitle}`);
  $('studentProgressCount').textContent = `已完成 ${completed} / ${total}`;
  $('studentProgressMeterFill').style.width = `${percent}%`;
  $('studentProgressList').innerHTML = parts.map((part, index) => {
    const isPrep = part.id === 0;
    const sectionIndex = index - 1;
    const done = isPrep ? prepCompleted : state.completedSectionIds.has(part.id);
    const active = isPrep ? !prepCompleted : (!done && sectionIndex === activeIndex && stageID > 0);
    const waiting = !done && !active && sectionIndex >= 0 && stageID > 0 && sectionIndex <= stageIndex;
    const statusClass = done ? 'done' : active ? 'active' : waiting ? 'waiting' : 'pending';
    const statusText = done ? '已完成' : active ? (isPrep ? '等待开启' : '进行中') : waiting ? '已开放，先完成前面部分' : '未开始';
    const icon = done ? '✓' : active ? (isPrep ? '…' : '▶') : waiting ? '…' : '○';
    return `
      <div class="student-progress-item student-progress-item--${statusClass}">
        <span class="student-progress-item__icon">${icon}</span>
        <div>
          <strong>${safeHtml(part.title)}</strong>
          <span>${statusText}</span>
        </div>
      </div>
    `;
  }).join('');
}

function renderQuestion(question) {
  const options = Array.isArray(question.options) ? question.options : [];
  const scoreRole = question.rules?.scoreRole || question.scoreRole || '';
  if (scoreRole === 'prediction') {
    return renderPredictionQuestion(question, options);
  }
  if (question.type === 'ai_chat') {
    return renderAIChatQuestion(question);
  }
  const optionHtml = options.map((item, index) => {
    const value = optionValue(item, index);
    const optionId = `q_${question.id}_${index}`;
    const isOther = isOtherOption(item);
    return `
      <label class="option${isOther ? ' option--other' : ''}" for="${optionId}" data-option-index="${index}" data-other-option="${isOther ? 'true' : 'false'}">
        <input type="${question.type === 'multiple_choice' ? 'checkbox' : 'radio'}" id="${optionId}" name="q_${question.id}" value="${safeHtml(value)}">
        <span class="option-text">
          ${isOther ? `
            <span class="choice-other-label">${OTHER_OPTION_LABEL}：</span>
            <input type="text" class="choice-other-input" data-question-id="${question.id}" data-option-index="${index}" placeholder="请填写其它内容">
          ` : renderRichText(item)}
        </span>
      </label>
    `;
  }).join('');
  const answerHtml = question.type === 'single_choice' || question.type === 'multiple_choice'
    ? `<div class="options">${optionHtml}</div>`
    : `<textarea name="q_${question.id}" placeholder="写下你的想法"></textarea>`;
  return `
    <div class="item">
      <div class="item-title student-question-title">${renderRichText(question.title)}</div>
      ${question.description ? `<div class="meta">${renderRichText(question.description)}</div>` : ''}
      <div class="meta">${safeHtml(questionTypeLabel(question.type))}</div>
      ${question.type === 'open_text'
        ? renderOpenTextQuestion(question)
        : answerHtml || '<div class="empty">AI 对话题暂无内容，请先发送学习相关问题。</div>'}
    </div>
  `;
}

function renderAIChatQuestion(question) {
  const messages = latestQuestionChatMessages(question.id);
  const rounds = countAIRounds(messages);
  const reachedLimit = rounds >= 5;
  const pending = state.aiChatPending.get(aiChatKey(question.id)) === true;
  setStoredAIChatMessages(question.id, messages);
  return `
    <div class="item ai-chat-question" data-question-id="${question.id}">
      <div class="item-title student-question-title">${renderRichText(normalizeAIChatTitle(question.title))}</div>
      ${question.description ? `<div class="meta">${renderRichText(question.description)}</div>` : ''}
      <div class="meta">AI 对话题 · 已对话 <span id="ai-chat-rounds-${question.id}">${rounds}</span> / 5 轮</div>
      <div class="ai-chat-box">
        <div id="ai-chat-messages-${question.id}" class="ai-chat-messages">${renderAIChatMessages(messages, pending)}</div>
        <div class="ai-chat-input-row">
          <textarea id="ai-chat-input-${question.id}" class="ai-chat-input" placeholder="请输入学习相关的问题，例如：人工智能为什么能理解文字？" maxlength="500" ${reachedLimit ? 'disabled' : ''}></textarea>
          <button type="button" class="ai-chat-send-btn" data-question-id="${question.id}" ${reachedLimit ? 'disabled' : ''}>${reachedLimit ? '已达上限' : '发送'}</button>
        </div>
        <div id="ai-chat-status-${question.id}" class="ai-chat-status">${reachedLimit ? '已达到 5 轮上限，可以提交本部分。' : AI_CHAT_GUIDE_TEXT}</div>
      </div>
    </div>
  `;
}

function syncChoiceOptionState(scope = document) {
  scope.querySelectorAll('.option, .rating-option').forEach((option) => {
    const input = option.querySelector('input[type="radio"], input[type="checkbox"]');
    option.classList.toggle('selected', !!input?.checked);
  });
}

function setupChoiceOptions(scope = document) {
  scope.querySelectorAll('.student-stage-block .option, .option, .rating-option').forEach((option) => {
    if (option.dataset.choiceBound === 'true') return;
    option.dataset.choiceBound = 'true';
    const input = option.querySelector('input[type="radio"], input[type="checkbox"]');
    if (!input) return;

    input.addEventListener('change', () => {
      const syncScope = option.closest('.student-stage-block, .student-quiz-block, .question-item, .item, .rating-options') || document;
      syncChoiceOptionState(syncScope);
    });
    option.addEventListener('pointerdown', (event) => {
      if (event.target.closest('.choice-other-input')) return;
      event.preventDefault();
    });
    option.addEventListener('click', (event) => {
      if (event.target.closest('.choice-other-input')) {
        if (!input.checked) {
          input.checked = true;
          input.dispatchEvent(new Event('change', { bubbles: true }));
        }
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      if (input.type === 'checkbox') {
        input.checked = !input.checked;
      } else {
        input.checked = true;
      }
      input.dispatchEvent(new Event('change', { bubbles: true }));
    });
  });
  scope.querySelectorAll('.choice-other-input').forEach((textInput) => {
    if (textInput.dataset.otherBound === 'true') return;
    textInput.dataset.otherBound = 'true';
    const option = textInput.closest('.option');
    const choiceInput = option?.querySelector('input[type="radio"], input[type="checkbox"]');
    const activate = () => {
      if (!choiceInput || choiceInput.checked) return;
      choiceInput.checked = true;
      choiceInput.dispatchEvent(new Event('change', { bubbles: true }));
    };
    textInput.addEventListener('focus', activate);
    textInput.addEventListener('input', activate);
  });
  syncChoiceOptionState(scope);
}

function updateAIChatView(questionId, messages, pending = false) {
  const normalized = normalizeChatMessages(messages);
  state.aiChatPending.set(aiChatKey(questionId), Boolean(pending));
  const messagesNode = $(`ai-chat-messages-${questionId}`);
  if (messagesNode) {
    messagesNode.innerHTML = renderAIChatMessages(normalized, pending);
    messagesNode.scrollTop = messagesNode.scrollHeight;
  }
  const rounds = countAIRounds(normalized);
  const roundsNode = $(`ai-chat-rounds-${questionId}`);
  if (roundsNode) roundsNode.textContent = String(rounds);
  const input = $(`ai-chat-input-${questionId}`);
  const sendBtn = document.querySelector(`.ai-chat-send-btn[data-question-id="${questionId}"]`);
  const statusNode = $(`ai-chat-status-${questionId}`);
  const reachedLimit = rounds >= 5;
  if (input) input.disabled = reachedLimit || pending;
  if (sendBtn) {
    sendBtn.disabled = reachedLimit || pending;
    sendBtn.textContent = pending ? '思考中' : (reachedLimit ? '已达上限' : '发送');
  }
  if (statusNode && reachedLimit) {
    statusNode.textContent = '已达到 5 轮上限，可以提交本部分。';
  }
}

function setupAIChatQuestions() {
  document.querySelectorAll('.ai-chat-send-btn').forEach((button) => {
    if (button.dataset.bound === 'true') return;
    button.dataset.bound = 'true';
    button.addEventListener('click', () => sendAIChatMessage(Number(button.dataset.questionId || 0)));
  });
  document.querySelectorAll('.ai-chat-input').forEach((input) => {
    if (input.dataset.bound === 'true') return;
    input.dataset.bound = 'true';
    input.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
        event.preventDefault();
        const questionId = Number(input.id.replace('ai-chat-input-', '') || 0);
        sendAIChatMessage(questionId);
      }
    });
  });
}

async function sendAIChatMessage(questionId) {
  const question = state.currentQuestions.find((item) => Number(item.id) === Number(questionId));
  const input = $(`ai-chat-input-${questionId}`);
  const statusNode = $(`ai-chat-status-${questionId}`);
  const message = input?.value.trim() || '';
  if (!question || !state.activeSection) {
    if (statusNode) statusNode.textContent = '当前题目暂时不可对话，请刷新后重试。';
    return;
  }
  if (!message) {
    if (statusNode) statusNode.textContent = '请先输入一个学习相关的问题。';
    return;
  }
  const messages = getStoredAIChatMessages(questionId);
  if (countAIRounds(messages) >= 5) {
    updateAIChatView(questionId, messages, false);
    return;
  }
  const optimisticMessages = normalizeChatMessages([...messages, { role: 'user', content: message }]);
  setStoredAIChatMessages(questionId, optimisticMessages);
  updateAIChatView(questionId, optimisticMessages, true);
  if (statusNode) statusNode.textContent = 'AI 正在思考，请稍等...';
  if (input) input.value = '';
  try {
    const data = await api('/student/ai-chat', {
      method: 'POST',
      body: JSON.stringify({
        courseId: state.selectedCourseId,
        classId: state.selectedClassId,
        studentId: state.student.id,
        sectionId: state.activeSection.id,
        questionId: question.id,
        message,
        messages
      })
    });
    const nextMessages = normalizeChatMessages(data.messages || []);
    setStoredAIChatMessages(questionId, nextMessages);
    updateAIChatView(questionId, nextMessages, false);
    if ($(`ai-chat-status-${questionId}`) && countAIRounds(nextMessages) < 5) {
      $(`ai-chat-status-${questionId}`).textContent = '本题已完成至少一轮对话，可以提交本部分，也可以继续追问。';
    }
  } catch (error) {
    setStoredAIChatMessages(questionId, messages);
    updateAIChatView(questionId, messages, false);
    if (input) input.value = message;
    if (statusNode) statusNode.textContent = `对话失败：${error.message}`;
  }
}

function parsePredictionOption(option, index) {
  const text = String(option ?? '').trim();
  const parts = text.split(/\s*-\s*/);
  const rawScore = parts[0] || `${index}分`;
  const scoreMatch = rawScore.match(/(\d+)/);
  const score = scoreMatch ? scoreMatch[1] : String(index);
  const desc = parts.slice(1).join(' - ').trim() || text;
  return { score, desc };
}

function renderPredictionQuestion(question, options) {
  const questionText = question.description || question.title || '请选择一个最符合的答案。';
  const scoreCards = options.length ? options.map((option, index) => {
    const parsed = parsePredictionOption(option, index);
    const emoji = ['😰', '😟', '😅', '🤔', '😊', '🌟'][index] || '⭐';
    return `
      <label class="rating-option">
        <input type="radio" name="q_${question.id}" value="${safeHtml(parsed.score)}">
        <div class="emoji">${emoji}</div>
        <div class="score">${safeHtml(parsed.score)}分</div>
        <div class="desc">${renderRichText(parsed.desc)}</div>
      </label>
    `;
  }).join('') : '<div class="empty">暂无评分选项。</div>';
  return `
    <section class="question-item student-quiz-block student-quiz-block--prediction">
      <div class="student-quiz-head">
        <span class="student-quiz-tag">热身关卡</span>
        <h3>题目一：猜一猜 🤔</h3>
      </div>
      <p class="student-quiz-text">${renderRichText(questionText)}</p>
      <div class="rating-options" id="prediction-score">
        ${scoreCards}
      </div>
    </section>
  `;
}

function renderOpenTextQuestion(question) {
  const annotationEnabled = true;
  return `
    <div class="student-open-question">
      <p class="student-quiz-text">${annotationEnabled ? '请写下你的思考，并按要求用颜色标注重点、疑惑和错误观点。' : '请写下你的思考，完整表达自己的想法。'}</p>
      ${annotationEnabled ? `
        <div class="student-part2-toolbar">
          <button type="button" class="btn student-mark-btn student-mark-btn--green" onclick="applyOpenTextHighlight('green', ${question.id})">绿色：关键事实或观点</button>
          <button type="button" class="btn student-mark-btn student-mark-btn--yellow" onclick="applyOpenTextHighlight('yellow', ${question.id})">黄色：完全看不懂或不清楚的地方</button>
          <button type="button" class="btn student-mark-btn student-mark-btn--red" onclick="applyOpenTextHighlight('red', ${question.id})">红色：明显错误或自己不同意的内容</button>
          <button type="button" class="btn student-mark-btn student-mark-btn--clear" onclick="clearOpenTextHighlight(${question.id})">去除颜色</button>
        </div>
      ` : ''}
      <div class="student-open-editor-wrap">
        <div id="open-text-editor-${question.id}" class="student-open-editor" contenteditable="true" data-annotation-enabled="${annotationEnabled ? 'true' : 'false'}" data-placeholder="${annotationEnabled ? '请在这里写下你的答案，并至少给一处文字加上颜色标注...' : '请在这里写下你的答案...'}"></div>
      </div>
      <div id="open-text-results-${question.id}" class="hidden student-inline-result">
        <h4>参考解析</h4>
        <p id="open-text-explanation-${question.id}" class="open-text-explanation"></p>
      </div>
    </div>
  `;
}

function renderRichExplanationContent(content) {
  return String(content || '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/\n/g, '<br>');
}

function isOtherOption(value) {
  const text = String(value ?? '').trim().replace(/\s+/g, '');
  return /^其[它他][:：_]+$/.test(text) || /^其[它他][:：]/.test(text);
}

function otherAnswerValue(questionId, optionIndex) {
  const input = document.querySelector(`.choice-other-input[data-question-id="${questionId}"][data-option-index="${optionIndex}"]`);
  const text = input?.value.trim() || '';
  return text ? `${OTHER_OPTION_LABEL}：${text}` : '';
}

function optionValue(text, index) {
  const raw = String(text || '').trim();
  if (isOtherOption(raw)) return `other_${index}`;
  const match = raw.match(/^([A-Z])[\.\s、]/i);
  if (match) return match[1].toUpperCase();
  const scoreMatch = raw.match(/^(\d+)分/);
  if (scoreMatch) return scoreMatch[1];
  return String(index);
}

async function renderScoreCompare() {
  if (!$('scoreCompare') && !$('reflectionScoreCompare')) return;
  const data = await api(`/student/score-summary?courseId=${state.selectedCourseId}&classId=${state.selectedClassId}&studentId=${state.student.id}`);
  const summary = data.summary || {};
  const target = $('scoreCompare') || $('reflectionScoreCompare');
  const guessResultText = summary.guessResultText || '-';
  const feedbackMap = {
    猜中: '你猜得很准，说明你很了解自己！',
    猜高: '你猜高了，可能有些地方没听懂哦。',
    猜低: '你猜低了，其实你比想象中厉害！'
  };
  target.innerHTML = `
    <div class="score-compare-card">
      <h3>1. 我的学习成果 📊</h3>
      <div class="score-compare">
        <div class="score-item">
          <div class="label">小测得分</div>
          <div class="value">${summary.actualScore ?? '待生成'}</div>
        </div>
        <div class="score-item">
          <div class="label">预测分数</div>
          <div class="value">${summary.predictedScore ?? '未填写'}</div>
        </div>
      </div>
      <div class="feedback ${guessResultText === '猜高' ? 'warning' : 'success'}">
        ${guessResultText === '-' ? '等待分数生成后展示对比结果。' : (feedbackMap[guessResultText] || `对比结果：${safeHtml(guessResultText)}`)}
      </div>
    </div>
  `;
}

function getOpenTextEditor(questionId = '') {
  return document.getElementById(`open-text-editor-${questionId}`);
}

function getOpenTextEditorHtml(questionId = '') {
  const editor = getOpenTextEditor(questionId);
  return editor ? editor.innerHTML.trim() : '';
}

function countOpenTextHighlights(root) {
  return countAnnotatedHighlights(root);
}

function setOpenTextEditorsLocked(locked) {
  document.querySelectorAll('.student-open-editor').forEach((editor) => {
    editor.contentEditable = locked ? 'false' : 'true';
    editor.classList.toggle('is-locked', locked);
  });
  document.querySelectorAll('.student-part2-toolbar button').forEach((button) => {
    button.disabled = locked;
  });
}

function setupOpenTextEditors() {
  document.querySelectorAll('.student-open-editor').forEach((editor) => {
    if (editor.dataset.pasteBound === 'true') return;
    editor.dataset.pasteBound = 'true';
    editor.addEventListener('paste', (event) => {
      event.preventDefault();
      const pastedText = (event.clipboardData?.getData('text/plain') || '')
        .replace(/\r\n?/g, '\n')
        .replace(/\u00a0/g, ' ')
        .replace(/\t/g, '    ')
        .replace(/[\u200b-\u200d\uFEFF]/g, '');
      const selection = window.getSelection();
      if (!selection || selection.rangeCount === 0) {
        editor.appendChild(document.createTextNode(pastedText));
        editor.normalize();
        return;
      }
      let range = selection.getRangeAt(0);
      const selectionInsideEditor = !!selection.anchorNode && !!selection.focusNode
        && editor.contains(selection.anchorNode)
        && editor.contains(selection.focusNode)
        && editor.contains(range.commonAncestorContainer);
      if (!selectionInsideEditor) {
        range = document.createRange();
        range.selectNodeContents(editor);
        range.collapse(false);
        selection.removeAllRanges();
        selection.addRange(range);
      }
      range.deleteContents();
      const textNode = document.createTextNode(pastedText);
      range.insertNode(textNode);
      range.setStartAfter(textNode);
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
      editor.normalize();
    });
  });
}

function getOpenTextSelectedTextSegments(editor, range) {
  if (!editor || !range) return [];
  const segments = [];
  const walker = document.createTreeWalker(editor, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (!node.textContent || !node.textContent.trim()) {
        return NodeFilter.FILTER_REJECT;
      }
      try {
        return range.intersectsNode(node) ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT;
      } catch (error) {
        return NodeFilter.FILTER_REJECT;
      }
    }
  });
  let node = walker.nextNode();
  while (node) {
    const start = range.startContainer === node ? range.startOffset : 0;
    const end = range.endContainer === node ? range.endOffset : node.textContent.length;
    if (end > start) {
      segments.push({ node, start, end });
    }
    node = walker.nextNode();
  }
  return segments;
}

function isolateOpenTextSegment(node, start, end) {
  let target = node;
  if (end < target.textContent.length) {
    target.splitText(end);
  }
  if (start > 0) {
    target = target.splitText(start);
  }
  return target;
}

function unwrapOpenTextHighlightForTextNode(textNode) {
  if (!textNode || !textNode.parentNode) return textNode;
  const highlight = textNode.parentNode;
  if (!(highlight instanceof HTMLElement) || !highlight.classList.contains('part2-highlight')) {
    return textNode;
  }
  const parent = highlight.parentNode;
  if (!parent) return textNode;
  const hasBefore = !!textNode.previousSibling;
  const hasAfter = !!textNode.nextSibling;
  if (hasBefore && hasAfter) {
    const afterWrapper = highlight.cloneNode(false);
    while (textNode.nextSibling) {
      afterWrapper.appendChild(textNode.nextSibling);
    }
    parent.insertBefore(afterWrapper, highlight.nextSibling);
    parent.insertBefore(textNode, afterWrapper);
  } else if (hasBefore) {
    parent.insertBefore(textNode, highlight.nextSibling);
  } else if (hasAfter) {
    parent.insertBefore(textNode, highlight);
  } else {
    parent.insertBefore(textNode, highlight);
  }
  if (!highlight.textContent) {
    parent.removeChild(highlight);
  }
  return textNode;
}

function wrapOpenTextNode(textNode, color) {
  if (!textNode || !textNode.parentNode || !textNode.textContent || !textNode.textContent.trim()) {
    return 0;
  }
  const span = document.createElement('span');
  span.className = `part2-highlight part2-highlight--${color}`;
  textNode.parentNode.replaceChild(span, textNode);
  span.appendChild(textNode);
  return 1;
}

function applyOpenTextHighlight(color, questionId) {
  const editor = getOpenTextEditor(questionId);
  if (!editor) return;
  editor.focus();
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
    window.showStudentAlert?.('请先选中你要标注的文字。', 'warning');
    return;
  }
  const range = selection.getRangeAt(0);
  if (!editor.contains(range.commonAncestorContainer)) {
    window.showStudentAlert?.('请在答题区域内选择文字后再标注。', 'warning');
    return;
  }
  const segments = getOpenTextSelectedTextSegments(editor, range);
  if (segments.length === 0) {
    window.showStudentAlert?.('请选中具体文字后再标注。', 'warning');
    return;
  }
  let appliedCount = 0;
  segments.reverse().forEach(({ node, start, end }) => {
    const isolatedNode = isolateOpenTextSegment(node, start, end);
    const plainNode = unwrapOpenTextHighlightForTextNode(isolatedNode);
    appliedCount += wrapOpenTextNode(plainNode, color);
  });
  if (appliedCount === 0) {
    window.showStudentAlert?.('请选中具体文字后再标注。', 'warning');
    return;
  }
  editor.normalize();
  selection.removeAllRanges();
}

function clearOpenTextHighlight(questionId) {
  const editor = getOpenTextEditor(questionId);
  if (!editor) return;
  editor.focus();
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
    window.showStudentAlert?.('请先选中要去除颜色的文字。', 'warning');
    return;
  }
  const range = selection.getRangeAt(0);
  if (!editor.contains(range.commonAncestorContainer)) {
    window.showStudentAlert?.('请在答题区域内选择文字后再去除颜色。', 'warning');
    return;
  }
  const segments = getOpenTextSelectedTextSegments(editor, range);
  if (segments.length === 0) {
    window.showStudentAlert?.('请选中具体文字后再去除颜色。', 'warning');
    return;
  }
  segments.reverse().forEach(({ node, start, end }) => {
    const isolatedNode = isolateOpenTextSegment(node, start, end);
    unwrapOpenTextHighlightForTextNode(isolatedNode);
  });
  editor.normalize();
  selection.removeAllRanges();
}

function renderQuizResult(data, keepVisible = false, targetId = 'quizResultPage', resultNodeId = 'sectionSubmitResult') {
  const results = data?.results || [];
  const target = $(targetId);
  const resultNode = $(resultNodeId);
  const showWaitingForReflection = targetId === 'quizResultPage';
  const html = `
    <div class="quiz-result-card">
      <h3>小测结果</h3>
      <div class="student-score-display">
        <div class="student-score-display__title">你的得分</div>
        <div class="student-score-display__value">${data?.score ?? 0}/${data?.total ?? results.length}</div>
      </div>
      <p class="meta">你可以再次作答，但系统只记录第一次提交的小测分数。</p>
      ${results.map((item, index) => `
        <div class="answer-item ${item.isCorrect ? 'correct' : 'incorrect'}">
          <h4>${index + 1}. ${renderRichText(item.question || '')}
            <span class="${item.isCorrect ? 'correct-mark' : 'incorrect-mark'}">${item.isCorrect ? '✅ 正确' : '❌ 错误'}</span>
          </h4>
          <p>你的答案：<strong class="${item.isCorrect ? 'student-answer-text--correct' : 'student-answer-text--wrong'}">${safeHtml(item.studentAnswer || '未填写')}</strong></p>
          <p>正确答案：<strong class="student-answer-text--correct">${safeHtml(item.correctAnswer || '-')}</strong></p>
          <p class="explanation"><span class="label">解析：</span><span class="content">${renderRichExplanationContent(item.explanation || '')}</span></p>
        </div>
      `).join('')}
      ${showWaitingForReflection ? `<div class="waiting-message" id="waiting-part4">
        <div class="emoji">⏳</div>
        <p>请等待老师开启第四部分...</p>
      </div>` : ''}
    </div>
  `;
  if (target) {
    target.innerHTML = html;
    target.classList.remove('hidden');
  }
  if (resultNode) {
    resultNode.innerHTML = keepVisible ? '' : html;
  }
  results.forEach((item, index) => {
    if (item.explanation) {
      const explanationSpan = (target || resultNode)?.querySelectorAll('.explanation .content')[index];
      if (explanationSpan) {
        explanationSpan.innerHTML = renderRichExplanationContent(item.explanation);
      }
    }
  });
}

function renderOpenTextSubmissionResults(results = []) {
  const resultMap = new Map((results || []).map((item) => [String(item.questionId), item]));
  state.currentQuestions.forEach((question) => {
    if (question.type !== 'open_text') return;
    const container = $(`open-text-results-${question.id}`);
    if (!container) return;
    const result = resultMap.get(String(question.id)) || {};
    const explanation = result.explanation || question.explanation || '暂无参考解析。';
    const answer = result.studentAnswer || getOpenTextEditorHtml(question.id);
    container.innerHTML = `
      <h4>参考解析</h4>
      <div class="student-inline-result__body">
        <div class="annotated-answer-block"><strong>你的作答</strong><div class="annotated-answer-content">${renderAnnotatedAnswer(answer, '已提交')}</div></div>
        <p id="open-text-explanation-${question.id}" class="open-text-explanation">${renderRichExplanationContent(explanation)}</p>
      </div>
    `;
    container.classList.remove('hidden');
  });
}

window.applyOpenTextHighlight = applyOpenTextHighlight;
window.clearOpenTextHighlight = clearOpenTextHighlight;

function collectCurrentAnswers(questions = state.currentQuestions) {
  const answers = {};
  const missing = [];
  for (const question of questions) {
    const key = String(question.id);
    if (question.type === 'open_text') {
      const editor = getOpenTextEditor(question.id);
      const annotationEnabled = editor?.dataset.annotationEnabled !== 'false';
      const html = sanitizeAnnotatedAnswer(editor?.innerHTML.trim() || '');
      const text = editor?.innerText.trim() || '';
      if (!text) missing.push(question.title);
      if (annotationEnabled && editor && countOpenTextHighlights(editor) === 0) {
        throw new Error('开放题至少需要标注一处颜色');
      }
      answers[key] = html;
      continue;
    }
    if (question.type === 'ai_chat') {
      const messages = getStoredAIChatMessages(question.id);
      const rounds = countAIRounds(messages);
      if (rounds < 1) missing.push(question.title);
      answers[key] = { messages, rounds };
      continue;
    }
    if (question.type === 'multiple_choice') {
      let blankOtherSelected = false;
      const checked = Array.from(document.querySelectorAll(`input[name="q_${question.id}"]:checked`)).map((item) => {
        const option = item.closest('.option');
        if (option?.dataset.otherOption === 'true') {
          const otherValue = otherAnswerValue(question.id, option.dataset.optionIndex || 0);
          if (!otherValue) blankOtherSelected = true;
          return otherValue;
        }
        return item.value;
      }).filter(Boolean);
      if (!checked.length) missing.push(question.title);
      if (blankOtherSelected) missing.push(`${question.title}（请填写其它内容）`);
      answers[key] = checked;
      continue;
    }
    if (question.type === 'single_choice') {
      const checked = document.querySelector(`input[name="q_${question.id}"]:checked`);
      if (!checked) missing.push(question.title);
      const option = checked?.closest('.option');
      answers[key] = option?.dataset.otherOption === 'true'
        ? otherAnswerValue(question.id, option.dataset.optionIndex || 0)
        : checked?.value || '';
      if (checked && option?.dataset.otherOption === 'true' && !answers[key]) {
        missing.push(`${question.title}（请填写其它内容）`);
      }
      continue;
    }
    const textarea = document.querySelector(`textarea[name="q_${question.id}"]`);
    const value = textarea?.value.trim() || '';
    if (!value) missing.push(question.title);
    answers[key] = value;
  }
  if (missing.length) {
    throw new Error(`还有题目未完成：${missing.map((item, index) => `${index + 1}. ${item}`).join('；')}`);
  }
  return answers;
}

async function submitCurrentSection() {
  if (!state.activeSection) return;
  const resultNode = $('sectionSubmitResult');
  try {
    const answers = collectCurrentAnswers();
    const data = await api('/student/submit-section', {
      method: 'POST',
      body: JSON.stringify({
        courseId: state.selectedCourseId,
        classId: state.selectedClassId,
        studentId: state.student.id,
        sectionId: state.activeSection.id,
        answers
      })
    });
    if (state.activeSection.type === 'quiz') {
      const submittedSectionId = state.activeSection.id;
      state.lastQuizResult = data;
      saveQuizResultCache(data);
      state.retryingQuizSectionId = null;
      state.completedSectionIds.add(submittedSectionId);
      await syncCompletedSectionsFromHistory();
      await renderClassroom(state.student);
      await renderScoreCompare();
      return;
    } else if (state.activeSection.type === 'reflection') {
      state.completedSectionIds.add(state.activeSection.id);
      renderProgress();
      resultNode.innerHTML = `<div class="success-message">完成反思</div>`;
      await renderScoreCompare();
    } else if (state.activeSection.type === 'learning') {
      state.completedSectionIds.add(state.activeSection.id);
      renderProgress();
      renderOpenTextSubmissionResults(data.results || []);
      setOpenTextEditorsLocked(true);
      resultNode.innerHTML = `<div class="success-message">提交成功</div>`;
    } else {
      state.completedSectionIds.add(state.activeSection.id);
      renderProgress();
      resultNode.innerHTML = `<div class="success-message">提交成功</div>`;
    }
    await refreshClassroomState({ forceRender: true });
  } catch (error) {
    resultNode.innerHTML = `<div class="error-message">${safeHtml(error.message)}</div>`;
  }
}

async function submitQuizRetry(quizSection, questions) {
  const resultNode = $('quizRetrySubmitResult');
  try {
    const answers = collectCurrentAnswers(questions);
    const data = await api('/student/submit-section', {
      method: 'POST',
      body: JSON.stringify({
        courseId: state.selectedCourseId,
        classId: state.selectedClassId,
        studentId: state.student.id,
        sectionId: quizSection.id,
        answers
      })
    });
    state.lastQuizResult = data;
    saveQuizResultCache(data);
    state.retryingQuizSectionId = null;
    await syncCompletedSectionsFromHistory();
    await renderClassroom(state.student);
    await renderScoreCompare();
  } catch (error) {
    if (resultNode) {
      resultNode.innerHTML = `<div class="error-message">${safeHtml(error.message)}</div>`;
    }
  }
}

function questionTypeLabel(type) {
  const labels = {
    single_choice: '单选题',
    multiple_choice: '多选题',
    fill_blank: '填空题',
    open_text: '开放题',
    ai_chat: 'AI 对话题'
  };
  return labels[type] || type;
}

function openHistoryPage() {
  clearLoginMessage();
  let student;
  try {
    student = validateStudent();
  } catch (error) {
    showLoginError(error.message);
    return;
  }
  const params = new URLSearchParams({
    classId: String(state.selectedClassId),
    studentId: String(student.id),
    className: $('classSelect').selectedOptions[0]?.textContent || '',
    studentName: student.name
  });
  window.location.href = `./student-history.html?${params.toString()}`;
}

document.addEventListener('DOMContentLoaded', async () => {
  try {
    await loadPage();
    enhanceCustomSelects(document);
    $('classSelect').addEventListener('change', loadClassScopedData);
    $('studentNameInput').addEventListener('input', () => {
      state.student = null;
      renderStudentNameList();
    });
    $('studentNameInput').addEventListener('focus', renderStudentNameList);
    $('studentNameList').addEventListener('click', (event) => {
      const option = event.target.closest('.student-name-option');
      if (!option) return;
      chooseStudentName(option.dataset.name || option.textContent.trim());
    });
    document.addEventListener('click', (event) => {
      if (!event.target.closest('#studentNameCombobox')) {
        closeStudentNameList();
      }
    });
    $('enterBtn').addEventListener('click', enterClassroom);
    $('historyBtn').addEventListener('click', openHistoryPage);
  } catch (error) {
    $('classroom').innerHTML = `<div class="panel">${safeHtml(error.message)}</div>`;
  }
});

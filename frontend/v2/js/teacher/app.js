import { api, apiBase } from '../core/api.js?v=2026080202';
import { enhanceCustomSelects } from '../core/custom-select.js?v=2026062034';
import { renderAnnotatedAnswer } from '../core/annotated-answer.js';
import { renderMarkdown } from '../core/markdown.js';
import { renderRichText } from '../core/rich-text.js?v=2026071101';

const legacyApiBase = () => `${window.location.protocol}//${window.location.hostname}:8080/api`;

const state = {
  classes: [],
  templates: [],
  courses: [],
  selectedClassId: null,
  selectedCourseId: null,
  selectedSectionId: null,
  sections: [],
  classroom: null,
  stats: null,
  questions: [],
  aiGuidanceConfig: null,
  boundCourseIds: [],
  currentStatsStudents: []
};

const TEACHER_PASSWORD = '0506';
const OTHER_OPTION_TEXT = '其它：____';
let appStarted = false;
let savingQuestionInFlight = false;

const $ = (id) => document.getElementById(id);
const $on = (id, eventName, handler) => {
  const element = $(id);
  if (!element) return;
  element.addEventListener(eventName, handler);
};

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function sectionByPart(part) {
  return state.sections[Number(part) - 1] || null;
}

function partBySectionId(sectionId) {
  return state.sections.findIndex((item) => item.id === Number(sectionId)) + 1;
}

function stageLabelBySectionId(sectionId) {
  const stageId = Number(sectionId || 0);
  if (!stageId) return '准备环节';
  const part = partBySectionId(stageId);
  const section = state.sections.find((item) => item.id === stageId);
  return part > 0 ? `第${part}部分：${section.title}` : `部分 ID ${stageId}`;
}

function renderList(node, items, renderer, emptyText) {
  node.innerHTML = items.length ? items.map(renderer).join('') : `<div class="empty">${emptyText}</div>`;
}

async function loadAll() {
  $('apiBase').textContent = apiBase();
  const [classData, templateData, courseData] = await Promise.all([
    api('/classes'),
    api('/course-templates'),
    api('/courses')
  ]);
  state.classes = classData.classes || [];
  state.templates = templateData.templates || [];
  state.courses = courseData.courses || [];
  if (!state.selectedClassId && state.classes[0]) state.selectedClassId = state.classes[0].id;
  if (!state.selectedCourseId && state.courses[0]) state.selectedCourseId = state.courses[0].id;
  renderTemplateOptions();
  await loadClassCourseBindings();
  renderClasses();
  renderCourses();
  await loadSections();
  await loadStudents();
  await loadClassroom();
  await loadStats();
}

function renderTemplateOptions() {
  $('courseTemplate').innerHTML = state.templates.map((tpl) => (
    `<option value="${tpl.code}">${tpl.name}</option>`
  )).join('');
}

function renderClasses() {
  const options = state.classes.map((item) => (
    `<option value="${item.id}" ${item.id === state.selectedClassId ? 'selected' : ''}>${item.name}</option>`
  )).join('');
  $('classSelectMain').innerHTML = `<option value="">请选择班级</option>${options}`;
  $('classSelect').innerHTML = `<option value="">请选择班级</option>${options}`;
  $('statsClassSelect').innerHTML = [
    '<option value="">所有班级</option>',
    ...state.classes.map((item) => `<option value="${item.id}">${item.name}</option>`)
  ].join('');

  const selectedClass = state.classes.find((item) => item.id === state.selectedClassId);
  if (selectedClass) {
    $('classInfo').innerHTML = `
      <div class="item-title">
        <span>${selectedClass.name}</span>
        <span class="badge">${selectedClass.studentCount || 0} 名学生</span>
      </div>
    `;
    $('classInfo').classList.remove('empty');
  } else {
    $('classInfo').innerHTML = '请选择班级查看信息。';
    $('classInfo').classList.add('empty');
  }
  renderClassCourseBinding();
}

async function loadClassCourseBindings() {
  state.boundCourseIds = [];
  if (!state.selectedClassId) return;
  const data = await api(`/classes/${state.selectedClassId}/courses`);
  state.boundCourseIds = (data.courses || []).map((item) => item.id);
  if (state.boundCourseIds.length) {
    if (!state.boundCourseIds.includes(state.selectedCourseId)) {
      state.selectedCourseId = state.boundCourseIds[0];
      state.selectedSectionId = null;
    }
  } else {
    state.selectedCourseId = null;
    state.selectedSectionId = null;
  }
}

function renderClassCourseBinding() {
  const list = $('classCourseBindingList');
  if (!list) return;
  if (!state.selectedClassId) {
    list.innerHTML = '<div class="empty">请先选择班级。</div>';
    $('saveClassCourseBindingBtn').disabled = true;
    return;
  }
  $('saveClassCourseBindingBtn').disabled = false;
  if (!state.courses.length) {
    list.innerHTML = '<div class="empty">还没有课堂，请先到题目管理创建课堂。</div>';
    return;
  }
  const selected = new Set(state.boundCourseIds);
  list.innerHTML = state.courses.map((course) => `
    <label class="class-course-option ${selected.has(course.id) ? 'is-selected' : ''}">
      <input type="checkbox" value="${course.id}" ${selected.has(course.id) ? 'checked' : ''}>
      <span class="class-course-option__check" aria-hidden="true">✓</span>
      <span class="class-course-option__body">
        <strong>${escapeHtml(course.title)}</strong>
        <small>${escapeHtml(course.mode || course.templateCode || 'custom')}</small>
      </span>
    </label>
  `).join('');
}

async function saveClassCourseBinding() {
  if (!state.selectedClassId) {
    alert('请先选择班级');
    return;
  }
  const courseIds = Array.from(document.querySelectorAll('#classCourseBindingList input[type="checkbox"]:checked'))
    .map((item) => Number(item.value))
    .filter(Boolean);
  try {
    await api(`/classes/${state.selectedClassId}/courses`, {
      method: 'PUT',
      body: JSON.stringify({ courseIds })
    });
    await loadClassCourseBindings();
    renderClassCourseBinding();
    alert('班级绑定课堂已保存');
  } catch (error) {
    alert(`保存绑定失败：${error.message}`);
  }
}

async function loadStudents() {
  if (!state.selectedClassId) {
    $('studentList').innerHTML = '<div class="empty">请选择班级。</div>';
    return;
  }
  const data = await api(`/classes/${state.selectedClassId}/students`);
  const students = data.students || [];
  if (!students.length) {
    $('studentList').innerHTML = '<div class="empty">当前班级还没有学生。</div>';
    return;
  }
  $('studentList').innerHTML = `
    <table class="student-table">
      <thead>
        <tr>
          <th>姓名</th>
          <th>学号</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        ${students.map((item) => `
          <tr>
            <td>${escapeHtml(item.name)}</td>
            <td>${escapeHtml(item.studentNo || '未填写')}</td>
            <td>
              <button class="danger" data-action="delete-student" data-id="${item.id}">删除</button>
            </td>
          </tr>
        `).join('')}
      </tbody>
    </table>
  `;
}

function renderCourses() {
  const classroomCourseOptions = state.selectedClassId
    ? state.courses
      .filter((item) => state.boundCourseIds.includes(item.id))
      .map((item) => (
        `<option value="${item.id}" ${item.id === state.selectedCourseId ? 'selected' : ''}>${item.title}</option>`
      )).join('')
    : '';
  const questionCourseOptions = state.courses.map((item) => (
    `<option value="${item.id}" ${item.id === state.selectedCourseId ? 'selected' : ''}>${item.title}</option>`
  )).join('');
  $('courseSelect').innerHTML = classroomCourseOptions || '<option value="">请先选择班级</option>';
  $('questionCourseSelect').innerHTML = questionCourseOptions || '<option value="">请先创建课堂</option>';
  $('statsCourseSelect').innerHTML = questionCourseOptions;
}

async function loadSections() {
  if (!state.selectedCourseId) {
    $('questionSectionSelect').innerHTML = '<option value="">请先选择课堂</option>';
    $('questionList').innerHTML = '<div class="empty">请选择课堂。</div>';
    return;
  }
  const data = await api(`/courses/${state.selectedCourseId}/sections`);
  state.sections = data.sections || [];
  if (!state.sections.some((item) => item.id === state.selectedSectionId)) {
    state.selectedSectionId = state.sections[0]?.id || null;
  }
  renderStageButtons();
  $('questionSectionSelect').innerHTML = state.sections.length
    ? state.sections.map((item, index) => (
      `<option value="${item.id}" ${item.id === state.selectedSectionId ? 'selected' : ''}>第${index + 1}部分：${item.title}</option>`
    )).join('')
    : '<option value="">当前课堂还没有部分</option>';
  await loadQuestions();
}

async function loadQuestions() {
  if (!state.selectedSectionId) {
    $('questionList').innerHTML = '<div class="empty">请选择部分。</div>';
    return;
  }
  const section = getSelectedSection();
  const guidanceEligible = getSelectedCourseMode() === 'reflection' &&
    (section?.sectionKey === 'prediction' || section?.sectionKey === 'reflection');
  const [data, guidanceData] = await Promise.all([
    api(`/sections/${state.selectedSectionId}/questions`),
    guidanceEligible ? api(`/sections/${state.selectedSectionId}/ai-guidance`) : Promise.resolve(null)
  ]);
  state.questions = data.questions || [];
  state.aiGuidanceConfig = guidanceData?.aiGuidance || null;
  const lockedReflectionQuiz = isLockedReflectionQuizSection();
  const addQuestionBtn = $('addQuestionBtn');
  if (addQuestionBtn) {
    addQuestionBtn.disabled = lockedReflectionQuiz;
    addQuestionBtn.textContent = lockedReflectionQuiz ? '第三部分固定 5 题' : '➕ 新增题目';
    addQuestionBtn.title = lockedReflectionQuiz ? '反思模式第三部分为固定小测，只能编辑题目内容，不能新增或停用题目。' : '';
  }
  renderList($('questionList'), state.questions, (item) => `
    <div class="item">
      <div class="item-title">
        <span>${renderRichText(item.title)}</span>
        <span class="badge">${item.enabled ? '启用' : '停用'}</span>
      </div>
      <div class="meta">${questionTypeLabel(item.type)} · ${item.score || 0} 分</div>
      ${item.type === 'open_text' ? '<div class="meta"><span class="badge">至少标注一处颜色</span></div>' : ''}
      ${item.rules?.showFor?.length ? `<div class="meta"><span class="badge">显示给：${item.rules.showFor.join('、')}</span></div>` : ''}
      ${renderQuestionOptionsPreview(item)}
      ${item.explanation ? `<div class="meta">解析：${escapeHtml(item.explanation)}</div>` : ''}
      <div class="question-actions">
        <button class="secondary" data-action="edit-question" data-id="${item.id}">编辑</button>
        ${lockedReflectionQuiz
          ? '<span class="badge">固定小测题</span>'
          : `<button class="danger" data-action="delete-question" data-id="${item.id}">停用</button>`}
      </div>
    </div>
  `, '当前部分还没有题目。');
  if (state.aiGuidanceConfig) {
    $('questionList').insertAdjacentHTML('afterbegin', renderAIGuidanceSettingCard(state.aiGuidanceConfig));
  }
  if (lockedReflectionQuiz && state.questions.length !== 5) {
    $('questionList').insertAdjacentHTML('afterbegin', `
      <div class="warning-message">反思模式第三部分应固定为 5 道题，当前为 ${state.questions.length} 道。请检查默认题目数据。</div>
    `);
  }
}

function renderAIGuidanceSettingCard(config) {
  const course = state.courses.find((item) => item.id === state.selectedCourseId);
  const objectiveMissing = !String(course?.learningObjective || '').trim();
  const disabled = config.locked || (objectiveMissing && !config.enabled);
  const stateText = config.enabled ? '已开启' : '未开启';
  return `
    <div class="item ai-guidance-setting-card ${config.enabled ? 'is-enabled' : ''}">
      <div class="item-title">
        <span>${escapeHtml(config.title || 'AI学习指导')}</span>
        <span class="badge">${stateText}</span>
      </div>
      <div class="meta">反思模板固定能力 · 不计入题目数量，不能新增、删除或移动。</div>
      ${objectiveMissing ? '<div class="warning-message">请先在“课堂设置”中填写学习目标，才能开启。</div>' : ''}
      ${config.locked ? '<div class="meta">该部分已有学生答题，开关已锁定。</div>' : ''}
      <div class="question-actions">
        <button type="button" data-action="toggle-ai-guidance" data-id="${state.selectedSectionId}"
          data-enabled="${config.enabled ? '1' : '0'}" ${disabled ? 'disabled' : ''}>
          ${config.enabled ? '关闭AI学习指导' : '开启AI学习指导'}
        </button>
      </div>
    </div>
  `;
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

function isChoiceType(type) {
  return type === 'single_choice' || type === 'multiple_choice';
}

function renderQuestionOptionsPreview(question) {
  if (!isChoiceType(question.type)) return '';
  const options = Array.isArray(question.options) ? question.options : [];
  if (!options.length) return '<div class="meta">暂无选项</div>';
  return `<div class="meta">选项：${options.map(escapeHtml).join(' / ')}</div>`;
}

async function startTeacherApp() {
  if (appStarted) return;
  appStarted = true;
  bindForms();
  bindClicks();
  try {
    await loadAll();
    enhanceCustomSelects(document);
  } catch (error) {
    const mainContainer = document.getElementById('mainContainer');
    if (mainContainer) {
      mainContainer.insertAdjacentHTML('afterbegin', `<div class="panel">${error.message}</div>`);
    }
    throw error;
  }
}

function showPasswordError(message) {
  const errorNode = document.getElementById('passwordError');
  if (errorNode) {
    errorNode.textContent = message;
  }
}

async function checkPassword() {
  const input = document.getElementById('passwordInput');
  const overlay = document.getElementById('passwordOverlay');
  const mainContainer = document.getElementById('mainContainer');
  const value = input?.value.trim() || '';
  if (value !== TEACHER_PASSWORD) {
    showPasswordError('密码错误，请重新输入。');
    input?.focus();
    input?.select?.();
    return false;
  }
  if (overlay) overlay.classList.add('hidden');
  if (mainContainer) mainContainer.classList.remove('hidden');
  try {
    await startTeacherApp();
  } catch (error) {
    showPasswordError(error.message);
    if (overlay) overlay.classList.remove('hidden');
    if (mainContainer) mainContainer.classList.add('hidden');
    appStarted = false;
    return false;
  }
  return true;
}

function normalizeCorrectAnswer(rawValue, type) {
  const value = String(rawValue || '').trim();
  if (!value) return null;
  if (type === 'multiple_choice') {
    return value.split(/[,，、\n]/).map((item) => item.trim()).filter(Boolean);
  }
  return value;
}

function resolveImageUrl(url) {
  if (!url) return '';
  if (/^https?:\/\//i.test(url)) return url;
  if (url.startsWith('/api/')) {
    return `${window.location.protocol}//${window.location.hostname}:8080${url}`;
  }
  return url;
}

function insertTextAtCursor(input, text) {
  if (input?.isContentEditable) {
    insertTextIntoRichEditor(input, text);
    return;
  }
  const start = input.selectionStart ?? input.value.length;
  const end = input.selectionEnd ?? input.value.length;
  input.value = `${input.value.slice(0, start)}${text}${input.value.slice(end)}`;
  const next = start + text.length;
  input.focus();
  input.setSelectionRange?.(next, next);
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

function richTextToEditorHtml(value) {
  const raw = String(value ?? '');
  const imageTokens = [];
  let text = raw.replace(/!\[([^\]]*)\]\(([^)\s]+)\)/g, (_match, alt, url) => {
    const token = `@@IMAGE_${imageTokens.length}@@`;
    imageTokens.push({ token, alt: alt || '图片', url });
    return token;
  });
  text = escapeHtml(text);
  text = text.replace(/\{\{red:([^{}]+)\}\}/g, '<span class="question-rich-red">$1</span>');
  text = text.replace(/\*\*([\s\S]+?)\*\*/g, '<strong>$1</strong>');
  imageTokens.forEach((image) => {
    const html = `<img class="question-inline-image" src="${escapeHtml(resolveImageUrl(image.url))}" alt="${escapeHtml(image.alt)}" data-rich-image-url="${escapeHtml(image.url)}">`;
    text = text.replace(escapeHtml(image.token), html);
  });
  return text.replace(/\n/g, '<br>');
}

function normalizeRichImageUrl(value) {
  const text = String(value || '');
  const marker = '/api/';
  const markerIndex = text.indexOf(marker);
  if (markerIndex >= 0) return text.slice(markerIndex);
  return text;
}

function editorNodeToRichText(node) {
  if (!node) return '';
  if (node.nodeType === Node.TEXT_NODE) return node.textContent || '';
  if (node.nodeType !== Node.ELEMENT_NODE) return '';

  const tag = node.tagName;
  if (tag === 'BR') return '\n';
  if (tag === 'IMG') {
    const alt = node.getAttribute('alt') || '图片';
    const url = node.getAttribute('data-rich-image-url') || normalizeRichImageUrl(node.getAttribute('src') || '');
    return `![${alt}](${url})`;
  }

  const content = Array.from(node.childNodes).map(editorNodeToRichText).join('');
  if (tag === 'STRONG' || tag === 'B') return `**${content}**`;
  if (node.classList?.contains('question-rich-red')) return `{{red:${content}}}`;
  if (tag === 'DIV' || tag === 'P') return `${content}\n`;
  return content;
}

function richEditorToText(editor) {
  return Array.from(editor.childNodes)
    .map(editorNodeToRichText)
    .join('')
    .replace(/\r\n?/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .replace(/\n+$/g, '');
}

function syncRichEditorToSource(editor) {
  const sourceId = editor?.dataset?.richSource;
  if (!sourceId) return;
  const source = $(sourceId);
  if (source) source.value = richEditorToText(editor);
  if (sourceId === 'editQuestionTitle') renderQuestionImagePreview();
}

function setRichEditorValue(editorId, value) {
  const editor = $(editorId);
  if (!editor) return;
  editor.innerHTML = richTextToEditorHtml(value);
  syncRichEditorToSource(editor);
}

function focusRichEditor(editorId) {
  const editor = $(editorId);
  if (!editor) return;
  editor.focus();
}

function selectedRangeInEditor(editor) {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) return null;
  const range = selection.getRangeAt(0);
  return editor.contains(range.commonAncestorContainer) ? range : null;
}

function insertTextIntoRichEditor(editor, text) {
  editor.focus();
  let range = selectedRangeInEditor(editor);
  if (!range) {
    range = document.createRange();
    range.selectNodeContents(editor);
    range.collapse(false);
  }
  range.deleteContents();
  range.insertNode(document.createTextNode(text));
  range.collapse(false);
  const selection = window.getSelection();
  selection.removeAllRanges();
  selection.addRange(range);
  syncRichEditorToSource(editor);
}

function insertLineBreakIntoRichEditor(editor) {
  editor.focus();
  let range = selectedRangeInEditor(editor);
  if (!range) {
    range = document.createRange();
    range.selectNodeContents(editor);
    range.collapse(false);
  }
  range.deleteContents();
  const lineBreak = document.createElement('br');
  range.insertNode(lineBreak);
  range.setStartAfter(lineBreak);
  range.collapse(true);
  const selection = window.getSelection();
  selection.removeAllRanges();
  selection.addRange(range);
  syncRichEditorToSource(editor);
}

function insertImageIntoRichEditor(editor, url, alt = '图片') {
  editor.focus();
  let range = selectedRangeInEditor(editor);
  if (!range) {
    range = document.createRange();
    range.selectNodeContents(editor);
    range.collapse(false);
  }
  const image = document.createElement('img');
  image.className = 'question-inline-image';
  image.src = resolveImageUrl(url);
  image.alt = alt;
  image.dataset.richImageUrl = url;
  range.deleteContents();
  range.insertNode(image);
  range.setStartAfter(image);
  range.collapse(true);
  const selection = window.getSelection();
  selection.removeAllRanges();
  selection.addRange(range);
  syncRichEditorToSource(editor);
}

function applyQuestionRichFormat(targetId, format) {
  const editor = $(targetId);
  if (!editor) return;
  editor.focus();
  let range = selectedRangeInEditor(editor);
  if (!range) {
    range = document.createRange();
    range.selectNodeContents(editor);
    range.collapse(false);
  }
  const selected = range.toString();
  const fallback = format === 'red' ? '标红文字' : '加粗文字';
  const content = selected || fallback;
  const wrapper = document.createElement(format === 'red' ? 'span' : 'strong');
  if (format === 'red') wrapper.className = 'question-rich-red';
  wrapper.textContent = content;
  range.deleteContents();
  range.insertNode(wrapper);
  const nextRange = document.createRange();
  nextRange.selectNodeContents(wrapper);
  const selection = window.getSelection();
  selection.removeAllRanges();
  selection.addRange(nextRange);
  syncRichEditorToSource(editor);
}

function extractImageTokens(text) {
  const pattern = /!\[([^\]]*)\]\(([^)\s]+)\)/g;
  const images = [];
  let match;
  while ((match = pattern.exec(text || '')) !== null) {
    images.push({ alt: match[1] || '图片', url: match[2] });
  }
  return images;
}

function renderQuestionImagePreview() {
  const images = extractImageTokens($('editQuestionTitle').value);
  $('questionImagePreview').innerHTML = images.length ? images.map((image) => `
    <div class="image-preview-card">
      <img src="${escapeHtml(resolveImageUrl(image.url))}" alt="${escapeHtml(image.alt)}">
      <span>${escapeHtml(image.alt)} · ${escapeHtml(image.url)}</span>
    </div>
  `).join('') : '';
}

async function uploadImageForInput(input) {
  const picker = document.createElement('input');
  picker.type = 'file';
  picker.accept = 'image/jpeg,image/png,image/gif,image/webp';
  picker.onchange = async () => {
    const file = picker.files?.[0];
    picker.remove();
    if (!file) return;
    if (file.size > 5 * 1024 * 1024) {
      alert('文件大小超过 5MB 限制');
      return;
    }
    const formData = new FormData();
    formData.append('image', file);
    try {
      const res = await fetch(`${legacyApiBase()}/teacher/upload-image`, {
        method: 'POST',
        body: formData
      });
      const data = await res.json().catch(() => ({ success: false, error: '上传响应解析失败' }));
      if (!res.ok || !data.success) {
        throw new Error(data.error || '上传失败');
      }
      if (input.isContentEditable) {
        insertImageIntoRichEditor(input, data.data.url);
      } else {
        insertTextAtCursor(input, `![图片](${data.data.url})`);
      }
      if (input.id === 'editQuestionTitleEditor' || input.id === 'editQuestionTitle') renderQuestionImagePreview();
    } catch (error) {
      alert(`上传失败：${error.message}`);
    }
  };
  document.body.appendChild(picker);
  picker.click();
}

async function loadClassroom() {
  if (!state.selectedCourseId || !state.selectedClassId) return;
  const data = await api(`/classrooms/${state.selectedCourseId}/${state.selectedClassId}`);
  const classroom = data.classroom || {};
  state.classroom = classroom;
  $('blackboard').value = classroom.blackboard || '';
  renderClassroomSummary();
  await touchCurrentClassroom();
  await loadCompletionStats();
}

async function touchCurrentClassroom() {
  if (!state.selectedCourseId || !state.selectedClassId || !state.classroom) return;
  try {
    await api(`/classrooms/${state.selectedCourseId}/${state.selectedClassId}/stage`, {
      method: 'POST',
      body: JSON.stringify({
        stageId: Number(state.classroom.stageId || 0)
      })
    });
  } catch (error) {
    console.warn('同步课堂活跃状态失败', error);
  }
}

function renderClassroomSummary() {
  const classroom = state.classroom;
  if (!classroom) {
    $('classroomSummary').innerHTML = '选择课程和班级后显示课堂状态。';
    return;
  }
  $('currentStageText').textContent = stageLabelBySectionId(classroom.stageId);
  $('classroomSummary').innerHTML = `
    <div class="item">
      <div class="item-title">
        <span>当前开启：${stageLabelBySectionId(classroom.stageId)}</span>
        <span class="badge">ID ${classroom.stageId || 0}</span>
      </div>
      <div class="meta">${classroom.blackboard ? '课堂黑板已填写' : '课堂黑板为空'}</div>
    </div>
  `;
  renderStageButtons();
}

function renderStageButtons() {
  if (!$('stageButtons')) return;
  const currentStage = Number(state.classroom?.stageId || 0);
  const buttons = [
    { label: '切换到准备环节', stageId: 0, active: currentStage === 0 },
    ...state.sections.slice(0, 4).map((section, index) => ({
      label: `切换到第${index + 1}部分`,
      stageId: section.id,
      active: currentStage === section.id,
      disabled: !section.enabled
    }))
  ];
  $('stageButtons').innerHTML = buttons.map((item) => `
    <button type="button" class="${item.active ? '' : 'secondary'}" data-action="set-stage" data-id="${item.stageId}" ${item.disabled ? 'disabled' : ''}>${item.label}</button>
  `).join('');
}

async function setStage(stageId) {
  if (!state.selectedCourseId || !state.selectedClassId) {
    alert('请先选择课程和班级');
    return;
  }
  try {
    await api(`/classrooms/${state.selectedCourseId}/${state.selectedClassId}/stage`, {
      method: 'POST',
      body: JSON.stringify({ stageId: Number(stageId || 0) })
    });
    await loadClassroom();
    await loadStats();
  } catch (error) {
    alert(`切换阶段失败：${error.message}`);
  }
}

async function loadCompletionStats() {
  if (!state.selectedCourseId || !state.selectedClassId) {
    $('courseCompletionStats').innerHTML = '';
    return;
  }
  const data = await api(`/stats?courseId=${state.selectedCourseId}&classId=${state.selectedClassId}`);
  const stats = data.stats || {};
  const parts = stats.parts || [];
  const total = stats.totalClass || stats.studentCount || 0;
  const notSubmitted = stats.notSubmittedStudents || [];
  let html = '<h4>📊 当前班级完成情况</h4>';
  html += `<p><strong>班级总人数：</strong>${total}人，已答题：${stats.submittedCount || 0}人，未提交：${notSubmitted.length}人</p>`;
  if (notSubmitted.length > 0) {
    html += `<p style="color:#f56c6c;"><strong>未提交学生：</strong>${notSubmitted.map(escapeHtml).join('、')}</p>`;
  }
  html += '<div style="height:1px;background:#eee;margin:10px 0;"></div>';
  for (let i = 0; i < 4; i += 1) {
    const part = parts[i] || { completed: 0, total };
    const percent = total ? Math.round((part.completed || 0) / total * 100) : 0;
    html += `<p>第${i + 1}部分: ${part.completed || 0}/${total} 人完成 (${percent}%)</p>`;
  }
  $('courseCompletionStats').innerHTML = html;
}

async function loadSectionsForCourse(courseId) {
  if (!courseId) return;
  const data = await api(`/courses/${courseId}/sections`);
  state.sections = data.sections || [];
}

async function loadStats() {
  const courseId = Number($('statsCourseSelect').value || state.selectedCourseId || 0);
  const classId = Number($('statsClassSelect').value || 0);

  if (!courseId) {
    $('statsCards').innerHTML = '<div class="empty">请先选择课堂后再加载统计。</div>';
    $('statsContent').innerHTML = '';
    return;
  }

  const params = new URLSearchParams();
  params.set('courseId', courseId);
  if (classId) params.set('classId', classId);

  const btn = $('refreshStatsBtn');
  if (btn) {
    btn.disabled = true;
    btn.textContent = '加载中...';
  }

  try {
    await loadSectionsForCourse(courseId);
    const data = await api(`/stats?${params.toString()}`);
    state.stats = data.stats || {};
    renderStats();
  } catch (error) {
    $('statsCards').innerHTML = '';
    $('statsContent').innerHTML = `<div class="empty" style="color:#f56c6c;">加载统计失败：${escapeHtml(error.message)}</div>`;
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = '加载统计';
    }
  }
}

function getStatsCourseMode() {
  const courseId = Number($('statsCourseSelect').value || state.selectedCourseId || 0);
  const course = state.courses.find((c) => c.id === courseId);
  return course ? course.mode || course.templateCode : '';
}

function getSelectedCourseMode() {
  const course = state.courses.find((c) => c.id === state.selectedCourseId);
  return course ? course.mode || course.templateCode : '';
}

function getSelectedSection() {
  return state.sections.find((section) => section.id === state.selectedSectionId) || null;
}

function isLockedReflectionQuizSection() {
  const section = getSelectedSection();
  return getSelectedCourseMode() === 'reflection' && section?.type === 'quiz' && section.fixed;
}

function renderStats() {
  const stats = state.stats || {};
  const cards = [
    ['班级总人数', stats.totalClass || stats.studentCount || 0],
    ['已答题', stats.submittedCount || 0],
    ['未提交', (stats.notSubmittedStudents || []).length],
    ['题目数', stats.questionCount || 0],
    ['部分数', stats.sectionCount || 0]
  ];
  $('statsCards').innerHTML = cards.map(([label, value]) => `
    <div class="stat-card">
      <strong>${value}</strong>
      <span class="meta">${label}</span>
    </div>
  `).join('');

  const parts = stats.parts || [];
  const total = stats.totalClass || stats.studentCount || 0;
  const students = Array.isArray(stats.students) ? stats.students : [];
  const notSubmitted = stats.notSubmittedStudents || [];
  state.currentStatsStudents = students;
  const part1Stats = stats.part1Stats || {};
  const part2Stats = stats.part2Stats || {};
  const part3Stats = stats.part3Stats || {};
  const predictionSummary = stats.predictionSummary || {};
  const optionDistributions = Array.isArray(stats.optionDistributions) ? stats.optionDistributions : [];
  const reflectionMode = getSelectedCourseMode() === 'reflection';

  // Build section type lookup
  const sectionById = {};
  (state.sections || []).forEach((s) => { sectionById[s.id] = s; });

  // Dynamic part completion: use actual parts array, not hardcoded 4
  const partCompletionHtml = parts.length
    ? parts.map((part) => {
        const percent = total ? Math.round((part.completed || 0) / total * 100) : 0;
        return `<p>${escapeHtml(part.title)}：${part.completed || 0}/${total} 人完成 (${percent}%)</p>`;
      }).join('')
    : '<p class="empty">暂无部分数据</p>';

  // Dynamic detailed stats: only show parts with actual answer data
  const detailedStatsHtml = (() => {
    if (!parts.length) return '';

    const sections = [];
    parts.forEach((part) => {
      const section = sectionById[part.sectionId];
      const sectionType = section ? section.type : '';
      const hasCompletions = (part.completed || 0) > 0;
      const sectionDistributions = optionDistributions.filter(
        (item) => Number(item.sectionId) === Number(part.sectionId)
      );
      const questionDistributionsHtml = renderQuestionOptionDistributions(
        sectionDistributions
      );

      if (sectionType === 'prediction') {
        const predictionDistribution = Object.entries(part1Stats.predictionScoreDistribution || {});
        const completionEntries = Object.entries(part1Stats.learningMethodsDistribution || {})
          .filter(([label]) => label === '开放题（已完成）' || label === 'AI 对话题（已完成）');
        const hasPredictionData = predictionDistribution.length > 0;
        const hasOptionData = Boolean(questionDistributionsHtml);

        if (!hasCompletions && !hasPredictionData && !hasOptionData) {
          sections.push(`
            <div class="stats-section">
              ${renderStatsPartHeader(part)}
              <p class="empty">该部分暂无学生作答</p>
            </div>
          `);
          return;
        }

        sections.push(`
          <div class="stats-section">
            ${renderStatsPartHeader(part)}
            ${hasPredictionData ? `
              <p><strong>预测分布：</strong></p>
              ${renderScoreDistribution(predictionDistribution, '暂无预测分数据', '分')}
            ` : ''}
            ${questionDistributionsHtml}
            ${renderCompletionCounts(completionEntries)}
          </div>
        `);
        return;
      }

      if (sectionType === 'learning') {
        const hasOptionData = Boolean(questionDistributionsHtml);
        const filledCount = part2Stats.filledCount || 0;

        if (!hasCompletions && filledCount === 0 && !hasOptionData) {
          sections.push(`
            <div class="stats-section">
              ${renderStatsPartHeader(part)}
              <p class="empty">该部分暂无学生作答</p>
            </div>
          `);
          return;
        }

        sections.push(`
          <div class="stats-section">
            ${renderStatsPartHeader(part)}
            <p><strong>填写人数：</strong>${filledCount}/${part2Stats.totalCount || total}人</p>
            ${questionDistributionsHtml}
          </div>
        `);
        return;
      }

      if (sectionType === 'quiz') {
        const scoreDistribution = Object.entries(part3Stats.scoreDistribution || {}).sort((a, b) => Number(a[0]) - Number(b[0]));
        const questionCorrectRates = Array.isArray(part3Stats.questionCorrectRate) ? part3Stats.questionCorrectRate : [];
        const hasScoreData = scoreDistribution.length > 0;
        const hasQuestionRates = questionCorrectRates.length > 0;
        const quizQuestionDistributionsHtml = renderQuestionOptionDistributions(sectionDistributions, {
          questionCorrectRates
        });

        if (!hasCompletions && !hasScoreData && !hasQuestionRates) {
          sections.push(`
            <div class="stats-section">
              ${renderStatsPartHeader(part)}
              <p class="empty">该部分暂无学生作答</p>
            </div>
          `);
          return;
        }

        sections.push(`
          <div class="stats-section">
            ${renderStatsPartHeader(part)}
            ${hasScoreData ? `
              <p><strong>得分分布：</strong></p>
              ${renderScoreDistribution(scoreDistribution, '暂无小测得分数据', '分')}
            ` : ''}
            ${quizQuestionDistributionsHtml}
          </div>
        `);
        return;
      }

      if (sectionType === 'reflection' && reflectionMode) {
        const comparisonHtml = renderPredictionComparison(predictionSummary);
        sections.push(`
          <div class="stats-section">
            ${renderStatsPartHeader(part)}
            <p><strong>预测与实际结果对比：</strong></p>
            ${comparisonHtml}
            ${questionDistributionsHtml}
          </div>
        `);
        return;
      }

      // Fallback: other section types (custom/free mode)
      if (!hasCompletions) {
        sections.push(`
          <div class="stats-section">
            ${renderStatsPartHeader(part)}
            <p class="empty">该部分暂无学生作答</p>
          </div>
        `);
        return;
      }

      sections.push(`
        <div class="stats-section">
          ${renderStatsPartHeader(part)}
          ${questionDistributionsHtml || '<p class="empty">暂无该部分详细统计数据</p>'}
        </div>
      `);
    });

    return sections.join('');
  })();

  $('statsContent').innerHTML = `
    <div class="stats-section">
      <h4>各部分完成情况</h4>
      <p><strong>班级总人数：</strong>${total}人，已答题：${stats.submittedCount || 0}人，未提交：${notSubmitted.length}人</p>
      ${notSubmitted.length ? `<p style="color:#f56c6c;"><strong>未提交学生：</strong>${notSubmitted.map(escapeHtml).join('、')}</p>` : ''}
      ${partCompletionHtml}
    </div>
    ${detailedStatsHtml}
    ${renderAIGuidanceStats(stats.aiGuidanceStats)}
    <div class="stats-section">
      <h4>学生完成情况</h4>
      <p style="color:#5a85a8; margin-bottom: 12px;">教师评分会作为最终实际分优先展示，并同步到学生历史页。</p>
      ${(() => {
        const courseMode = getStatsCourseMode();
        const isReflection = courseMode === 'reflection';
        const colSpan = isReflection ? 11 : 8;

        const headerRow = `
          <tr>
            <th>班级</th>
            <th>姓名</th>
            <th>完成状态</th>
            ${isReflection ? '<th>预测分</th><th>小测分</th><th>重测分</th>' : ''}
            <th>教师评分</th>
            <th>实际分</th>
            <th>实际来源</th>
            <th>备注</th>
            <th>操作</th>
          </tr>
        `;

        const bodyHtml = students.length
          ? students.map((s, index) => {
              const hasSubmission = Number(s.submissionId || 0) > 0;
              return `
              <tr>
                <td>${escapeHtml(s.className)}</td>
                <td>${escapeHtml(s.studentName)}</td>
                <td>${escapeHtml(s.statusText)}</td>
                ${isReflection ? `<td>${s.predictedScore ?? '未填写'}</td><td>${s.quizScore ?? '待评分'}</td><td>${s.retakeScore ?? ''}</td>` : ''}
                <td>
                  <input id="teacher-score-${index}" type="number" min="0" max="5" step="1" value="${s.teacherScore ?? ''}" class="stats-score-input" placeholder="可留空">
                </td>
                <td>${s.actualScore ?? '待评分'}</td>
                <td><span class="badge">${escapeHtml(formatScoreSourceLabel(s.actualScoreSource))}</span></td>
                <td>
                  <input id="teacher-score-note-${index}" type="text" value="${escapeHtml(s.teacherScoreNote || '')}" class="stats-note-input" placeholder="评分备注">
                </td>
                <td>
                  <button class="secondary" type="button" data-action="save-score" data-id="${index}">保存评分</button>
                  <button type="button" data-action="student-detail" data-id="${index}" ${hasSubmission ? '' : 'disabled'}>查看详情</button>
                </td>
              </tr>
            `; }).join('')
          : `<tr><td colspan="${colSpan}">暂无学生提交数据</td></tr>`;

        return `
          <table class="stats-table">
            <thead>${headerRow}</thead>
            <tbody>${bodyHtml}</tbody>
          </table>
        `;
      })()}
    </div>
  `;
}

function renderScoreDistribution(entries, emptyText, suffix = '') {
  if (!entries || entries.length === 0) {
    return `<div class="empty">${emptyText}</div>`;
  }
  const normalized = entries.map(([key, value]) => [key, Number(value) || 0]);
  const total = normalized.reduce((sum, [, value]) => sum + value, 0);
  const max = Math.max(...normalized.map(([, value]) => value), 0);
  return `
    <div class="stats-bar-chart stats-bar-chart--score">
      ${normalized.map(([key, value]) => {
        const percentage = total ? Math.round(value * 100 / total) : 0;
        return `
        <div class="stats-bar-row${value === max && max > 0 ? ' stats-bar-row--peak' : ''}">
          <div class="stats-bar-row__top">
            <span class="stats-bar-row__label">${escapeHtml(String(key))}${suffix}</span>
            <span class="stats-bar-row__metrics"><strong>${value}</strong> 人 <em>${percentage}%</em></span>
          </div>
          <div class="stats-bar-row__track"><span style="width:${percentage}%"></span></div>
        </div>
      `; }).join('')}
    </div>
  `;
}

function renderStatsPartHeader(part) {
  return `
    <div class="stats-part-header">
      <h4>${escapeHtml(part.title || '')}</h4>
      <span class="stats-part-header__count">作答总人数 <strong>${Number(part.completed) || 0}</strong> 人</span>
    </div>
  `;
}

function renderQuestionOptionDistributions(distributions, { questionCorrectRates = [] } = {}) {
  if (!distributions?.length) return '';
  const typeLabels = { single_choice: '单选题', multiple_choice: '多选题' };
  const correctRateByQuestion = new Map(
    questionCorrectRates.map((item) => [Number(item.questionId), item])
  );
  return `
    <div class="stats-question-distributions">
      <p><strong>各题选项分布：</strong></p>
      ${distributions.map((item) => {
        const options = Array.isArray(item.options) ? item.options : [];
        const maxCount = Math.max(...options.map((option) => Number(option.count) || 0), 0);
        const correctRate = correctRateByQuestion.get(Number(item.questionId));
        const showAnswerColors = Boolean(correctRate) && options.some((option) => option.isCorrect);
        return `
          <article class="stats-option-chart${correctRate ? ' stats-option-chart--quiz' : ''}">
            <header class="stats-option-chart__header">
              <div>
                <span class="stats-option-chart__number">第${item.sortOrder || '-'}题</span>
                <span class="stats-option-chart__type">${typeLabels[item.questionType] || escapeHtml(item.questionType || '选择题')}</span>
              </div>
              <span class="stats-option-chart__respondents">${Number(item.respondentCount) || 0} 人作答</span>
            </header>
            <div class="stats-option-chart__title">${renderRichText(item.questionText || '')}</div>
            ${correctRate ? renderQuestionCorrectSummary(correctRate) : ''}
            <div class="stats-bar-chart">
              ${options.map((option) => {
                const count = Number(option.count) || 0;
                const percentage = Number(option.percentage) || 0;
                const answerClass = showAnswerColors
                  ? (option.isCorrect ? ' stats-bar-row--correct-answer' : ' stats-bar-row--wrong-answer')
                  : (count === maxCount && maxCount > 0 ? ' stats-bar-row--peak' : '');
                return `
                  <div class="stats-bar-row${answerClass}">
                    <div class="stats-bar-row__top">
                      <span class="stats-bar-row__label">${renderRichText(option.label || '-')}</span>
                      <span class="stats-bar-row__metrics"><strong>${count}</strong> 人 <em>${percentage}%</em></span>
                    </div>
                    <div class="stats-bar-row__track"><span style="width:${Math.max(0, Math.min(100, percentage))}%"></span></div>
                  </div>
                `;
              }).join('')}
            </div>
          </article>
        `;
      }).join('')}
    </div>
  `;
}

function renderCompletionCounts(entries) {
  if (!entries?.length) return '';
  return `
    <div class="stats-completion-summary">
      ${entries.map(([label, count]) => `
        <div>
          <span>${escapeHtml(label)}</span>
          <strong>${Number(count) || 0} 人</strong>
        </div>
      `).join('')}
    </div>
  `;
}

function renderQuestionCorrectSummary(item) {
  const total = Math.max(0, Number(item.totalCount) || 0);
  const correct = Math.max(0, Math.min(total, Number(item.correctCount) || 0));
  const incorrect = Math.max(0, total - correct);
  const correctRate = total ? Math.round(correct * 100 / total) : 0;
  return `
    <div class="stats-correct-summary">
      <div class="stats-correct-summary__counts">
        <span class="stats-correct-count stats-correct-count--right">✓ 答对 <strong>${correct}</strong> 人</span>
        <span class="stats-correct-count stats-correct-count--wrong">✕ 答错 <strong>${incorrect}</strong> 人</span>
        <span class="stats-correct-summary__rate">正确率 <strong>${correctRate}%</strong></span>
      </div>
      <div class="stats-correct-summary__track" aria-label="正确率 ${correctRate}%">
        <span style="width:${correctRate}%"></span>
      </div>
    </div>
  `;
}

function renderPredictionComparison(summary) {
  const rows = [
    { key: 'correct', label: '猜中', className: 'correct' },
    { key: 'high', label: '猜高', className: 'high' },
    { key: 'low', label: '猜低', className: 'low' }
  ];
  const effectiveTotal = rows.reduce((sum, row) => sum + (Number(summary?.[row.key]) || 0), 0);
  const unknown = Number(summary?.unknown) || 0;
  if (effectiveTotal === 0) {
    return `
      <div class="empty">暂无预测与实际分数对比数据</div>
      ${unknown > 0 ? `<p class="stats-comparison-unknown">未形成对比 ${unknown} 人</p>` : ''}
    `;
  }
  return `
    <div class="stats-comparison-chart">
      ${rows.map((row) => {
        const count = Number(summary?.[row.key]) || 0;
        const percentage = Math.round(count * 100 / effectiveTotal);
        return `
          <div class="stats-comparison-row stats-comparison-row--${row.className}">
            <div class="stats-bar-row__top">
              <span class="stats-bar-row__label">${row.label}</span>
              <span class="stats-bar-row__metrics"><strong>${count}</strong> 人 <em>${percentage}%</em></span>
            </div>
            <div class="stats-comparison-row__track"><span style="width:${percentage}%"></span></div>
          </div>
        `;
      }).join('')}
    </div>
    ${unknown > 0 ? `<p class="stats-comparison-unknown">未形成对比 ${unknown} 人</p>` : ''}
  `;
}

function renderAIGuidanceStats(stats) {
  const phases = [
    ['AI学习计划', stats?.plan],
    ['AI学习评价与反思', stats?.evaluation]
  ].filter(([, item]) => item?.enabled);
  if (!phases.length) return '';
  return `
    <div class="stats-section">
      <h4>AI学习指导</h4>
      <div class="stat-grid ai-guidance-stat-grid">
        ${phases.map(([title, item]) => `
          <div class="stat-card">
            <span>${escapeHtml(title)}</span>
            <strong>${item.completedCount || 0} 人完成</strong>
            <small>已生成 ${item.generatedCount || 0} · 跳过 ${item.skippedCount || 0} · 失败 ${item.failedCount || 0} · 追问 ${item.followUpRounds || 0} 轮</small>
          </div>
        `).join('')}
      </div>
    </div>
  `;
}

function formatScoreSourceLabel(value) {
  if (value === 'teacher') return '教师评分';
  if (value === 'quiz') return '小测';
  if (value === 'none') return '无';
  return value || '-';
}

function renderDetailText(value) {
  return escapeHtml(String(value ?? '')).replace(/\n/g, '<br>');
}

function renderChatMessages(messages = []) {
  const normalized = (Array.isArray(messages) ? messages : [])
    .map((item) => ({
      role: String(item?.role || '').trim(),
      content: String(item?.content || '').trim()
    }))
    .filter((item) => item.content && (item.role === 'user' || item.role === 'assistant'));
  if (!normalized.length) return '<div class="empty">暂无 AI 对话记录。</div>';
  return `
    <div class="ai-chat-transcript">
      ${normalized.map((item) => `
        <div class="ai-chat-message ai-chat-message--${item.role === 'user' ? 'user' : 'assistant'}">
          <div class="ai-chat-message__role">${item.role === 'user' ? '学生' : 'AI 学习助手'}</div>
          <div class="ai-chat-message__content">${renderMarkdown(item.content)}</div>
        </div>
      `).join('')}
    </div>
  `;
}

function openStudentDetailModal() {
  $('studentDetailModal').classList.remove('hidden');
}

function closeStudentDetailModal() {
  $('studentDetailModal').classList.add('hidden');
  $('studentDetailContent').innerHTML = '';
}

// 学生作答预览卡片：点击展开全文
function renderAnswerCard(contentHtml, previewClass = '') {
  const isChat = previewClass.split(/\s+/).includes('answer-card--chat');
  return `
    <div class="answer-card ${previewClass}">
      <div class="answer-card-preview">
        ${contentHtml}
      </div>
      <div class="answer-card-fade"></div>
      <div class="answer-card-hint">${isChat ? '点击查看完整对话' : '点击查看全文'}</div>
    </div>
    <div class="answer-card-full" style="display:none">${contentHtml}</div>
  `;
}

function showFullAnswerModal(cardEl) {
  const fullContent = cardEl.nextElementSibling?.innerHTML;
  if (!fullContent) return;
  $('fullAnswerBody').innerHTML = fullContent;
  $('fullAnswerModal').classList.remove('hidden');
}

function closeFullAnswerModal() {
  $('fullAnswerModal').classList.add('hidden');
  $('fullAnswerBody').innerHTML = '';
}

function renderStudentDetail(detail) {
  const summary = detail || {};
  const actualSource = formatScoreSourceLabel(summary.actualScoreSource);
  const sections = Array.isArray(summary.sections) ? summary.sections : [];
  const courseMode = getStatsCourseMode();
  const isReflection = courseMode === 'reflection';
  const guidanceSessions = Array.isArray(summary.aiGuidance) ? summary.aiGuidance : [];

  // 检查是否有任何答题记录
  const hasAnyAttempts = sections.some((section) =>
    Array.isArray(section.attempts) && section.attempts.length > 0
  );

  const sectionHtml = sections.length
    ? sections.map((section, sectionIndex) => `
      <div class="student-detail-section">
        <div class="item-title">
          <span>第${sectionIndex + 1}部分：${escapeHtml(section.title || '')}</span>
          <span class="badge">${section.completed ? '已完成' : '未完成'}</span>
        </div>
        <div class="meta">类型：${escapeHtml(section.type || '-')}${section.fixed ? ' · 固定部分' : ''}</div>
        ${Array.isArray(section.attempts) && section.attempts.length ? section.attempts.map((attempt) => `
          <div class="student-detail-attempt">
            <div class="item-title">
              <span>第 ${attempt.attemptNo || 1} 次提交</span>
              <span class="badge">${attempt.score ?? '未评分'}</span>
            </div>
            <div class="meta">提交时间：${escapeHtml(attempt.submittedAt || '-')}</div>
            <div class="student-detail-question-list">
              ${(Array.isArray(attempt.questions) ? attempt.questions : []).map((question, qIndex) => question.questionType === 'ai_chat' ? `
                <div class="student-detail-question student-detail-question--correct">
                  <div class="item-title">
                    <span>${qIndex + 1}. ${renderRichText(question.questionText || '')}</span>
                    <span class="badge">AI 对话</span>
                  </div>
                  <div class="meta">题型：AI 对话题 · 轮次：${(question.chatMessages || []).filter((item) => item.role === 'user').length}</div>
                  ${renderAnswerCard(renderChatMessages(question.chatMessages || []), 'answer-card--chat')}
                </div>
              ` : question.questionType === 'open_text' ? `
                <div class="student-detail-question student-detail-question--open-text">
                  <div class="item-title">
                    <span>${qIndex + 1}. ${renderRichText(question.questionText || '')}</span>
                    <span class="badge">颜色标注开放题</span>
                  </div>
                  <div class="meta">已保留学生提交时的颜色标注</div>
                  ${renderAnswerCard(`<strong>学生作答</strong><div class="annotated-answer-content">${renderAnnotatedAnswer(question.answer)}</div>`, 'answer-card--open-text')}
                  ${question.explanation ? `<div class="open-text-reference"><strong>参考解析</strong><div>${renderDetailText(question.explanation)}</div></div>` : ''}
                </div>
              ` : `
                <div class="student-detail-question ${question.isCorrect ? 'student-detail-question--correct' : 'student-detail-question--wrong'}">
                  <div class="item-title">
                    <span>${qIndex + 1}. ${renderRichText(question.questionText || '')}</span>
                    <span class="badge">${question.isCorrect ? '正确' : '错误'}</span>
                  </div>
                  <div class="meta">题型：${escapeHtml(question.questionType || '-')} · 分值：${question.score ?? 0}</div>
                  <p><strong>我的答案：</strong><span>${renderDetailText(question.answer || '未提交')}</span></p>
                  <p><strong>正确答案：</strong><span>${renderDetailText(question.correctAnswer || '-')}</span></p>
                  ${question.explanation ? `<p><strong>解析：</strong><span>${renderDetailText(question.explanation)}</span></p>` : ''}
                </div>
              `).join('')}
            </div>
          </div>
        `).join('') : '<div class="empty">该部分暂时没有答题记录。</div>'}
      </div>
    `).join('')
    : '<div class="empty">暂无详细答题记录。</div>';

  $('studentDetailTitle').textContent = `${summary.className || ''} · ${summary.studentName || ''} 答题详情`;
  $('studentDetailContent').innerHTML = `
    <div class="stats-section">
      <h4>基础信息</h4>
      <p><strong>课堂：</strong>${escapeHtml(summary.courseTitle || '-')}</p>
      <p><strong>班级：</strong>${escapeHtml(summary.className || '-')}</p>
      <p><strong>学生：</strong>${escapeHtml(summary.studentName || '-')}</p>
      <p><strong>完成状态：</strong>${escapeHtml(summary.statusText || summary.status || '-')}</p>
      ${isReflection ? `
        <p><strong>预测分：</strong>${summary.predictedScore ?? '未填写'}</p>
        <p><strong>小测分：</strong>${summary.quizScore ?? '待评分'}</p>
        <p><strong>重测分：</strong>${summary.retakeScore ?? ''}</p>
      ` : ''}
      <p><strong>教师评分：</strong>${summary.teacherScore ?? '未评分'}</p>
      <p><strong>实际分：</strong>${summary.actualScore ?? '待评分'}（${escapeHtml(actualSource)}）</p>
      ${summary.teacherScoreNote ? `<p><strong>评分备注：</strong>${renderDetailText(summary.teacherScoreNote)}</p>` : ''}
      <p><strong>提交时间：</strong>${escapeHtml(summary.startedAt || '-')} ${summary.completedAt ? `｜完成时间：${escapeHtml(summary.completedAt)}` : ''}</p>
    </div>
    ${guidanceSessions.length ? `
      <div class="stats-section">
        <h4>AI学习指导</h4>
        <div class="ai-guidance-detail-grid">
          ${guidanceSessions.map((session) => {
            const messages = Array.isArray(session.messages) ? session.messages : [];
            const initial = messages.find((item) => item.role === 'assistant');
            const full = renderChatMessages(messages);
            return `
              <div class="student-detail-question student-detail-question--correct">
                <div class="item-title">
                  <span>${escapeHtml(session.title || 'AI学习指导')}</span>
                  <span class="badge">${escapeHtml(session.status || '-')}</span>
                </div>
                <div class="meta">学生追问 ${session.followUpRounds || 0} 轮</div>
                <div class="answer-card answer-card--chat">
                  <div class="answer-card-preview">
                    ${initial ? renderMarkdown(initial.content) : '<div class="empty">暂无生成内容</div>'}
                  </div>
                  <div class="answer-card-fade"></div>
                  <div class="answer-card-hint">点击查看完整指导与对话</div>
                </div>
                <div class="answer-card-full" style="display:none">${full}</div>
              </div>
            `;
          }).join('')}
        </div>
      </div>
    ` : ''}
    ${hasAnyAttempts ? sectionHtml : '<div class="stats-section"><p class="empty">该学生尚未作答任何内容</p></div>'}
  `;
  openStudentDetailModal();
}

async function loadStudentDetailBySubmissionId(submissionId) {
  if (!submissionId) {
    alert('缺少提交记录');
    return;
  }
  $('studentDetailContent').innerHTML = '<div class="empty">正在加载学生详情...</div>';
  openStudentDetailModal();
  try {
    const data = await api(`/stats/student-detail?submissionId=${encodeURIComponent(submissionId)}`);
    renderStudentDetail(data.detail || {});
  } catch (error) {
    $('studentDetailContent').innerHTML = `<div class="empty">加载学生详情失败：${escapeHtml(error.message)}</div>`;
  }
}

async function saveScoreByIndex(index) {
  const student = state.currentStatsStudents[index];
  if (!student) {
    alert('缺少学生信息');
    return;
  }
  const scoreInput = document.getElementById(`teacher-score-${index}`);
  const noteInput = document.getElementById(`teacher-score-note-${index}`);
  const rawScore = scoreInput ? scoreInput.value.trim() : '';
  let teacherScore = null;
  if (rawScore !== '') {
    teacherScore = Number(rawScore);
    if (!Number.isInteger(teacherScore) || teacherScore < 0 || teacherScore > 5) {
      alert('教师评分请输入 0-5 的整数，留空则表示清空评分');
      return;
    }
  }
  try {
    await api('/stats/manual-score', {
      method: 'POST',
      body: JSON.stringify({
        submissionId: student.submissionId || 0,
        courseId: Number($('statsCourseSelect').value || state.selectedCourseId || 0),
        classId: student.classId,
        studentId: student.studentId,
        teacherScore,
        note: noteInput ? noteInput.value.trim() : ''
      })
    });
    await loadStats();
    alert('教师评分已保存');
  } catch (error) {
    alert(`保存教师评分失败：${error.message}`);
  }
}

function openCreateClassModal() {
  $('createClassName').value = '';
  $('createClassModal').classList.remove('hidden');
  $('createClassName').focus();
}

function closeCreateClassModal() {
  $('createClassModal').classList.add('hidden');
}

async function submitCreateClass() {
  const name = $('createClassName').value;
  if (!name || !name.trim()) {
    $('createClassName').focus();
    return;
  }
  try {
    await api('/classes', {
      method: 'POST',
      body: JSON.stringify({ name: name.trim() })
    });
    closeCreateClassModal();
    await loadAll();
  } catch (error) {
    alert(`创建班级失败：${error.message}`);
  }
}

function openAddStudentModal() {
  if (!state.selectedClassId) {
    alert('请先选择班级');
    return;
  }
  $('batchStudentInput').value = '';
  $('addStudentModal').classList.remove('hidden');
  $('batchStudentInput').focus();
}

function closeAddStudentModal() {
  $('addStudentModal').classList.add('hidden');
}

function openCreateCourseModal() {
  $('courseForm').reset();
  $('createCourseModal').classList.remove('hidden');
  $('courseTitle').focus();
}

function closeCreateCourseModal() {
  $('createCourseModal').classList.add('hidden');
}

function openCourseSettingsModal() {
  const course = state.courses.find((item) => item.id === state.selectedCourseId);
  if (!course) {
    alert('请先在“部分与题目”中选择课堂');
    return;
  }
  $('courseSettingsTitle').textContent = `当前课堂：${course.title}`;
  $('courseSettingsDescription').value = course.description || '';
  $('courseSettingsLearningObjective').value = course.learningObjective || '';
  $('courseSettingsModal').classList.remove('hidden');
}

function closeCourseSettingsModal() {
  $('courseSettingsModal').classList.add('hidden');
}

function renderDeleteCourseList() {
  const list = $('deleteCourseList');
  if (!list) return;
  if (!state.courses.length) {
    list.innerHTML = '<div class="empty">当前还没有课堂。</div>';
    return;
  }
  list.innerHTML = state.courses.map((course) => `
    <label class="delete-course-option ${course.id === state.selectedCourseId ? 'is-current' : ''}">
      <input type="checkbox" value="${course.id}">
      <span class="delete-course-option__check" aria-hidden="true">✓</span>
      <span class="delete-course-option__body">
        <strong>${escapeHtml(course.title)}</strong>
        <small>${escapeHtml(course.mode || course.templateCode || 'custom')}${course.id === state.selectedCourseId ? ' · 当前正在编辑' : ''}</small>
      </span>
    </label>
  `).join('');
}

function openDeleteCourseModal() {
  renderDeleteCourseList();
  $('deleteCourseModal').classList.remove('hidden');
}

function closeDeleteCourseModal() {
  $('deleteCourseModal').classList.add('hidden');
}

function openCloneCourseModal() {
  // Populate course select
  const options = state.courses.map((c) => `<option value="${c.id}">${escapeHtml(c.title)}</option>`).join('');
  $('cloneCourseSelect').innerHTML = options;
  // Pre-fill title with "——副本" suffix
  updateCloneCourseTitle();
  $('cloneCourseModal').classList.remove('hidden');
}

function closeCloneCourseModal() {
  $('cloneCourseModal').classList.add('hidden');
}

function updateCloneCourseTitle() {
  const select = $('cloneCourseSelect');
  if (!select || !select.value) {
    $('cloneCourseTitle').value = '';
    return;
  }
  const course = state.courses.find((c) => c.id === Number(select.value));
  $('cloneCourseTitle').value = course ? `${course.title}——副本` : '';
}

async function submitCloneCourse() {
  const courseID = Number($('cloneCourseSelect').value);
  const title = $('cloneCourseTitle').value.trim();
  if (!courseID) { alert('请选择要复制的课堂'); return; }
  if (!title) { alert('请输入新课堂名称'); return; }

  const btn = $('confirmCloneCourseBtn');
  btn.disabled = true;
  btn.textContent = '复制中...';
  try {
    const data = await api(`/courses/${courseID}/clone`, {
      method: 'POST',
      body: JSON.stringify({ title })
    });
    alert(`复制成功！新课堂ID：${data.id}`);
    closeCloneCourseModal();
    await loadAll();
  } catch (error) {
    alert(`复制失败：${error.message}`);
  } finally {
    btn.disabled = false;
    btn.textContent = '确认复制';
  }
}

async function submitDeleteCourses() {
  const ids = Array.from(document.querySelectorAll('#deleteCourseList input[type="checkbox"]:checked'))
    .map((input) => Number(input.value))
    .filter(Boolean);
  if (!ids.length) {
    alert('请先勾选要删除的课堂');
    return;
  }
  const titles = state.courses
    .filter((course) => ids.includes(course.id))
    .map((course) => course.title)
    .join('、');
  if (!confirm(`确定删除以下 ${ids.length} 个课堂吗？\n${titles}`)) return;

  const button = $('confirmDeleteCourseBtn');
  button?.setAttribute('disabled', 'disabled');
  if (button) button.textContent = '删除中...';
  try {
    for (const id of ids) {
      await api(`/courses/${id}`, { method: 'DELETE' });
    }
    if (ids.includes(state.selectedCourseId)) {
      state.selectedCourseId = null;
      state.selectedSectionId = null;
    }
    closeDeleteCourseModal();
    await loadAll();
    alert('课堂删除成功');
  } catch (error) {
    alert(`删除课堂失败：${error.message}`);
  } finally {
    if (button) {
      button.removeAttribute('disabled');
      button.textContent = '确认删除';
    }
  }
}

async function submitBatchStudents() {
  const raw = $('batchStudentInput').value || '';
  const lines = raw.split('\n').map((line) => line.trim()).filter(Boolean);
  if (lines.length === 0) {
    $('batchStudentInput').focus();
    return;
  }

  const students = [];
  for (const line of lines) {
    const parts = line.split(/[\s,，]+/).filter(Boolean);
    if (parts.length === 0) continue;
    const name = parts[0];
    const studentNo = parts.slice(1).join(' ');
    if (!name) continue;
    students.push({ name, studentNo });
  }

  if (students.length === 0) {
    alert('未解析到有效的学生信息');
    $('batchStudentInput').focus();
    return;
  }

  try {
    const result = await api(`/classes/${state.selectedClassId}/students/batch`, {
      method: 'POST',
      body: JSON.stringify({ students })
    });
    closeAddStudentModal();
    await loadStudents();
    await loadAll();
    const count = result.count || 0;
    const skipped = Array.isArray(result.skipped) ? result.skipped : [];
    if (skipped.length > 0) {
      alert(`成功导入 ${count} 人，${skipped.join('、')} 同学已存在，已跳过`);
    } else {
      alert(`成功导入 ${count} 人`);
    }
  } catch (error) {
    alert(`批量导入失败：${error.message}`);
  }
}

function resetQuestionModal() {
  $('editQuestionId').value = '';
  $('editQuestionType').value = 'single_choice';
  setRichEditorValue('editQuestionTitleEditor', '');
  setRichEditorValue('editQuestionDescriptionEditor', '');
  $('editCorrectAnswer').value = '';
  $('editQuestionScore').value = '0';
  $('editQuestionExplanation').value = '';
  // Reset showFor checkboxes
  if ($('showForGuessCorrect')) $('showForGuessCorrect').checked = false;
  if ($('showForGuessHigh')) $('showForGuessHigh').checked = false;
  if ($('showForGuessLow')) $('showForGuessLow').checked = false;
  if ($('editAIChatPresets')) $('editAIChatPresets').value = '';
  $('editOptionsList').innerHTML = '';
  addQuestionOption();
  addQuestionOption();
  updateQuestionTypeFields();
}

function openQuestionModal(question = null) {
  resetQuestionModal();
  if (question) {
    $('questionModalTitle').textContent = '编辑题目';
    $('editQuestionId').value = question.id;
    $('editQuestionType').value = question.type || 'single_choice';
    setRichEditorValue('editQuestionTitleEditor', question.title || '');
    setRichEditorValue('editQuestionDescriptionEditor', question.description || '');
    $('editQuestionScore').value = question.score || 0;
    $('editQuestionExplanation').value = question.explanation || '';
    $('editOptionsList').innerHTML = '';
    const options = Array.isArray(question.options) ? question.options : [];
    if (options.length) {
      options.forEach((option) => addQuestionOption(option));
    } else {
      addQuestionOption();
      addQuestionOption();
    }
    if (Array.isArray(question.correctAnswer)) {
      $('editCorrectAnswer').value = question.correctAnswer.join('，');
    } else if (question.correctAnswer !== null && question.correctAnswer !== undefined) {
      $('editCorrectAnswer').value = String(question.correctAnswer);
    }
    // Restore showFor checkboxes from question rules
    const showFor = question?.rules?.showFor || [];
    if ($('showForGuessCorrect')) $('showForGuessCorrect').checked = showFor.includes('猜中');
    if ($('showForGuessHigh')) $('showForGuessHigh').checked = showFor.includes('猜高');
    if ($('showForGuessLow')) $('showForGuessLow').checked = showFor.includes('猜低');
    // Restore AI chat preset questions
    const presets = question?.rules?.presetQuestions || [];
    if ($('editAIChatPresets') && Array.isArray(presets)) {
      $('editAIChatPresets').value = presets.join('\n');
    }
  } else {
    $('questionModalTitle').textContent = '新增题目';
  }
  updateQuestionTypeFields();
  renderQuestionImagePreview();
  $('questionModal').classList.remove('hidden');
  focusRichEditor('editQuestionTitleEditor');
}

function closeQuestionModal() {
  $('questionModal').classList.add('hidden');
}

function addQuestionOption(value = '') {
  const index = $('editOptionsList').children.length + 1;
  const row = document.createElement('div');
  row.className = 'question-option-row';
  row.innerHTML = `
    <span class="question-option-index">${index}.</span>
    <input class="question-option-input" value="${escapeHtml(value)}" placeholder="请输入选项内容">
    <button type="button" class="secondary" data-action="upload-option-image">上传图片</button>
    <button type="button" class="danger" data-action="remove-option">删除</button>
  `;
  $('editOptionsList').appendChild(row);
}

function addOtherQuestionOption() {
  const inputs = Array.from(document.querySelectorAll('#editOptionsList .question-option-input'));
  const existing = inputs.find((input) => input.value.trim().replace(/_/g, '') === '其它：');
  if (existing) {
    existing.focus();
    existing.select?.();
    return;
  }
  addQuestionOption(OTHER_OPTION_TEXT);
}

function refreshQuestionOptionIndexes() {
  document.querySelectorAll('#editOptionsList .question-option-row').forEach((row, index) => {
    row.querySelector('.question-option-index').textContent = `${index + 1}.`;
  });
}

function updateQuestionTypeFields() {
  const type = $('editQuestionType').value;
  const showChoice = isChoiceType(type);
  $('editOptionsGroup').style.display = showChoice ? 'block' : 'none';
  $('editCorrectAnswerGroup').style.display = (type === 'ai_chat' || type === 'open_text') ? 'none' : 'block';
  $('editOpenTextRulesGroup').classList.toggle('hidden', type !== 'open_text');
  // Show showFor group only for reflection section questions
  const section = getSelectedSection();
  const isReflectionSection = getSelectedCourseMode() === 'reflection' && section?.type === 'reflection';
  if ($('editShowForGroup')) {
    $('editShowForGroup').classList.toggle('hidden', !isReflectionSection);
  }
  // Show AI chat presets only for ai_chat type
  if ($('editAIChatPresetsGroup')) {
    $('editAIChatPresetsGroup').classList.toggle('hidden', type !== 'ai_chat');
  }
}

async function saveQuestionFromModal() {
  if (savingQuestionInFlight) return;
  const sectionId = state.selectedSectionId;
  const questionId = Number($('editQuestionId').value || 0);
  const type = $('editQuestionType').value;
  syncRichEditorToSource($('editQuestionTitleEditor'));
  syncRichEditorToSource($('editQuestionDescriptionEditor'));
  const title = $('editQuestionTitle').value.trim();
  const saveButton = $('saveQuestionBtn');
  if (!sectionId) {
    alert('请先选择部分');
    return;
  }
  if (!title) {
    alert('请填写题目内容');
    focusRichEditor('editQuestionTitleEditor');
    return;
  }

  let options = [];
  if (isChoiceType(type)) {
    options = Array.from(document.querySelectorAll('#editOptionsList .question-option-input'))
      .map((input) => input.value.trim())
      .filter(Boolean);
    if (options.length < 2) {
      alert('选择题至少需要填写 2 个选项');
      return;
    }
  }

  // Build rules: preserve annotation settings for open_text, add showFor for reflection
  let rules = {};
  if (type === 'open_text') {
    rules = { annotationEnabled: true, annotationRequired: true };
  }
  // Add showFor for reflection section questions
  const section = getSelectedSection();
  const isReflectionSection = getSelectedCourseMode() === 'reflection' && section?.type === 'reflection';
  if (isReflectionSection) {
    const showFor = [];
    if ($('showForGuessCorrect')?.checked) showFor.push('猜中');
    if ($('showForGuessHigh')?.checked) showFor.push('猜高');
    if ($('showForGuessLow')?.checked) showFor.push('猜低');
    if (showFor.length > 0) {
      rules.showFor = showFor;
    }
  }
  // Add AI chat preset questions
  if (type === 'ai_chat') {
    const presetsRaw = $('editAIChatPresets')?.value || '';
    const presets = presetsRaw.split('\n').map(s => s.trim()).filter(Boolean).slice(0, 10);
    if (presets.length > 0) {
      rules.presetQuestions = presets;
    }
  }

  const payload = {
    type,
    title,
    description: $('editQuestionDescription').value,
    options,
    correctAnswer: normalizeCorrectAnswer($('editCorrectAnswer').value, type),
    explanation: $('editQuestionExplanation').value,
    score: Number($('editQuestionScore').value || 0),
    rules
  };

  savingQuestionInFlight = true;
  saveButton?.setAttribute('disabled', 'disabled');
  saveButton && (saveButton.textContent = '保存中...');

  try {
    if (questionId) {
      await api(`/questions/${questionId}`, {
        method: 'PUT',
        body: JSON.stringify(payload)
      });
    } else {
      await api(`/sections/${sectionId}/questions`, {
        method: 'POST',
        body: JSON.stringify(payload)
      });
    }
    alert('题目保存成功！');
    closeQuestionModal();
    await loadQuestions();
  } catch (error) {
    alert(questionId ? `保存题目失败：${error.message}` : `创建题目失败：${error.message}`);
  } finally {
    savingQuestionInFlight = false;
    if (saveButton) {
      saveButton.removeAttribute('disabled');
      saveButton.textContent = '保存题目';
    }
  }
}

function bindForms() {
  const $on = (id, event, handler, options) => {
    const el = document.getElementById(id);
    if (!el) {
      console.warn(`[teacher] 绑定事件失败：未找到元素 #${id}`);
      return;
    }
    el.addEventListener(event, handler, options);
  };

  $on('createClassBtn', 'click', () => {
    openCreateClassModal();
  });

  $on('cancelCreateClass', 'click', closeCreateClassModal);
  $on('confirmCreateClass', 'click', submitCreateClass);
  $on('createClassModal', 'click', (event) => {
    if (event.target === $('createClassModal')) closeCreateClassModal();
  });
  $on('createClassName', 'keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      submitCreateClass();
    }
  });

  $on('addStudentBtn', 'click', openAddStudentModal);
  $on('cancelAddStudent', 'click', closeAddStudentModal);
  $on('confirmBatchStudent', 'click', submitBatchStudents);
  $on('addStudentModal', 'click', (event) => {
    if (event.target === $('addStudentModal')) closeAddStudentModal();
  });
  $on('batchStudentInput', 'keydown', (event) => {
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      submitBatchStudents();
    }
  });

  $on('courseForm', 'submit', async (event) => {
    event.preventDefault();
    try {
      await api('/courses', {
        method: 'POST',
        body: JSON.stringify({
          title: $('courseTitle').value,
          description: $('courseDescription').value,
          learningObjective: $('courseLearningObjective').value,
          templateCode: $('courseTemplate').value
        })
      });
      event.target.reset();
      closeCreateCourseModal();
      await loadAll();
      alert('课堂创建成功');
    } catch (error) {
      alert(`创建课堂失败：${error.message}`);
    }
  });

  $on('showCreateCourseBtn', 'click', openCreateCourseModal);
  $on('cancelCreateCourseBtn', 'click', closeCreateCourseModal);
  $on('createCourseModal', 'click', (event) => {
    if (event.target === $('createCourseModal')) closeCreateCourseModal();
  });
  $on('courseSettingsBtn', 'click', openCourseSettingsModal);
  $on('cancelCourseSettingsBtn', 'click', closeCourseSettingsModal);
  $on('courseSettingsModal', 'click', (event) => {
    if (event.target === $('courseSettingsModal')) closeCourseSettingsModal();
  });
  $on('courseSettingsForm', 'submit', async (event) => {
    event.preventDefault();
    if (!state.selectedCourseId) return;
    try {
      await api(`/courses/${state.selectedCourseId}`, {
        method: 'PUT',
        body: JSON.stringify({
          description: $('courseSettingsDescription').value,
          learningObjective: $('courseSettingsLearningObjective').value
        })
      });
      closeCourseSettingsModal();
      await loadAll();
      alert('课堂设置已保存');
    } catch (error) {
      alert(`保存课堂设置失败：${error.message}`);
    }
  });
  $on('deleteSelectedCourseBtn', 'click', openDeleteCourseModal);
  $on('cancelDeleteCourseBtn', 'click', closeDeleteCourseModal);
  $on('confirmDeleteCourseBtn', 'click', submitDeleteCourses);
  $on('deleteCourseModal', 'click', (event) => {
    if (event.target === $('deleteCourseModal')) closeDeleteCourseModal();
  });
  $on('deleteCourseList', 'change', (event) => {
    const input = event.target.closest('input[type="checkbox"]');
    if (!input) return;
    input.closest('.delete-course-option')?.classList.toggle('is-selected', input.checked);
  });
  // Clone course
  $on('cloneCourseBtn', 'click', openCloneCourseModal);
  $on('cancelCloneCourseBtn', 'click', closeCloneCourseModal);
  $on('confirmCloneCourseBtn', 'click', submitCloneCourse);
  $on('cloneCourseSelect', 'change', updateCloneCourseTitle);
  $on('cloneCourseModal', 'click', (event) => {
    if (event.target === $('cloneCourseModal')) closeCloneCourseModal();
  });

  $on('sectionForm', 'submit', async (event) => {
    event.preventDefault();
    if (!state.selectedCourseId) {
      alert('请先选择课堂');
      return;
    }
    try {
      await api(`/courses/${state.selectedCourseId}/sections`, {
        method: 'POST',
        body: JSON.stringify({ title: $('sectionTitle').value, type: 'custom' })
      });
      event.target.reset();
      await loadSections();
    } catch (error) {
      alert(`新增部分失败：${error.message}`);
    }
  });

  $on('addQuestionBtn', 'click', () => {
    if (isLockedReflectionQuizSection()) {
      alert('反思模式第三部分固定为 5 道小测题，不能新增题目。');
      return;
    }
    openQuestionModal();
  });
  $on('cancelQuestionEdit', 'click', closeQuestionModal);
  $on('saveQuestionBtn', 'click', saveQuestionFromModal);
  $on('addOptionBtn', 'click', () => addQuestionOption());
  $on('addOtherOptionBtn', 'click', addOtherQuestionOption);
  $on('editQuestionType', 'change', updateQuestionTypeFields);
  document.querySelectorAll('.question-rich-editor').forEach((editor) => {
    editor.addEventListener('input', () => syncRichEditorToSource(editor));
    editor.addEventListener('keydown', (event) => {
      if (event.key !== 'Enter' || event.isComposing) return;
      event.preventDefault();
      insertLineBreakIntoRichEditor(editor);
    });
    editor.addEventListener('paste', (event) => {
      event.preventDefault();
      const text = event.clipboardData?.getData('text/plain') || '';
      insertTextIntoRichEditor(editor, text);
    });
  });
  $on('uploadQuestionImageBtn', 'click', () => uploadImageForInput($('editQuestionTitleEditor')));
  document.querySelectorAll('[data-question-rich-format][data-rich-target]').forEach((button) => {
    button.addEventListener('mousedown', (event) => event.preventDefault());
    button.addEventListener('click', () => {
      applyQuestionRichFormat(button.dataset.richTarget, button.dataset.questionRichFormat);
    });
  });
  $on('questionModal', 'click', (event) => {
    if (event.target === $('questionModal')) closeQuestionModal();
  });
  $on('cancelStudentDetail', 'click', closeStudentDetailModal);
  $on('studentDetailModal', 'click', (event) => {
    if (event.target === $('studentDetailModal')) closeStudentDetailModal();
  });

  // 作答预览卡片：点击展开全文
  $on('studentDetailModal', 'click', (event) => {
    const card = event.target.closest('.answer-card');
    if (card) showFullAnswerModal(card);
  });
  $on('closeFullAnswer', 'click', closeFullAnswerModal);
  $on('fullAnswerModal', 'click', (event) => {
    if (event.target === $('fullAnswerModal')) closeFullAnswerModal();
  });
  // Escape 键关闭全文弹窗
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && !['INPUT', 'TEXTAREA'].includes(event.target.tagName)) {
      const fullModal = $('fullAnswerModal');
      if (fullModal && !fullModal.classList.contains('hidden')) closeFullAnswerModal();
    }
  });

  $on('classroomForm', 'submit', async (event) => {
    event.preventDefault();
    try {
      await api(`/classrooms/${state.selectedCourseId}/${state.selectedClassId}/blackboard`, {
        method: 'POST',
        body: JSON.stringify({ content: $('blackboard').value })
      });
      await loadClassroom();
      await loadStats();
    } catch (error) {
      alert(`保存黑板内容失败：${error.message}`);
    }
  });

  $on('saveClassCourseBindingBtn', 'click', saveClassCourseBinding);
  $on('classCourseBindingList', 'change', (event) => {
    const input = event.target.closest('input[type="checkbox"]');
    if (!input) return;
    input.closest('.class-course-option')?.classList.toggle('is-selected', input.checked);
  });

  $on('refreshStatsBtn', 'click', loadStats);
  $on('exportStatsBtn', 'click', () => {
    const params = new URLSearchParams();
    const courseId = Number($('statsCourseSelect').value || state.selectedCourseId || 0);
    const classId = Number($('statsClassSelect').value || 0);
    if (courseId) params.set('courseId', courseId);
    if (classId) params.set('classId', classId);
    window.location.href = `${apiBase()}/stats/export?${params.toString()}`;
  });
  $on('exportAllStatsBtn', 'click', () => {
    window.location.href = `${apiBase()}/stats/export-all`;
  });
  $on('exportPredictionSummaryBtn', 'click', () => {
    const params = new URLSearchParams();
    const classId = Number($('statsClassSelect').value || 0);
    if (classId) params.set('classId', classId);
    window.location.href = `${apiBase()}/stats/export-prediction-summary?${params.toString()}`;
  });

  // 弹窗右上角 ✕ 关闭按钮 — 统一事件委托
  const modalCloseMap = {
    createClassModal: closeCreateClassModal,
    createCourseModal: closeCreateCourseModal,
    deleteCourseModal: closeDeleteCourseModal,
    cloneCourseModal: closeCloneCourseModal,
    addStudentModal: closeAddStudentModal,
    questionModal: closeQuestionModal,
    studentDetailModal: closeStudentDetailModal,
    fullAnswerModal: closeFullAnswerModal,
    courseSettingsModal: closeCourseSettingsModal,
  };
  document.body.addEventListener('click', (event) => {
    const closeBtn = event.target.closest('.modal-close-x');
    if (!closeBtn) return;
    const overlay = closeBtn.closest('.modal-overlay');
    if (!overlay || !overlay.id) return;
    const closeFn = modalCloseMap[overlay.id];
    if (closeFn) closeFn();
  });
}

function bindClicks() {
  document.body.addEventListener('click', async (event) => {
    const button = event.target.closest('button[data-action]');
    if (!button) return;
    const action = button.dataset.action;
    const id = Number(button.dataset.id);
    if (action === 'delete-student') {
      if (!confirm('确定要删除该学生吗？此操作不可撤销。')) return;
      try {
        await api(`/students/${id}`, { method: 'DELETE' });
        await loadStudents();
        await loadAll();
      } catch (error) {
        alert(`删除学生失败：${error.message}`);
      }
    }
    if (action === 'remove-option') {
      const optionRows = document.querySelectorAll('#editOptionsList .question-option-row');
      if (optionRows.length <= 1) {
        alert('至少保留一个选项');
        return;
      }
      button.closest('.question-option-row')?.remove();
      refreshQuestionOptionIndexes();
    }
    if (action === 'upload-option-image') {
      const input = button.closest('.question-option-row')?.querySelector('.question-option-input');
      if (input) await uploadImageForInput(input);
    }
    if (action === 'set-stage') {
      await setStage(id);
    }
    if (action === 'select-course') {
      state.selectedCourseId = id;
      state.selectedSectionId = null;
      await loadAll();
    }
    if (action === 'delete-course') {
      if (!confirm('确定要删除该课堂吗？此操作不可撤销。')) return;
      try {
        await api(`/courses/${id}`, { method: 'DELETE' });
        state.selectedCourseId = null;
        state.selectedSectionId = null;
        await loadAll();
      } catch (error) {
        alert(`删除课堂失败：${error.message}`);
      }
    }
    if (action === 'select-section') {
      state.selectedSectionId = id;
      await loadQuestions();
    }
    if (action === 'edit-question') {
      const question = state.questions.find((item) => item.id === id);
      if (!question) {
        alert('未找到题目，请刷新后重试');
        return;
      }
      openQuestionModal(question);
    }
    if (action === 'delete-question') {
      if (isLockedReflectionQuizSection()) {
        alert('反思模式第三部分固定为 5 道小测题，不能停用或删除题目。');
        return;
      }
      if (!confirm('确定要停用该题目吗？')) return;
      try {
        await api(`/questions/${id}`, { method: 'DELETE' });
        await loadQuestions();
      } catch (error) {
        alert(`停用题目失败：${error.message}`);
      }
    }
    if (action === 'toggle-ai-guidance') {
      const nextEnabled = button.dataset.enabled !== '1';
      try {
        await api(`/sections/${id}/ai-guidance`, {
          method: 'PUT',
          body: JSON.stringify({ enabled: nextEnabled })
        });
        await loadSections();
        alert(nextEnabled ? 'AI学习指导已开启' : 'AI学习指导已关闭');
      } catch (error) {
        alert(`修改AI学习指导失败：${error.message}`);
      }
    }
    if (action === 'save-score') {
      await saveScoreByIndex(id);
    }
    if (action === 'student-detail') {
      const student = state.currentStatsStudents[id];
      if (!student || !student.submissionId) {
        alert('未找到学生提交记录');
        return;
      }
      await loadStudentDetailBySubmissionId(student.submissionId);
    }
  });

  $on('classSelectMain', 'change', async (event) => {
    state.selectedClassId = Number(event.target.value) || null;
    await loadAll();
  });

  $on('deleteClassBtn', 'click', async () => {
    if (!state.selectedClassId) {
      alert('请先选择要删除的班级');
      return;
    }
    if (!confirm('确定要删除该班级吗？')) return;
    try {
      await api(`/classes/${state.selectedClassId}`, { method: 'DELETE' });
      state.selectedClassId = null;
      await loadAll();
    } catch (error) {
      alert(`删除失败：${error.message}`);
    }
  });

  $on('classSelect', 'change', async (event) => {
    state.selectedClassId = Number(event.target.value) || null;
    await loadAll();
  });
  $on('courseSelect', 'change', async (event) => {
    state.selectedCourseId = Number(event.target.value);
    state.selectedSectionId = null;
    await loadAll();
  });
  $on('questionCourseSelect', 'custom-select-change', async (event) => {
    state.selectedCourseId = Number(event.target.value) || null;
    state.selectedSectionId = null;
    // 题目管理可以编辑所有课堂，不应被当前班级的课堂绑定关系重置。
    $('questionList').innerHTML = '<div class="empty">正在加载当前课堂题目...</div>';
    await loadSections();
  });
  $on('questionSectionSelect', 'change', async (event) => {
    state.selectedSectionId = Number(event.target.value) || null;
    await loadQuestions();
  });
  $on('statsCourseSelect', 'change', loadStats);
  $on('statsClassSelect', 'change', loadStats);

  document.querySelectorAll('.teacher-tabs button').forEach((button) => {
    button.addEventListener('click', async () => {
      document.querySelectorAll('.teacher-tabs button').forEach((item) => item.classList.remove('active'));
      document.querySelectorAll('.tab-content').forEach((item) => item.classList.remove('active'));
      button.classList.add('active');
      document.getElementById(button.dataset.tab).classList.add('active');
      if (button.dataset.tab === 'stats') await loadStats();
    });
  });
}

document.addEventListener('DOMContentLoaded', () => {
  const input = document.getElementById('passwordInput');
  const button = document.getElementById('passwordSubmitBtn');

  window.checkPassword = checkPassword;

  button?.addEventListener('click', () => {
    void checkPassword();
  });
  input?.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      void checkPassword();
    }
  });
  input?.focus();
});

// 学生端 JavaScript
let currentCourseId = null;
let currentStudentId = null;
let currentStudentName = null;
let currentClassName = null;
let currentStage = 0;
let surveyData = null;
let part3Score = null;
let predictionScore = null;
let pollingInterval = null;
let currentClassStudents = []; // 当前选择班级的学生名单
let historyScores = []; // 学生历史课程分数对比数据
let currentLessonNumber = 1; // 当前课程的课程序号
let part2Explanation = null; // 第二部分题目解析
let currentPartSettings = { 1: true, 2: true, 3: true, 4: true };
let currentPart2Questions = [];

const API_BASE = window.APP_CONFIG?.apiBase || `${window.location.protocol}//${window.location.hostname}:8080/api`;
let studentAlertCleanup = null;

function inferStudentAlertType(message) {
  const text = String(message || '');
  if (text.includes('成功') || text.includes('完成') || text.includes('太棒了') || text.includes('得了')) {
    return 'success';
  }
  if (text.includes('失败') || text.includes('错误') || text.includes('异常')) {
    return 'error';
  }
  return 'warning';
}

function setupStudentAlertModal() {
  const modal = document.getElementById('student-alert-modal');
  const panel = modal?.querySelector('.student-alert-panel');
  const iconNode = modal?.querySelector('.student-alert-icon');
  const titleNode = document.getElementById('student-alert-title');
  const messageNode = document.getElementById('student-alert-message');
  const confirmButton = document.getElementById('student-alert-confirm');

  if (!modal || !panel || !iconNode || !titleNode || !messageNode || !confirmButton) return;

  const closeModal = () => {
    modal.classList.add('hidden');
    document.body.style.overflow = '';
    if (studentAlertCleanup) {
      studentAlertCleanup();
      studentAlertCleanup = null;
    }
  };

  window.showStudentAlert = (message, type = inferStudentAlertType(message)) => {
    panel.classList.remove('student-alert-panel--success', 'student-alert-panel--warning', 'student-alert-panel--error');
    panel.classList.add(`student-alert-panel--${type}`);

    if (type === 'success') {
      iconNode.textContent = '🎉';
      titleNode.textContent = '提交成功';
      confirmButton.textContent = String(message || '').includes('完成')
        ? '完成闯关'
        : '继续闯关';
    } else if (type === 'error') {
      iconNode.textContent = '🚨';
      titleNode.textContent = '出现问题';
      confirmButton.textContent = '重新查看';
    } else {
      iconNode.textContent = '⚠️';
      titleNode.textContent = '还差一步';
      confirmButton.textContent = '我知道了';
    }

    messageNode.textContent = String(message || '');
    modal.classList.remove('hidden');
    document.body.style.overflow = 'hidden';

    const handleKeydown = (event) => {
      if (event.key === 'Escape' || event.key === 'Enter') {
        event.preventDefault();
        closeModal();
      }
    };

    confirmButton.onclick = closeModal;
    window.addEventListener('keydown', handleKeydown);
    studentAlertCleanup = () => {
      window.removeEventListener('keydown', handleKeydown);
      confirmButton.onclick = null;
    };

    requestAnimationFrame(() => confirmButton.focus());
  };

  window.alert = (message) => window.showStudentAlert(message);
}

// 通用选项点击处理
function handleOptionClick(e) {
  const option = e.currentTarget;
  // 如果点击的是文本输入框，不处理
  if (e.target.tagName === 'INPUT' && (e.target.type === 'text' || e.target.type === 'textarea')) {
    return;
  }
  
  const input = option.querySelector('input');
  if (!input) return;
  
  if (input.type === 'radio') {
    // 单选：选中当前，取消同name的其他
    const name = input.name;
    document.querySelectorAll(`input[name="${name}"]`).forEach(r => {
      r.closest('.option').classList.remove('selected');
    });
    input.checked = true;
    option.classList.add('selected');
  } else if (input.type === 'checkbox') {
    // 多选：切换选中状态
    input.checked = !input.checked;
    option.classList.toggle('selected', input.checked);
  }
}

function handleInputChange(e) {
  const input = e.target;
  const option = input.closest('.option');
  if (!option) return;
  
  if (input.type === 'radio') {
    const name = input.name;
    document.querySelectorAll(`input[name="${name}"]`).forEach(r => {
      r.closest('.option').classList.remove('selected');
    });
    option.classList.add('selected');
  } else if (input.type === 'checkbox') {
    option.classList.toggle('selected', input.checked);
  }
}

function bindOptionClickEvents() {
  // 处理所有option的点击事件
  document.querySelectorAll('.option').forEach(option => {
    // 先移除已经绑定的事件，避免重复绑定
    option.removeEventListener('click', handleOptionClick);
    option.addEventListener('click', handleOptionClick);
    
    // 绑定input的change事件，切换selected类
    const input = option.querySelector('input');
    if (input) {
      input.removeEventListener('change', handleInputChange);
      input.addEventListener('change', handleInputChange);
    }
  });
  updatePart3Progress();
}

// 初始化
document.addEventListener('DOMContentLoaded', () => {
  setupStudentAlertModal();

  // 从URL获取courseId
  const urlParams = new URLSearchParams(window.location.search);
  currentCourseId = urlParams.get('courseId');
  const nameInput = document.getElementById('student-name');
  const nameDropdown = document.getElementById('name-dropdown');

  // 班级选择变化时加载班级名单，用于姓名自动补全
  document.getElementById('class-select').addEventListener('change', async function() {
    const className = this.value;
    currentClassStudents = [];
    nameDropdown.style.display = 'none';
    if (!className) return;
    
    try {
      const courseHint = currentCourseId || 0;
      const res = await fetch(`${API_BASE}/teacher/class-list/${courseHint}/${encodeURIComponent(className)}`);
      
      if (!res.ok) {
        console.warn(`加载班级名单请求失败，状态码: ${res.status}`);
        return;
      }
      
      const data = await safeFetchJson(res);
      if (data.success && data.data && data.data.students) {
        currentClassStudents = data.data.students.map(item => item.student_name);
      }
    } catch (e) {
      console.error('加载班级名单失败:', e);
    }
  });

  // 姓名输入框变化时过滤显示匹配的姓名
  nameInput.addEventListener('input', function() {
    const keyword = this.value.trim().toLowerCase();
    nameDropdown.innerHTML = '';
    
    if (!keyword || currentClassStudents.length === 0) {
      nameDropdown.style.display = 'none';
      return;
    }

    // 过滤匹配的姓名
    const matched = currentClassStudents.filter(name => 
      name.toLowerCase().includes(keyword)
    );

    if (matched.length === 0) {
      nameDropdown.style.display = 'none';
      return;
    }

    // 渲染下拉选项
    matched.forEach(name => {
      const item = document.createElement('div');
      item.style.padding = '8px 12px';
      item.style.cursor = 'pointer';
      item.style.borderBottom = '1px solid #f0f0f0';
      item.textContent = name;
      item.addEventListener('click', () => {
        nameInput.value = name;
        nameDropdown.style.display = 'none';
      });
      item.addEventListener('mouseenter', () => {
        item.style.background = '#f5f7fa';
      });
      item.addEventListener('mouseleave', () => {
        item.style.background = 'white';
      });
      nameDropdown.appendChild(item);
    });

    nameDropdown.style.display = 'block';
  });

  // 点击页面其他地方隐藏下拉框
  document.addEventListener('click', (e) => {
    if (!nameInput.contains(e.target) && !nameDropdown.contains(e.target)) {
      nameDropdown.style.display = 'none';
    }
  });

  window.addEventListener('resize', () => {
    const blackboard = document.getElementById('tipBlackboard');
    applyTipLayout(!!blackboard && blackboard.style.display !== 'none');
  });
});

// 显示/隐藏加载
function showLoading(show) {
  document.getElementById('loading').classList.toggle('hidden', !show);
}

// 通用fetch响应解析，避免JSON解析错误
async function safeFetchJson(response) {
  try {
    const text = await response.text();
    if (!text) {
      throw new Error('服务器返回空响应');
    }
    return JSON.parse(text);
  } catch (e) {
    console.error('JSON解析失败，响应内容:', e);
    throw new Error('服务器响应格式错误，请重试');
  }
}

function renderRichExplanation(target, content) {
  if (!target) return;
  const normalized = String(content || '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/\n/g, '<br>');
  target.innerHTML = normalized;
}

function getDisplayStageLabel(stage) {
  return Number(stage) === 0 ? '准备环节' : `第${stage}部分`;
}

function updatePart3Progress() {
  const progressCard = document.getElementById('part3-progress');
  const progressText = document.getElementById('part3-progress-text');
  const progressHint = document.getElementById('part3-progress-hint');

  const radios = Array.from(document.querySelectorAll('#part3-content input[type="radio"]'));
  if (!radios.length) {
    return [];
  }

  const questionNames = [...new Set(radios.map((input) => input.name))];
  const answeredNames = new Set(radios.filter((input) => input.checked).map((input) => input.name));
  const unanswered = [];

  questionNames.forEach((name, index) => {
    const block = document.getElementById(name)?.closest('.student-quiz-block');
    if (!answeredNames.has(name)) {
      unanswered.push(index + 1);
      block?.classList.add('student-quiz-block--pending');
    } else {
      block?.classList.remove('student-quiz-block--pending');
    }
  });

  return unanswered;
}

function resizeTextareaToContent(textarea) {
  if (!textarea) return;
  textarea.style.height = 'auto';
  textarea.style.height = `${textarea.scrollHeight}px`;
}

function setupPart2TextareaAutosize() {
  const textarea = document.getElementById('part2-answer');
  if (!textarea) return;

  const syncHeight = () => resizeTextareaToContent(textarea);
  textarea.removeEventListener('input', syncHeight);
  textarea.addEventListener('input', syncHeight);
  syncHeight();
}

function shouldShowTopTipBar() {
  const part1Submitted = !!(surveyData && surveyData.part1_answers);
  return Number(currentStage) > 0 || part1Submitted;
}

function applyTipLayout(visible) {
  const blackboard = document.getElementById('tipBlackboard');
  if (blackboard) {
    blackboard.style.display = visible ? 'block' : 'none';
  }

  document.body.style.paddingTop = visible
    ? (window.innerWidth <= 768 ? '110px' : '90px')
    : '0px';
}

function openHistoryPage() {
  if (!currentStudentId) {
    window.showStudentAlert?.('未找到当前学生信息', 'error');
    return;
  }
  const params = new URLSearchParams({
    studentId: currentStudentId,
    courseId: String(currentCourseId || ''),
    studentName: document.getElementById('student-name')?.value?.trim() || '',
    className: currentClassName || ''
  });
  window.location.href = `student-history.html?${params.toString()}`;
}

function parseQuestionOptions(rawOptions) {
  if (!rawOptions) return [];
  try {
    let options = JSON.parse(rawOptions);
    if (typeof options === 'string') {
      options = JSON.parse(options);
    }
    return Array.isArray(options) ? options : [];
  } catch (error) {
    console.error('解析题目选项失败:', error);
    return [];
  }
}

function parseStoredPart2Answer(raw) {
  if (!raw) return null;
  if (typeof raw !== 'string') return raw;
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' ? parsed : null;
  } catch (error) {
    return null;
  }
}

function getStoredPart2Responses(raw) {
  const parsed = parseStoredPart2Answer(raw);
  if (!parsed) return [];
  if (Array.isArray(parsed.responses) && parsed.responses.length > 0) {
    return parsed.responses;
  }
  return [parsed];
}

function normalizePart2RichText(html) {
  return String(html || '')
    .replace(/<div><br><\/div>/gi, '<div></div>')
    .replace(/<div>/gi, '<br>')
    .replace(/<\/div>/gi, '')
    .replace(/^<br>/i, '')
    .trim();
}

function getPart2Editor(questionId = '') {
  const suffix = questionId ? `-${questionId}` : '';
  return document.getElementById(`part2-answer-editor${suffix}`);
}

function countPart2Highlights(root) {
  if (!root) return 0;
  return root.querySelectorAll('.part2-highlight--green, .part2-highlight--yellow, .part2-highlight--red').length;
}

function unwrapPart2HighlightNode(node) {
  if (!node || !node.parentNode) return;
  const parent = node.parentNode;
  while (node.firstChild) {
    parent.insertBefore(node.firstChild, node);
  }
  parent.removeChild(node);
}

function unwrapPart2HighlightsInContainer(container) {
  if (!container || typeof container.querySelectorAll !== 'function') return;
  const nodes = Array.from(container.querySelectorAll('.part2-highlight'));
  nodes.forEach((node) => unwrapPart2HighlightNode(node));
}

function getPart2SelectedTextSegments(editor, range) {
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

function isolatePart2TextSegment(node, start, end) {
  let target = node;
  if (end < target.textContent.length) {
    target.splitText(end);
  }
  if (start > 0) {
    target = target.splitText(start);
  }
  return target;
}

function unwrapPart2HighlightForTextNode(textNode) {
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

function wrapPart2TextNode(textNode, color) {
  if (!textNode || !textNode.parentNode || !textNode.textContent || !textNode.textContent.trim()) {
    return 0;
  }
  const span = document.createElement('span');
  span.className = `part2-highlight part2-highlight--${color}`;
  textNode.parentNode.replaceChild(span, textNode);
  span.appendChild(textNode);
  return 1;
}

function getPart2EditorHtml() {
  const editor = getPart2Editor();
  return editor ? normalizePart2RichText(editor.innerHTML) : '';
}

function getPart2EditorPlainText() {
  const editor = getPart2Editor();
  return editor ? editor.innerText.replace(/\n{3,}/g, '\n\n').trim() : '';
}

function setupPart2RichEditor() {
  document.querySelectorAll('.student-part2-editor').forEach((editor) => {
    if (!editor.innerHTML.trim()) {
      editor.innerHTML = '';
    }
  });
}

function applyPart2Highlight(color, questionId = '') {
  const editor = getPart2Editor(questionId);
  if (!editor) return;
  editor.focus();
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
    window.showStudentAlert?.('请先选中你要标注的文字。', 'warning');
    return;
  }

  const range = selection.getRangeAt(0);
  if (!editor.contains(range.commonAncestorContainer)) {
    window.showStudentAlert?.('请在第二部分答题区域内选择文字后再标注。', 'warning');
    return;
  }

  const segments = getPart2SelectedTextSegments(editor, range);
  if (segments.length === 0) {
    window.showStudentAlert?.('请选中具体文字后再标注。', 'warning');
    return;
  }

  let appliedCount = 0;
  segments.reverse().forEach(({ node, start, end }) => {
    const isolatedNode = isolatePart2TextSegment(node, start, end);
    const plainNode = unwrapPart2HighlightForTextNode(isolatedNode);
    appliedCount += wrapPart2TextNode(plainNode, color);
  });
  if (appliedCount === 0) {
    window.showStudentAlert?.('请选中具体文字后再标注。', 'warning');
    return;
  }
  editor.normalize();
  selection.removeAllRanges();
}

function clearPart2Highlight(questionId = '') {
  const editor = getPart2Editor(questionId);
  if (!editor) return;
  editor.focus();
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
    window.showStudentAlert?.('请先选中要去除颜色的文字。', 'warning');
    return;
  }

  const range = selection.getRangeAt(0);
  if (!editor.contains(range.commonAncestorContainer)) {
    window.showStudentAlert?.('请在第二部分答题区域内选择文字后再去除颜色。', 'warning');
    return;
  }

  const segments = getPart2SelectedTextSegments(editor, range);
  if (segments.length === 0) {
    window.showStudentAlert?.('请选中具体文字后再去除颜色。', 'warning');
    return;
  }

  segments.reverse().forEach(({ node, start, end }) => {
    const isolatedNode = isolatePart2TextSegment(node, start, end);
    unwrapPart2HighlightForTextNode(isolatedNode);
  });
  editor.normalize();
  selection.removeAllRanges();
}

// 开始问卷
async function startSurvey() {
  const studentName = document.getElementById('student-name').value.trim();
  const className = document.getElementById('class-select').value;
  
  if (!studentName) {
    window.showStudentAlert?.('请输入你的姓名！', 'warning');
    return;
  }
  
  if (!className) {
    window.showStudentAlert?.('请选择你的班级！', 'warning');
    return;
  }
  
  currentStudentName = studentName;
  currentClassName = className;
  currentStudentId = `${className}_${studentName}`;
  
  try {
    showLoading(true);
    
    // 验证学生
    const validateRes = await fetch(`${API_BASE}/student/validate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        courseId: currentCourseId,
        className,
        studentName
      })
    });
    
    if (!validateRes.ok) {
      throw new Error(`验证请求失败，状态码: ${validateRes.status}`);
    }
    
    const validateResult = await safeFetchJson(validateRes);
    
    if (!validateResult.success || !validateResult.data || !validateResult.data.valid) {
      const errMsg = validateResult && validateResult.data && validateResult.data.message ? validateResult.data.message : '验证失败，请联系老师';
      window.showStudentAlert?.(errMsg, 'error');
      return;
    }
    
    // 使用班级绑定的课程ID，优先级最高
    currentCourseId = validateResult.data.courseId;
    
    // 获取学生状态
    const statusRes = await fetch(`${API_BASE}/student/${encodeURIComponent(currentStudentId)}?courseId=${currentCourseId}`);
    
    if (!statusRes.ok) {
      throw new Error(`获取学生状态失败，状态码: ${statusRes.status}`);
    }
    
    const statusResult = await safeFetchJson(statusRes);
    
    if (statusResult.success) {
      surveyData = statusResult.data.survey || null; // 空的时候设为null
      currentStage = statusResult.data.currentStage;
      historyScores = statusResult.data.historyScores || []; // 保存历史分数对比数据
      currentLessonNumber = statusResult.data.lessonNumber || 1; // 保存当前课程的课程序号
      currentPartSettings = normalizePartSettings(statusResult.data.partSettings);
      
      // 更新标题
      document.getElementById('course-title').textContent = `人工智能学习平台 · ${statusResult.data.courseName}`;
      document.getElementById('lesson-name').textContent = statusResult.data.courseName;
      
      // 加载题目（加载成功后再切换页面）
      await loadAllQuestions();
      
      // 切换到问卷页面
      document.getElementById('login-page').classList.add('hidden');
      document.getElementById('survey-page').classList.remove('hidden');
      await loadTip();
      
      // 恢复已提交的内容
      restoreSubmittedData();
      
      // 更新UI
      updateUI();
      
      // 开始轮询阶段变化
      startPolling();
    }
  } catch (error) {
    console.error('开始问卷失败:', error);
    // 出错后恢复登录页显示
    document.getElementById('login-page').classList.remove('hidden');
    document.getElementById('survey-page').classList.add('hidden');
    // 显示具体错误信息方便排查
    window.showStudentAlert?.(`错误详情：${error.message || '网络错误，请重试！'}`, 'error');
  } finally {
    showLoading(false);
  }
}

// 加载所有部分的题目
async function loadAllQuestions() {
  for (let part = 1; part <= 3; part++) {
    if (!isPartEnabled(part)) continue;
    try {
      const res = await fetch(`${API_BASE}/student/questions/${part}${currentCourseId ? `?courseId=${currentCourseId}` : ''}`);
      if (!res.ok) throw new Error(`请求失败，状态码: ${res.status}`);
      
      const result = await safeFetchJson(res);
      if (result.success) {
        renderQuestions(part, result.data.questions);
      } else {
        throw new Error(result.error || `加载第${part}部分题目失败`);
      }
    } catch (error) {
      console.error(`加载第${part}部分题目失败:`, error);
      throw error; // 抛出错误让上层捕获
    }
  }
}

function normalizePartSettings(partSettings) {
  const defaults = { 1: true, 2: true, 3: true, 4: true };
  if (!partSettings) return defaults;
  Object.keys(defaults).forEach((part) => {
    defaults[part] = partSettings[part] !== false;
  });
  return defaults;
}

function isPartEnabled(part) {
  return currentPartSettings[String(part)] !== false && currentPartSettings[part] !== false;
}

function getNextRequiredPart(partSubmittedMap) {
  for (let part = 1; part <= 4; part++) {
    if (!isPartEnabled(part)) continue;
    if (!partSubmittedMap[part]) return part;
  }
  return null;
}

// 加载第二部分答题指引
async function loadPart2Guide() {
  if (!currentCourseId) return;
  
  try {
    const res = await fetch(`${API_BASE}/teacher/part2-guide/${currentCourseId}`);
    const data = await res.json();
    if (data.success && data.data.guide) {
      document.getElementById('part2GuideContent').innerHTML = data.data.guide;
      document.getElementById('part2GuideSection').style.display = 'block';
    } else {
      document.getElementById('part2GuideSection').style.display = 'none';
    }
  } catch (error) {
    console.error('加载答题指引失败:', error);
    document.getElementById('part2GuideSection').style.display = 'none';
  }
}

// 渲染题目
function renderQuestions(part, questions) {
  const container = document.getElementById(`part${part}-content`);
  if (!container || !questions || questions.length === 0) return;
  
  let html = '';
  
  questions.forEach((q, index) => {
    const qNum = index + 1;
    const options = parseQuestionOptions(q.options);
    
    if (part === 1) {
      // 第一部分：评分题+多选题
      if (qNum === 1) {
        // 第一题是评分题
        html += `
          <section class="question-item student-quiz-block student-quiz-block--prediction">
            <div class="student-quiz-head">
              <span class="student-quiz-tag">热身关卡</span>
              <h3>题目一：猜一猜 🤔</h3>
            </div>
            <p class="student-quiz-text">${q.question_text}</p>
            <div class="rating-options" id="prediction-score">
              ${options.map((opt, i) => `
                <div class="rating-option" data-score="${i}">
                  <div class="emoji">${['😰', '😟', '😅', '🤔', '😊', '🌟'][i]}</div>
                  <div class="score">${i}分</div>
                  <div class="desc">${opt.split(' - ')[1]}</div>
                </div>
              `).join('')}
            </div>
          </section>
        `;
      } else {
        // 第二题是多选题
        html += `
          <section class="question-item student-quiz-block student-quiz-block--plan">
            <div class="student-quiz-head">
              <span class="student-quiz-tag">准备关卡</span>
              <h3>题目二：我的计划 📋</h3>
            </div>
            <p class="student-quiz-text">${q.question_text}</p>
            <div class="options" id="learning-methods">
              ${options.map((opt, i) => `
                <div class="option">
                  <input type="checkbox" id="method${i}" value="${opt}">
                  <label for="method${i}">${opt}</label>
                </div>
              `).join('')}
              <div class="option option-with-input">
                <input type="checkbox" id="method-custom-check">
                <label for="method-custom-check">其他：</label>
                <input type="text" id="method-custom" placeholder="请写下你的方法...">
              </div>
            </div>
          </section>
        `;
      }
    } else if (part === 2) {
      // 加载答题指引
      loadPart2Guide();
      if (index === 0) {
        currentPart2Questions = questions;
      }
      html += `
        <section class="question-item student-quiz-block student-quiz-block--thinking">
          <div class="student-quiz-head">
            <span class="student-quiz-tag">思考关卡</span>
            <h3>${q.question_text}</h3>
          </div>
      `;
      if (q.question_type === 'text') {
        const annotationEnabled = q.annotation_enabled !== false;
        part2Explanation = q.explanation || '';
        html += `
          <p class="student-quiz-text">${annotationEnabled ? '请写下你的思考，并按要求用颜色标注重点、疑惑和错误观点。' : '请写下你的思考，完整表达自己的想法。'}</p>
        `;
        if (annotationEnabled) {
          html += `
          <div class="student-part2-guide">
            <div class="student-part2-toolbar">
              <button type="button" class="btn student-mark-btn student-mark-btn--green" onclick="applyPart2Highlight('green', ${q.id})">绿色：关键事实或观点</button>
              <button type="button" class="btn student-mark-btn student-mark-btn--yellow" onclick="applyPart2Highlight('yellow', ${q.id})">黄色：完全看不懂或不清楚的地方</button>
              <button type="button" class="btn student-mark-btn student-mark-btn--red" onclick="applyPart2Highlight('red', ${q.id})">红色：明显错误或自己不同意的内容</button>
              <button type="button" class="btn student-mark-btn student-mark-btn--clear" onclick="clearPart2Highlight(${q.id})">去除颜色</button>
            </div>
          </div>
          `;
        }
        html += `
          <div class="input-group">
            <div id="part2-answer-editor-${q.id}" class="student-part2-editor" contenteditable="true" data-placeholder="${annotationEnabled ? '请在这里写下你的答案，并至少给一处文字加上颜色标注...' : '请在这里写下你的答案...'}"></div>
          </div>
        `;
      } else if (q.question_type === 'multi') {
        html += `
          <p class="student-quiz-text">请选择所有符合你的答案。</p>
          <div class="options" id="part2-answer-options-${q.id}">
            ${options.map((opt, i) => `
              <div class="option">
                <input type="checkbox" id="part2-option-${q.id}-${i}" value="${opt}">
                <label for="part2-option-${q.id}-${i}">${opt}</label>
              </div>
            `).join('')}
          </div>
        `;
      } else {
        html += `
          <p class="student-quiz-text">请选择一个最符合的答案。</p>
          <div class="options" id="part2-answer-options-${q.id}">
            ${options.map((opt, i) => `
              <div class="option">
                <input type="radio" id="part2-option-${q.id}-${i}" name="part2-answer-${q.id}" value="${opt}">
                <label for="part2-option-${q.id}-${i}">${opt}</label>
              </div>
            `).join('')}
          </div>
        `;
      }
      if (q.question_type === 'text') {
        html += `
          <div id="part2-results" class="hidden student-inline-result">
            <h4>💡 参考解析</h4>
            <p id="part2-explanation-content" style="line-height: 1.6; margin: 0;"></p>
          </div>
        `;
      }
      html += `</section>`;
    } else if (part === 3) {
      // 第三部分：选择题/判断题
      html += `
        <section class="question-item student-quiz-block student-quiz-block--quiz">
          <div class="student-quiz-head">
            <span class="student-quiz-tag">答题关卡 ${qNum}</span>
            <h3>${qNum}. ${q.question_text}</h3>
          </div>
          <div class="options" id="q${qNum}">
            ${options.map((opt, i) => `
              <div class="option">
                <input type="radio" id="q${qNum}-${i}" name="q${qNum}" value="${opt.split('.')[0]}">
                <label for="q${qNum}-${i}">${opt}</label>
              </div>
            `).join('')}
          </div>
        </section>
      `;
    }
  });
  
  container.innerHTML = html;
  
  // 绑定评分选项点击事件
  if (part === 1) {
    document.querySelectorAll('.rating-option').forEach(opt => {
      opt.addEventListener('click', () => {
        document.querySelectorAll('.rating-option').forEach(o => o.classList.remove('selected'));
        opt.classList.add('selected');
        predictionScore = parseInt(opt.dataset.score);
      });
    });
  }
  
  // 绑定所有选项点击事件（单选/多选通用）
  bindOptionClickEvents();
  setupPart2TextareaAutosize();
  setupPart2RichEditor();
}

// 恢复已提交的数据
function restoreSubmittedData() {
  if (!surveyData) return;
  
  // 恢复第一部分
  if (surveyData.part1_answers) {
    // 恢复预测分数
    predictionScore = surveyData.part1_answers.predictionScore;
    const ratingOptions = document.querySelectorAll('.rating-option');
    ratingOptions.forEach(opt => {
      if (parseInt(opt.dataset.score) === predictionScore) {
        opt.classList.add('selected');
      }
    });
    
    // 恢复学习方法
    if (surveyData.part1_answers.learningMethods) {
      surveyData.part1_answers.learningMethods.forEach(method => {
        const checkbox = Array.from(document.querySelectorAll('#learning-methods input[type="checkbox"]'))
          .find(cb => cb.value === method);
        if (checkbox) {
          checkbox.checked = true;
          checkbox.closest('.option').classList.add('selected');
        }
      });
    }
    
    // 恢复自定义方法
    if (surveyData.part1_answers.customMethod) {
      document.getElementById('method-custom').value = surveyData.part1_answers.customMethod;
      document.getElementById('method-custom-check').checked = true;
      document.getElementById('method-custom-check').closest('.option').classList.add('selected');
    }
    
    // 锁定第一部分
    lockPart(1);
  }
  
  // 恢复第二部分
  if (surveyData.part2_answer) {
    const responses = getStoredPart2Responses(surveyData.part2_answer);
    currentPart2Questions.forEach((question, index) => {
      const response = responses.find((item) => item.questionId === question.id) || responses[index];
      if (!response) return;
      if (question.question_type === 'text') {
        const editor = getPart2Editor(question.id);
        if (editor) {
          editor.innerHTML = response.value || response.label || '';
        }
      } else if (question.question_type === 'multi') {
        const values = response.values || [];
        values.forEach((value) => {
          const input = Array.from(document.querySelectorAll(`#part2-answer-options-${question.id} input`)).find((node) => node.value === value);
          if (input) {
            input.checked = true;
            input.closest('.option')?.classList.add('selected');
          }
        });
      } else {
        const value = response.value || response.label || '';
        const input = Array.from(document.querySelectorAll(`#part2-answer-options-${question.id} input`)).find((node) => node.value === value);
        if (input) {
          input.checked = true;
          input.closest('.option')?.classList.add('selected');
        }
      }
    });
    // 显示解析
    if (part2Explanation) {
      renderRichExplanation(document.getElementById('part2-explanation-content'), part2Explanation);
      document.getElementById('part2-results').classList.remove('hidden');
    }
    lockPart(2);
  }
  
  // 恢复第三部分
  if (surveyData.part3_answers) {
    part3Score = surveyData.part3_score;
    Object.entries(surveyData.part3_answers).forEach(([key, value]) => {
      const input = document.querySelector(`input[name="${key}"][value="${value}"]`);
      if (input) {
        input.checked = true;
        input.closest('.option').classList.add('selected');
      }
    });
    lockPart(3);
    // 显示结果
    document.getElementById('part3-results').classList.remove('hidden');
    document.getElementById('total-score').textContent = `${part3Score}/5`;
  }
  
  // 恢复第四部分
  if (surveyData.part4_answers) {
    lockPart(4);
    document.getElementById('completed-page').classList.remove('hidden');
  }

  updatePart3Progress();
}

// 构建历史分数与预测对比内容
function buildHistoryScoresHtml() {
  // 加上当前课程的分数（如果已经提交了第三部分），只保留当前课程之前的历史课程（前N-1节课）
  let allScores = historyScores.filter(item => item.lessonNumber < currentLessonNumber);
  if (surveyData && surveyData.part3_score !== null && surveyData.part1_answers) {
    allScores.push({
      courseName: '本次课程',
      lessonNumber: 999, // 排到最后
      actualScore: surveyData.part3_score,
      predictedScore: surveyData.part1_answers.predictionScore || 0
    });
  }

  if (allScores.length === 0) {
    return '<p style="text-align: center; color: #999; margin: 0;">暂无历史课程数据</p>';
  }

  // 渲染所有分数
  let html = '';
  allScores.forEach(item => {
    html += `
      <div class="student-history-score-row">
        <div class="student-history-score-name">${item.courseName}</div>
        <div class="student-history-score-value">
          预测：<strong>${item.predictedScore}分</strong> / 实际：<strong>${item.actualScore}分</strong>
        </div>
      </div>
    `;
  });

  return html;
}

// 锁定部分
function lockPart(part) {
  const partEl = document.getElementById(`part${part}`);
  const btnEl = document.getElementById(`part${part}-btn`);
  if (partEl) partEl.classList.add('locked');
  if (btnEl) btnEl.classList.add('hidden');
}

// 更新UI
function updateUI() {
  // 隐藏所有部分
  const prepStage = document.getElementById('prep-stage');
  if (prepStage) prepStage.classList.add('hidden');
  for (let i = 1; i <= 4; i++) {
    const partEl = document.getElementById(`part${i}`);
    if (partEl) partEl.classList.add('hidden');
    
    // waiting部分只有2和3，判断元素存在再操作
    const waitingEl = document.getElementById(`waiting-part${i}`);
    if (waitingEl) waitingEl.classList.add('hidden');
  }
  const part3Results = document.getElementById('part3-results');
  if (part3Results) part3Results.classList.add('hidden');
  const completedPage = document.getElementById('completed-page');
  if (completedPage) completedPage.classList.add('hidden');
  
  // 根据当前阶段和提交状态显示
  const part1Submitted = surveyData && surveyData.part1_answers;
  const part2Submitted = surveyData && surveyData.part2_answer;
  const part3Submitted = surveyData && surveyData.part3_answers;
  const part4Submitted = surveyData && surveyData.part4_answers;
  const partSubmittedMap = {
    1: !!part1Submitted,
    2: !!part2Submitted,
    3: !!part3Submitted,
    4: !!part4Submitted
  };
  
  if (part4Submitted) {
    document.getElementById('completed-page').classList.remove('hidden');
    return;
  }
  
  // 显示已提交的部分
  if (part1Submitted) {
    document.getElementById('part1').classList.remove('hidden');
  }
  if (part2Submitted) {
    document.getElementById('part2').classList.remove('hidden');
  }
  if (part3Submitted) {
    document.getElementById('part3').classList.remove('hidden');
    document.getElementById('part3-results').classList.remove('hidden');
  }
  
  const currentStudentStage = getNextRequiredPart(partSubmittedMap);
  if (currentStudentStage === null) {
    document.getElementById('completed-page').classList.remove('hidden');
    return;
  }

  if (!part1Submitted && Number(currentStage) === 0) {
    prepStage?.classList.remove('hidden');
    return;
  }

  // 显示当前学生需要填写的部分：如果教师开启的阶段 >= 学生当前要完成的阶段，就显示该阶段让学生填写
  if (currentStudentStage <= currentStage) {
    if (currentStudentStage === 1 && !part1Submitted) {
      document.getElementById('part1').classList.remove('hidden');
    } else if (currentStudentStage === 2 && !part2Submitted) {
      document.getElementById('part2').classList.remove('hidden');
    } else if (currentStudentStage === 3 && !part3Submitted) {
      document.getElementById('part3').classList.remove('hidden');
      updatePart3Progress();
    } else if (currentStudentStage === 4 && !part4Submitted) {
      // 生成第四部分题目
      generatePart4Content();
      document.getElementById('part4').classList.remove('hidden');
    }
  } 
  // 如果学生已经完成了教师当前开启的所有阶段，显示等待下一部分开启的提示
  else {
    if (currentStudentStage === 2 && currentStage === 1) {
      document.getElementById('waiting-part2').classList.remove('hidden');
    } else if (currentStudentStage === 3 && currentStage === 2) {
      document.getElementById('waiting-part3').classList.remove('hidden');
    } else if (currentStudentStage === 4 && isPartEnabled(4)) {
      const waitNode = part3Submitted ? document.getElementById('waiting-part4') : document.getElementById('waiting-part3');
      if (waitNode) {
        waitNode.classList.remove('hidden');
        const textNode = waitNode.querySelector('p');
        if (textNode) {
          textNode.innerHTML = '已完成当前开放部分！<br>请等待老师开启第四部分...';
        }
      }
    } else if (currentStudentStage === 3 && currentStage === 3) {
      document.getElementById('waiting-part3').classList.remove('hidden');
    }
  }
}

// 生成第四部分内容
function generatePart4Content() {
  const container = document.getElementById('part4-content');
  if (!container) return;
  
  // 计算对比结果
  let reflection = '';
  let feedbackText = '';
  let feedbackClass = '';
  const hasQuizScore = part3Score !== null && part3Score !== undefined;
  const hasPrediction = predictionScore !== null && predictionScore !== undefined;

  if (hasQuizScore && hasPrediction) {
    if (part3Score === predictionScore) {
      reflection = 'equal';
      feedbackText = '你猜得很准，说明你很了解自己！';
      feedbackClass = 'success';
    } else if (part3Score < predictionScore) {
      reflection = 'low';
      feedbackText = '你猜高了，可能有些地方没听懂哦。';
      feedbackClass = 'warning';
    } else {
      reflection = 'high';
      feedbackText = '你猜低了，其实你比想象中厉害！';
      feedbackClass = 'success';
    }
  } else {
    reflection = 'generic';
    feedbackText = '这一部分已切换为通用课后反思，请根据本节课的学习情况完成总结。';
    feedbackClass = 'success';
  }
  
  let html = `
    <div class="student-history-score-card">
      <h3>📊 历史与预测分数对比</h3>
      ${buildHistoryScoresHtml()}
    </div>
    <h3>1. 我的学习成果 📊</h3>
    <div class="score-compare">
      <div class="score-item">
        <div class="label">小测得分</div>
        <div class="value">${hasQuizScore ? part3Score : '未开启'}</div>
      </div>
      <div class="score-item">
        <div class="label">预测分数</div>
        <div class="value">${hasPrediction ? predictionScore : '未填写'}</div>
      </div>
    </div>
    <div class="feedback ${feedbackClass}">
      ${feedbackText}
    </div>
  `;
  
  // 第二题根据结果显示
  if (reflection === 'equal' || reflection === 'high') {
    html += `
      <h3 style="margin-top: 25px;">2. 你觉得你用到了哪些方法，帮你学得这么好？（可以多选）</h3>
      <div class="options" id="part4-q2">
        <div class="option">
          <input type="checkbox" id="p4-q2-1" value="认真听老师讲课">
          <label for="p4-q2-1">A. 认真听老师讲课</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-2" value="认真看学习材料">
          <label for="p4-q2-2">B. 认真看学习材料</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-3" value="边学边做笔记">
          <label for="p4-q2-3">C. 边学边做笔记</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-4" value="上网查资料">
          <label for="p4-q2-4">D. 上网查资料</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-5" value="请教老师或同学">
          <label for="p4-q2-5">E. 请教老师或同学</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-6" value="多做练习题">
          <label for="p4-q2-6">F. 多做练习题</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-7" value="和同学一起讨论">
          <label for="p4-q2-7">G. 和同学一起讨论</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-8" value="课后复习">
          <label for="p4-q2-8">H. 课后复习</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-custom-check">
          <label for="p4-q2-custom-check">I. 其他：</label>
          <input type="text" id="p4-q2-custom" placeholder="请写下你的方法..." style="margin-left: 10px; flex: 1;">
        </div>
      </div>
    `;
  } else if (reflection === 'low') {
    html += `
      <h3 style="margin-top: 25px;">2. 你觉得是什么原因，让你没有达到学习目标？（可以多选）</h3>
      <div class="options" id="part4-q2">
        <div class="option">
          <input type="checkbox" id="p4-q2-1" value="有些地方没听懂">
          <label for="p4-q2-1">A. 有些地方没听懂</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-2" value="上课走神了">
          <label for="p4-q2-2">B. 上课走神了</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-3" value="没有认真看学习材料">
          <label for="p4-q2-3">C. 没有认真看学习材料</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-4" value="没有做笔记">
          <label for="p4-q2-4">D. 没有做笔记</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-5" value="遇到问题没有及时问">
          <label for="p4-q2-5">E. 遇到问题没有及时问</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-6" value="练习不够">
          <label for="p4-q2-6">F. 练习不够</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-7" value="课后没有复习">
          <label for="p4-q2-7">G. 课后没有复习</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-8" value="计划的方法没用上">
          <label for="p4-q2-8">H. 计划的方法没用上</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-custom-check">
          <label for="p4-q2-custom-check">I. 其他：</label>
          <input type="text" id="p4-q2-custom" placeholder="请写下原因..." style="margin-left: 10px; flex: 1;">
        </div>
      </div>
    `;
  } else {
    html += `
      <h3 style="margin-top: 25px;">2. 这节课哪些做法对你最有帮助？（可以多选）</h3>
      <div class="options" id="part4-q2">
        <div class="option">
          <input type="checkbox" id="p4-q2-1" value="认真听老师讲课">
          <label for="p4-q2-1">A. 认真听老师讲课</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-2" value="认真看学习材料">
          <label for="p4-q2-2">B. 认真看学习材料</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-3" value="边学边做笔记">
          <label for="p4-q2-3">C. 边学边做笔记</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-4" value="和同学一起讨论">
          <label for="p4-q2-4">D. 和同学一起讨论</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-5" value="遇到问题及时提问">
          <label for="p4-q2-5">E. 遇到问题及时提问</label>
        </div>
        <div class="option">
          <input type="checkbox" id="p4-q2-custom-check">
          <label for="p4-q2-custom-check">F. 其他：</label>
          <input type="text" id="p4-q2-custom" placeholder="请写下你的想法..." style="margin-left: 10px; flex: 1;">
        </div>
      </div>
    `;
  }
  
  // 第三题
  html += `
    <h3 style="margin-top: 25px;">3. 下节课，我可以怎样做得更好？（可以多选）</h3>
    <div class="options" id="part4-q3">
      <div class="option">
        <input type="checkbox" id="p4-q3-1" value="更认真听讲，不走神">
        <label for="p4-q3-1">更认真听讲，不走神</label>
      </div>
      <div class="option">
        <input type="checkbox" id="p4-q3-2" value="不懂的地方马上问老师或同学">
        <label for="p4-q3-2">不懂的地方马上问老师或同学</label>
      </div>
      <div class="option">
        <input type="checkbox" id="p4-q3-3" value="提前预习一下新课内容">
        <label for="p4-q3-3">提前预习一下新课内容</label>
      </div>
      <div class="option">
        <input type="checkbox" id="p4-q3-4" value="把重要的地方记在本子上">
        <label for="p4-q3-4">把重要的地方记在本子上</label>
      </div>
      <div class="option">
        <input type="checkbox" id="p4-q3-5" value="和同桌互相考一考">
        <label for="p4-q3-5">和同桌互相考一考</label>
      </div>
      <div class="option">
        <input type="checkbox" id="p4-q3-6" value="课后复习一遍今天学的知识">
        <label for="p4-q3-6">课后复习一遍今天学的知识</label>
      </div>
      <div class="option">
        <input type="checkbox" id="p4-q3-custom-check">
        <label for="p4-q3-custom-check">其他：</label>
        <input type="text" id="p4-q3-custom" placeholder="请写下你的方法..." style="margin-left: 10px; flex: 1;">
      </div>
    </div>
  `;
  
  container.innerHTML = html;
  
  // 绑定所有选项点击事件（单选/多选通用）
  bindOptionClickEvents();
}

// 提交第一部分
async function submitPart1() {
  if (predictionScore === null) {
    window.showStudentAlert?.('请先给自己打分！', 'warning');
    return;
  }
  
  // 收集学习方法
  const learningMethods = [];
  document.querySelectorAll('#learning-methods input[type="checkbox"]:checked').forEach(cb => {
    if (cb.id !== 'method-custom-check') {
      learningMethods.push(cb.value);
    }
  });
  
  const customCheck = document.getElementById('method-custom-check');
  const customMethod = customCheck.checked ? document.getElementById('method-custom').value.trim() : null;
  
  if (learningMethods.length === 0 && !customMethod) {
    window.showStudentAlert?.('请至少选择一种学习方法！', 'warning');
    return;
  }
  
  try {
    showLoading(true);
    
    const res = await fetch(`${API_BASE}/student/${encodeURIComponent(currentStudentId)}/part1`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        courseId: currentCourseId,
        studentName: currentStudentName,
        className: currentClassName,
        answers: {
          predictionScore,
          learningMethods,
          customMethod
        }
      })
    });
    
    if (!res.ok) {
      throw new Error(`提交请求失败，状态码: ${res.status}`);
    }
    
    const result = await safeFetchJson(res);
    
    if (result.success) {
      // 更新本地数据
      if (!surveyData) surveyData = {};
      surveyData.part1_answers = result.data;
      
      // 锁定第一部分
      lockPart(1);
      
      // 更新UI
      updateUI();
      
      window.showStudentAlert?.('第一部分提交成功！🎉', 'success');
    } else {
      window.showStudentAlert?.(result.error || '提交失败，请重试', 'error');
    }
  } catch (error) {
    console.error('提交失败:', error);
    window.showStudentAlert?.(`提交失败: ${error.message || '请重试'}`, 'error');
  } finally {
    showLoading(false);
  }
}

// 提交第二部分
async function submitPart2() {
  const responses = [];
  for (const question of currentPart2Questions) {
    if (question.question_type === 'text') {
      const editor = getPart2Editor(question.id);
      const answer = editor ? normalizePart2RichText(editor.innerHTML) : '';
      const plainText = editor ? editor.innerText.replace(/\n{3,}/g, '\n\n').trim() : '';
      if (!plainText) {
        window.showStudentAlert?.('请填写第二部分的思考题答案！', 'warning');
        return;
      }
      if (question.annotation_enabled !== false && countPart2Highlights(editor) === 0) {
        window.showStudentAlert?.('请至少对一处内容进行颜色标注后再提交。', 'warning');
        return;
      }
      responses.push({ questionId: question.id, answer, answers: [] });
    } else if (question.question_type === 'multi') {
      const answers = Array.from(document.querySelectorAll(`#part2-answer-options-${question.id} input[type="checkbox"]:checked`)).map((input) => input.value);
      if (answers.length === 0) {
        window.showStudentAlert?.('请完成第二部分的选择题后再提交！', 'warning');
        return;
      }
      responses.push({ questionId: question.id, answer: '', answers });
    } else {
      const answer = document.querySelector(`#part2-answer-options-${question.id} input[type="radio"]:checked`)?.value || '';
      if (!answer) {
        window.showStudentAlert?.('请先选择你对人工智能生成内容的理解程度！', 'warning');
        return;
      }
      responses.push({ questionId: question.id, answer, answers: [] });
    }
  }
  
  try {
    showLoading(true);
    
    const res = await fetch(`${API_BASE}/student/${encodeURIComponent(currentStudentId)}/part2`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        courseId: currentCourseId,
        responses
      })
    });
    
    if (!res.ok) {
      throw new Error(`提交请求失败，状态码: ${res.status}`);
    }
    
    const result = await safeFetchJson(res);
    
    if (result.success) {
      // 更新本地数据
      if (!surveyData) surveyData = {};
      surveyData.part2_answer = result.data.answer || '';
      
      // 显示解析
      if (result.data.explanation) {
        part2Explanation = result.data.explanation;
      }
      if (part2Explanation) {
        renderRichExplanation(document.getElementById('part2-explanation-content'), part2Explanation);
        document.getElementById('part2-results').classList.remove('hidden');
      }
      
      // 锁定第二部分
      lockPart(2);
      
      // 更新UI
      updateUI();
      
      window.showStudentAlert?.('第二部分提交成功！🎉', 'success');
    } else {
      window.showStudentAlert?.(result.error || '提交失败，请重试', 'error');
    }
  } catch (error) {
    console.error('提交失败:', error);
    window.showStudentAlert?.(`提交失败: ${error.message || '请重试'}`, 'error');
  } finally {
    showLoading(false);
  }
}

// 提交第三部分
async function submitPart3() {
  // 收集答案
  const answers = {};
  let allAnswered = true;
  
  for (let i = 1; i <= 5; i++) {
    const key = `q${i}`;
    const selected = document.querySelector(`input[name="${key}"]:checked`);
    if (selected) {
      answers[key] = selected.value;
    } else {
      allAnswered = false;
      break;
    }
  }
  
  if (!allAnswered) {
    const unanswered = updatePart3Progress();
    const hint = unanswered.length
      ? `还差第 ${unanswered.join('、')} 题没做`
      : '请回答所有问题！';
    window.showStudentAlert?.(hint, 'warning');
    return;
  }
  
  try {
    showLoading(true);
    
    const res = await fetch(`${API_BASE}/student/${encodeURIComponent(currentStudentId)}/part3`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        courseId: currentCourseId,
        answers
      })
    });
    
    if (!res.ok) {
      throw new Error(`提交请求失败，状态码: ${res.status}`);
    }
    
    const result = await safeFetchJson(res);
    
    if (result.success) {
      // 更新本地数据
      part3Score = result.data.score;
      if (!surveyData) surveyData = {};
      surveyData.part3_answers = answers;
      surveyData.part3_score = part3Score;
      
      // 锁定第三部分
      lockPart(3);
      
      // 显示结果
      document.getElementById('part3-results').classList.remove('hidden');
      document.getElementById('total-score').textContent = `${part3Score}/${result.data.total}`;
      
      // 渲染答案解析
      const resultsContainer = document.getElementById('results-content');
      let html = '';
      result.data.results.forEach((res, index) => {
        html += `
          <div class="answer-item ${res.isCorrect ? 'correct' : 'incorrect'}">
            <h4>
              ${index + 1}. ${res.question}
              <span class="${res.isCorrect ? 'correct-mark' : 'incorrect-mark'}">
                ${res.isCorrect ? '✅ 正确' : '❌ 错误'}
              </span>
            </h4>
            <p>你的答案：<strong class="${res.isCorrect ? 'student-answer-text--correct' : 'student-answer-text--wrong'}">${res.studentAnswer}</strong></p>
            <p>正确答案：<strong class="student-answer-text--correct">${res.correctAnswer}</strong></p>
            <p class="explanation"><span class="label">解析：</span><span class="content">${res.explanation || ''}</span></p>
          </div>
        `;
      });
      resultsContainer.innerHTML = html;
      
      // 将解析内容用innerHTML渲染，支持加粗、标红、换行
      result.data.results.forEach((res, index) => {
        if (res.explanation) {
          const explanationSpan = resultsContainer.querySelectorAll('.explanation .content')[index];
          if (explanationSpan) {
            renderRichExplanation(explanationSpan, res.explanation);
          }
        }
      });
      
      // 更新UI
      updateUI();
      
      window.showStudentAlert?.(`第三部分提交成功！你得了 ${part3Score}/5 分！🎉`, 'success');
    } else {
      window.showStudentAlert?.(result.error || '提交失败，请重试', 'error');
    }
  } catch (error) {
    console.error('提交失败:', error);
    window.showStudentAlert?.(`提交失败: ${error.message || '请重试'}`, 'error');
  } finally {
    showLoading(false);
  }
}

// 提交第四部分
async function submitPart4() {
  // 收集第二题答案
  const q2Answers = [];
  document.querySelectorAll('#part4-q2 input[type="checkbox"]:checked').forEach(cb => {
    if (cb.id !== 'p4-q2-custom-check') {
      q2Answers.push(cb.value);
    }
  });
  
  const q2CustomCheck = document.getElementById('p4-q2-custom-check');
  const q2Custom = q2CustomCheck.checked ? document.getElementById('p4-q2-custom').value.trim() : null;
  
  if (q2Answers.length === 0 && !q2Custom) {
    window.showStudentAlert?.('请回答第二题！', 'warning');
    return;
  }
  
  // 收集第三题答案
  const q3Answers = [];
  document.querySelectorAll('#part4-q3 input[type="checkbox"]:checked').forEach(cb => {
    if (cb.id !== 'p4-q3-custom-check') {
      q3Answers.push(cb.value);
    }
  });
  
  const q3CustomCheck = document.getElementById('p4-q3-custom-check');
  const q3Custom = q3CustomCheck.checked ? document.getElementById('p4-q3-custom').value.trim() : null;
  
  if (q3Answers.length === 0 && !q3Custom) {
    window.showStudentAlert?.('请回答第三题！', 'warning');
    return;
  }
  
  try {
    showLoading(true);
    
    const res = await fetch(`${API_BASE}/student/${encodeURIComponent(currentStudentId)}/part4`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        courseId: currentCourseId,
        answers: {
          scoreCompare: {
            actual: part3Score ?? null,
            predicted: predictionScore ?? null
          },
          q2: {
            answers: q2Answers,
            custom: q2Custom
          },
          q3: {
            answers: q3Answers,
            custom: q3Custom
          }
        }
      })
    });
    
    if (!res.ok) {
      throw new Error(`提交请求失败，状态码: ${res.status}`);
    }
    
    const result = await safeFetchJson(res);
    
    if (result.success) {
      // 更新本地数据
      if (!surveyData) surveyData = {};
      surveyData.part4_answers = {
        scoreCompare: {
          actual: part3Score ?? null,
          predicted: predictionScore ?? null
        },
        q2: {
          answers: q2Answers,
          custom: q2Custom
        },
        q3: {
          answers: q3Answers,
          custom: q3Custom
        }
      };
      
      // 锁定第四部分
      lockPart(4);
      
      // 显示完成页面
      document.getElementById('completed-page').classList.remove('hidden');
    } else {
      window.showStudentAlert?.(result.error || '提交失败，请重试', 'error');
    }
  } catch (error) {
    console.error('提交失败:', error);
    window.showStudentAlert?.(`提交失败: ${error.message || '请重试'}`, 'error');
  } finally {
    showLoading(false);
  }
}

// 加载提示语
async function loadTip() {
  if (!currentClassName) return;
  
  try {
    const res = await fetch(`${API_BASE}/student/tip?className=${encodeURIComponent(currentClassName)}`);
    const tipContent = document.getElementById('tipContent');
    const prepTipContent = document.getElementById('prepTipContent');
    if (!res.ok) {
      if (tipContent) {
        tipContent.innerHTML = '';
      }
      if (prepTipContent) {
        prepTipContent.textContent = '课堂提示语暂时不可用，请稍等老师开启课堂。';
      }
      applyTipLayout(false);
      return;
    }
    
    const result = await safeFetchJson(res);
    const content = result?.success ? String(result?.data?.content || '').trim() : '';
    if (content) {
      if (tipContent) {
        tipContent.innerHTML = content;
      }
      if (prepTipContent) {
        prepTipContent.innerHTML = content;
      }
      applyTipLayout(shouldShowTopTipBar());
    } else {
      if (tipContent) {
        tipContent.innerHTML = '';
      }
      if (prepTipContent) {
        prepTipContent.textContent = '老师还没有填写课堂提示语，请稍等。';
      }
      applyTipLayout(false);
    }
  } catch (error) {
    console.error('加载提示语失败:', error);
    const tipContent = document.getElementById('tipContent');
    if (tipContent) {
      tipContent.innerHTML = '';
    }
    const prepTipContent = document.getElementById('prepTipContent');
    if (prepTipContent) {
      prepTipContent.textContent = '课堂提示语加载失败，请稍后再试。';
    }
    applyTipLayout(false);
  }
}

// 轮询阶段变化
function startPolling() {
  if (pollingInterval) clearInterval(pollingInterval);
  
  pollingInterval = setInterval(async () => {
    try {
      // 加载阶段和学生数据
      const res = await fetch(`${API_BASE}/student/${encodeURIComponent(currentStudentId)}${currentCourseId ? `?courseId=${currentCourseId}` : ''}`);
      
      if (!res.ok) {
        console.warn(`轮询请求失败，状态码: ${res.status}`);
        return;
      }
      
      const result = await safeFetchJson(res);
      
      if (result.success) {
        const newStage = result.data.currentStage;
        const newSurvey = result.data.survey || null; // 空的时候设为null
        const newHistoryScores = result.data.historyScores || [];
        const newPartSettings = normalizePartSettings(result.data.partSettings);
        
        const settingsChanged = JSON.stringify(newPartSettings) !== JSON.stringify(currentPartSettings);
        if (newStage !== currentStage || JSON.stringify(newSurvey) !== JSON.stringify(surveyData) || JSON.stringify(newHistoryScores) !== JSON.stringify(historyScores) || settingsChanged) {
          currentStage = newStage;
          surveyData = newSurvey;
          historyScores = newHistoryScores;
          currentLessonNumber = result.data.lessonNumber || 1;
          currentPartSettings = newPartSettings;
          if (settingsChanged) {
            await loadAllQuestions();
          }
          restoreSubmittedData();
          updateUI();
        }
      }
      
      // 加载提示语
      await loadTip();
    } catch (error) {
      console.error('轮询失败:', error);
    }
  }, 2000);
}

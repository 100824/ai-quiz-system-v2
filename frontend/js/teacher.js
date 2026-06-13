let currentCourseId = null;
let currentStage = 0;
let currentPartSettings = { 1: true, 2: true, 3: true, 4: true };
const API_BASE = window.APP_CONFIG?.apiBase || `${window.location.protocol}//${window.location.hostname}:8080/api`;
let courseAutoRefreshTimer = null;
let loadCurrentStageInFlight = false;
let currentStatsStudents = [];

// 页面加载完成
document.addEventListener('DOMContentLoaded', () => {
  loadCourses();
  initTabs();
  updateApiBaseHint();
});

function updateApiBaseHint() {
  const apiBaseNode = document.getElementById('apiBaseHint');
  if (apiBaseNode) {
    apiBaseNode.textContent = API_BASE;
    apiBaseNode.title = API_BASE;
  }
}

function isPartEnabled(part) {
  return currentPartSettings[String(part)] !== false && currentPartSettings[part] !== false;
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function formatStageLabel(stage) {
  return Number(stage) === 0 ? '准备环节' : `第${stage}部分`;
}

function renderPartToggleList() {
  const container = document.getElementById('partToggleList');
  if (!container) return;

  container.innerHTML = [1, 2, 3, 4].map((part) => `
    <label style="display: inline-flex; align-items: center; gap: 8px; padding: 10px 14px; border-radius: 14px; background: white; border: 1px solid #d7e6fa;">
      <input type="checkbox" class="part-toggle" data-part="${part}" ${isPartEnabled(part) ? 'checked' : ''}>
      <span>第${part}部分</span>
    </label>
  `).join('');
}

function updateStageButtons() {
  for (let part = 1; part <= 4; part++) {
    const button = document.getElementById(`stage-btn-${part}`);
    if (!button) continue;
    const enabled = isPartEnabled(part);
    button.disabled = !enabled;
    button.style.opacity = enabled ? '1' : '0.45';
    button.title = enabled ? '' : `第${part}部分当前已关闭`;
  }
}

async function loadPartSettings(courseId) {
  try {
    const res = await fetch(`${API_BASE}/teacher/part-settings/${courseId}`);
    const data = await safeFetchJson(res);
    if (data.success) {
      currentPartSettings = {};
      (data.data.settings || []).forEach((item) => {
        currentPartSettings[item.part] = item.enabled;
      });
      renderPartToggleList();
      updateStageButtons();
    }
  } catch (error) {
    console.error('加载部分启用状态失败:', error);
  }
}

async function savePartSettings() {
  const courseId = document.getElementById('questionCourseSelect').value || document.getElementById('stageCourseSelect').value;
  if (!courseId) {
    alert('请先选择课程');
    return;
  }

  const settings = Array.from(document.querySelectorAll('.part-toggle')).map((input) => ({
    part: Number(input.dataset.part),
    enabled: input.checked
  }));

  if (!settings.some((item) => item.enabled)) {
    alert('至少需要开启一个部分');
    return;
  }

  try {
    const res = await fetch(`${API_BASE}/teacher/part-settings/${courseId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ settings })
    });
    const data = await safeFetchJson(res);

    if (data.success) {
      currentPartSettings = {};
      settings.forEach((item) => {
        currentPartSettings[item.part] = item.enabled;
      });
      updateStageButtons();
      loadQuestions();
      loadCurrentStage();
      alert('部分启用状态已更新');
    } else {
      alert(data.error || '保存失败');
    }
  } catch (error) {
    alert('保存失败: ' + error.message);
  }
}

async function safeFetchJson(response) {
  const text = await response.text();
  if (!text) {
    throw new Error('服务器返回空响应');
  }
  return JSON.parse(text);
}

function formatPart2Answer(answer) {
  if (!answer) return '';
  if (typeof answer !== 'string') return String(answer);
  try {
    const parsed = JSON.parse(answer);
    if (Array.isArray(parsed.responses) && parsed.responses.length > 0) {
      return parsed.responses.map((item) => {
        const label = Array.isArray(item.labels) && item.labels.length > 0
          ? item.labels.join('、')
          : item.label || (Array.isArray(item.values) ? item.values.join('、') : item.value || '');
        return item.questionText ? `${item.questionText}：${label}` : label;
      }).join('；');
    }
    if (Array.isArray(parsed.labels) && parsed.labels.length > 0) {
      return parsed.labels.join('、');
    }
    if (parsed.label) {
      return parsed.label;
    }
    if (Array.isArray(parsed.values) && parsed.values.length > 0) {
      return parsed.values.join('、');
    }
    if (parsed.value) {
      return parsed.value;
    }
  } catch (error) {
    // 历史数据可能是纯文本
  }
  return answer;
}

function formatPart2AnswerRich(answer) {
  if (!answer) return '';
  if (typeof answer !== 'string') return String(answer);
  try {
    const parsed = JSON.parse(answer);
    if (Array.isArray(parsed.responses) && parsed.responses.length > 0) {
      return parsed.responses.map((item) => {
        const label = item.questionType === 'text' && item.value
          ? item.value
          : (Array.isArray(item.labels) && item.labels.length > 0
            ? item.labels.join('、')
            : item.label || (Array.isArray(item.values) ? item.values.join('、') : item.value || ''));
        return `<div style="margin-bottom: 12px;"><strong>${item.questionText || '第二部分题目'}：</strong><div>${label}</div></div>`;
      }).join('');
    }
    if (parsed.questionType === 'text' && parsed.value) {
      return parsed.value;
    }
  } catch (error) {
    return answer;
  }
  return formatPart2Answer(answer);
}

function updateAnnotationControlVisibility() {
  const questionType = document.getElementById('editQuestionType')?.value;
  const group = document.getElementById('annotationControlGroup');
  if (!group) return;
  group.style.display = questionType === 'text' ? 'block' : 'none';
}

// 初始化tab切换
function initTabs() {
  const navBtns = document.querySelectorAll('.nav-btn');
  const tabContents = document.querySelectorAll('.tab-content');
  
  navBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      const tab = btn.dataset.tab;
      
      // 切换激活状态
      navBtns.forEach(b => b.classList.remove('active'));
      tabContents.forEach(c => c.classList.remove('active'));
      
      btn.classList.add('active');
      document.getElementById(tab).classList.add('active');
      
      // 切换tab时加载对应数据
      if (tab === 'class') loadClassList();
      if (tab === 'question') loadQuestionCourses();
      if (tab === 'stats') loadStatsCourses();

      if (tab === 'course') {
        startCourseAutoRefresh();
        loadCurrentStage();
      } else {
        stopCourseAutoRefresh();
      }
    });
  });
}

function isCourseTabActive() {
  const courseTab = document.getElementById('course');
  return !!courseTab && courseTab.classList.contains('active');
}

function startCourseAutoRefresh() {
  stopCourseAutoRefresh();
  if (!isCourseTabActive()) return;

  courseAutoRefreshTimer = setInterval(() => {
    if (!isCourseTabActive()) {
      stopCourseAutoRefresh();
      return;
    }
    loadCurrentStage();
  }, 10000);
}

function stopCourseAutoRefresh() {
  if (courseAutoRefreshTimer) {
    clearInterval(courseAutoRefreshTimer);
    courseAutoRefreshTimer = null;
  }
}

// 加载课程列表
async function loadCourses() {
  try {
    const res = await fetch(`${API_BASE}/teacher/courses`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      currentCourseId = data.data.currentCourseId;
      renderCourseList(data.data.courses);
      updateHeaderInfo(data.data.courses);
    }
  } catch (error) {
    alert('加载课程失败: ' + error.message);
  }
}

// 渲染课程下拉列表
function renderCourseList(courses) {
  // 只更新进度控制的课程下拉
  const stageSelect = document.getElementById('stageCourseSelect');
  if (stageSelect) {
    stageSelect.innerHTML = courses.map(c => `<option value="${c.id}">${c.name}</option>`).join('');
    stageSelect.value = currentCourseId;
    // 加载当前阶段
    loadCurrentStage();
  }
}

// 切换课程
async function switchCourse(courseId) {
  if (courseId === currentCourseId) return;
  
  try {
    const res = await fetch(`${API_BASE}/teacher/current-course`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ courseId })
    });
    const data = await safeFetchJson(res);
    
    if (data.success) {
      currentCourseId = courseId;
      loadCourses();
      alert('切换课程成功');
    }
  } catch (error) {
    alert('切换课程失败: ' + error.message);
  }
}

// 更新头部信息
function updateHeaderInfo(courses) {
  const currentCourse = courses.find(c => c.id === currentCourseId);
  document.getElementById('currentCourse').textContent = `当前课程: ${currentCourse ? currentCourse.name : '未选择'}`;
  getCurrentStage();
}

// 获取当前阶段
async function getCurrentStage() {
  if (!currentCourseId) return;
  
  try {
    const res = await fetch(`${API_BASE}/teacher/stage/${currentCourseId}`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      currentStage = data.data.stage;
      document.getElementById('currentStage').textContent = `当前阶段: ${formatStageLabel(currentStage)}`;
    }
  } catch (error) {
    console.error('获取阶段失败:', error);
  }
}

let currentEditClassName = '';

// 关闭学生编辑模态框
function closeStudentModal() {
  document.getElementById('studentModal').classList.add('hidden');
  currentEditClassName = '';
  document.getElementById('studentNames').value = '';
}

// 加载班级学生数量
async function loadClassList() {
  // 所有班级默认用课程1查人数，前端不显示课程选项
  const courseId = currentCourseId || 1;
  
  try {
    // 依次加载7个班的学生数
    for (let i = 1; i <=7; i++) {
      const className = `五年级（${i}）班`;
      const res = await fetch(`${API_BASE}/teacher/class-list/${courseId}/${encodeURIComponent(className)}`);
      const data = await safeFetchJson(res);
      
      if (data.success) {
        const students = Array.isArray(data.data.students) ? data.data.students : [];
        const count = students.length;
        document.getElementById(`class-${i}-count`).textContent = `学生数: ${count}`;
      }
    }
  } catch (error) {
    console.error('加载班级人数失败:', error);
  }
}

// 查看班级学生
async function viewClassStudents(courseId, className) {
  try {
    const res = await fetch(`${API_BASE}/teacher/class-list/${courseId}/${encodeURIComponent(className)}`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      const students = (Array.isArray(data.data.students) ? data.data.students : []).map(s => s.student_name).join('、');
      alert(`班级 ${className} 学生:\n${students}`);
    }
  } catch (error) {
    alert('加载学生失败: ' + error.message);
  }
}

// 加载题目管理的课程下拉
async function loadQuestionCourses() {
  try {
    const res = await fetch(`${API_BASE}/teacher/courses`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      const select = document.getElementById('questionCourseSelect');
      select.innerHTML = data.data.courses.map(c => `<option value="${c.id}">${c.name}</option>`).join('');
      select.value = currentCourseId;
      await loadPartSettings(select.value);
      // 自动加载题目
      loadQuestions();
      // 绑定下拉切换自动加载事件
    }
  } catch (error) {
    console.error('加载课程失败:', error);
  }
}

// 加载题目
async function loadQuestions() {
  const courseId = document.getElementById('questionCourseSelect').value;
  const part = document.getElementById('questionPartSelect').value;
  if (!courseId || !part) return;
  await loadPartSettings(courseId);

  // 切换部分时显示/隐藏答题指引编辑区域
  const guideSection = document.getElementById('part2GuideSection');
  if (part == 2) {
    guideSection.style.display = 'block';
    // 加载当前课程的指引
    await loadPart2Guide();
  } else {
    guideSection.style.display = 'none';
  }

  if (part == 4) {
    document.getElementById('questionList').innerHTML = renderPart4QuestionPreview();
    return;
  }
  
  try {
    const res = await fetch(`${API_BASE}/teacher/questions/${courseId}/${part}`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      const html = data.data.questions.map(q => {
        let optionsText = '';
        if (q.options) {
          try {
            let options = JSON.parse(q.options);
            // 兼容双重转义的情况：如果解析后还是字符串，再解析一次
            if (typeof options === 'string') {
              options = JSON.parse(options);
            }
            if (Array.isArray(options)) {
              optionsText = `<p>选项: ${options.join(' | ')}</p>`;
            }
          } catch (e) {
            console.error('解析选项失败:', e);
          }
        }
        return `
        <div class="question-item">
          <h4>${q.id}. ${q.question_text}</h4>
          <p>状态: <strong style="color: ${q.enabled === false ? '#f56c6c' : '#67c23a'};">${q.enabled === false ? '已关闭' : '已开启'}</strong></p>
          ${q.question_type === 'text' ? `<p>颜色标注: <strong style="color: ${q.annotation_enabled === false ? '#909399' : '#409eff'};">${q.annotation_enabled === false ? '关闭' : '开启'}</strong></p>` : ''}
          ${optionsText}
          ${q.correct_answer ? `<p>正确答案: ${q.correct_answer}</p>` : ''}
          ${q.explanation ? `<p>解析: ${q.explanation}</p>` : ''}
          <div style="display: flex; gap: 8px; flex-wrap: wrap; margin-top: 8px;">
            ${!(q.part == 1 && q.sort_order == 1) ? `<button class="btn btn-sm" onclick="editQuestion(${q.id})">编辑</button>` : '<span style="color: #999; font-size: 12px; margin-left: 10px;">系统默认题目，不可编辑</span>'}
            <button class="btn btn-sm" onclick="toggleQuestionEnabled(${q.id}, ${q.enabled === false ? 'true' : 'false'})">${q.enabled === false ? '开启题目' : '关闭题目'}</button>
          </div>
        </div>
      `}).join('');
      document.getElementById('questionList').innerHTML = html || '<p>暂无题目</p>';
    }
  } catch (error) {
    alert('加载题目失败: ' + error.message);
  }
}

function renderPart4QuestionPreview() {
  return `
    <div class="question-item">
      <h4>第四部分为动态反思题</h4>
      <p>第四部分不会像前 1-3 部分一样完全固定，它会根据学生第三部分得分和第一部分预测分数动态调整提示内容。</p>
    </div>
    <div class="question-item">
      <h4>1. 我的学习成果</h4>
      <p>系统会展示“小测得分”和“预测分数”的对比，并给出反馈文案。</p>
      <p style="color: #666;">示例反馈：你猜得很准 / 你猜高了 / 你猜低了。</p>
    </div>
    <div class="question-item">
      <h4>2. 反思原因或有效方法</h4>
      <p>如果学生实际表现达到或超过预测，系统会展示“哪些方法帮助你学得这么好”。</p>
      <p>如果学生实际表现低于预测，系统会展示“是什么原因让你没有达到学习目标”。</p>
    </div>
    <div class="question-item">
      <h4>3. 下节课如何做得更好</h4>
      <p>系统会展示改进计划多选项和自定义输入，帮助学生形成下次课堂行动方案。</p>
      <p style="color: #999; font-size: 12px;">第四部分当前为系统动态题，题目管理中仅提供展示预览，不支持逐题编辑。</p>
    </div>
  `;
}

// 加载统计的课程下拉
async function loadStatsCourses() {
  try {
    const res = await fetch(`${API_BASE}/teacher/courses`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      const select = document.getElementById('statsCourseSelect');
      select.innerHTML = data.data.courses.map(c => `<option value="${c.id}">${c.name}</option>`).join('');
      select.value = currentCourseId;
      loadStatsClasses();
    }
  } catch (error) {
    console.error('加载课程失败:', error);
  }
}

// 加载统计的班级下拉
async function loadStatsClasses() {
  const courseId = document.getElementById('statsCourseSelect').value;
  if (!courseId) return;
  
  try {
    const res = await fetch(`${API_BASE}/teacher/classes/${courseId}`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      const select = document.getElementById('statsClassSelect');
      select.innerHTML = '<option value="">所有班级</option>' + data.data.classes.map(c => `<option value="${c.class_name}">${c.class_name}</option>`).join('');
    }
  } catch (error) {
    console.error('加载班级失败:', error);
  }
}
async function loadStats() {
  try {
    const courseId = document.getElementById("statsCourseSelect").value;
    const className = document.getElementById("statsClassSelect").value;
    if (!courseId) {
      alert("请先选择课程");
      return;
    }
    const res = await fetch(`${API_BASE}/teacher/stats/${courseId}${className ? `?className=${encodeURIComponent(className)}` : ""}`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      const stats = data.data;
      let html = '';
      
      // 加载班级名单（仅当选择了具体班级时）
      let totalClass = 0;
      let notSubmittedStudents = [];
      if (className) {
        try {
          const classRes = await fetch(`${API_BASE}/teacher/class-list/${courseId}/${encodeURIComponent(className)}`);
          const classData = await safeFetchJson(classRes);
          if (classData.success) {
            const allStudents = (Array.isArray(classData.data.students) ? classData.data.students : []).map(s => s.student_name);
            const submittedStudents = (Array.isArray(stats.students) ? stats.students : []).map(s => s.student_name);
            totalClass = allStudents.length;
            notSubmittedStudents = allStudents.filter(name => !submittedStudents.includes(name));
          }
        } catch (e) {
          console.error('加载班级名单失败:', e);
        }
      }
      
      // 完成情况
      html += '<div class="stats-section"><h4>各部分完成情况</h4>';
      if (className && totalClass > 0) {
        html += `<p><strong>班级总人数：</strong>${totalClass}人，已答题：${(Array.isArray(stats.students) ? stats.students : []).length}人，未提交：${notSubmittedStudents.length}人</p>`;
        if (notSubmittedStudents.length > 0) {
          html += `<p style="color: #f56c6c;"><strong>未提交学生：</strong>${notSubmittedStudents.join('、')}</p>`;
        }
        html += `<div style="height: 1px; background: #eee; margin: 10px 0;"></div>`;
        for (let i = 1; i <=4; i++) {
          const part = stats[`part${i}`];
          html += `<p>第${i}部分: ${part.completed}/${totalClass} 人完成 (${Math.round(part.completed / totalClass * 100 || 0)}%)</p>`;
        }
      } else {
        for (let i = 1; i <=4; i++) {
          const part = stats[`part${i}`];
          html += `<p>第${i}部分: ${part.completed}/${part.total} 人完成 (${Math.round(part.completed / part.total * 100 || 0)}%)</p>`;
        }
      }
      html += '</div>';
      
      // ==================== 各部分填写情况统计 ====================
      html += '<div class="stats-section"><h4>📊 各部分填写情况统计</h4>';
      
      // 第一部分统计
      html += '<div style="margin-bottom: 20px;">';
      html += '<h5 style="margin: 0 0 10px 0; color: #409eff;">第一部分：上课前填写情况</h5>';
      // 预测得分分布
      html += '<p><strong>预测得分分布：</strong></p><ul style="margin: 5px 0 15px 0; padding-left: 20px;">';
      for (let score in stats.part1Stats.predictionScoreDistribution) {
        const count = stats.part1Stats.predictionScoreDistribution[score];
        html += `<li>${score}分：${count}人</li>`;
      }
      html += '</ul>';
      // 学习方法分布
      html += '<p><strong>学习方法选择分布：</strong></p><ul style="margin: 5px 0 0 20px; padding-left: 20px;">';
      for (let method in stats.part1Stats.learningMethodsDistribution) {
        const count = stats.part1Stats.learningMethodsDistribution[method];
        html += `<li>${method}：${count}人</li>`;
      }
      html += '</ul></div>';
      
      // 第二部分统计
      html += '<div style="margin-bottom: 20px;">';
      html += '<h5 style="margin: 0 0 10px 0; color: #67c23a;">第二部分：思考与讨论填写情况</h5>';
      const filledRate = stats.part2Stats.totalCount > 0 ? Math.round(stats.part2Stats.filledCount / stats.part2Stats.totalCount * 100) : 0;
      html += `<p>填写人数：${stats.part2Stats.filledCount}/${stats.part2Stats.totalCount}人（${filledRate}%）</p>`;
      const understandingStats = stats.part2Stats.understandingDistribution || {};
      const understandingItems = Object.entries(understandingStats);
      if (understandingItems.length > 0) {
        html += '<p><strong>理解程度分布：</strong></p><ul style="margin: 5px 0 0 20px; padding-left: 20px;">';
        understandingItems.forEach(([label, count]) => {
          html += `<li>${label}：${count}人</li>`;
        });
        html += '</ul>';
      }
      html += '</div>';
      
      // 第三部分统计
      html += '<div>';
      html += '<h5 style="margin: 0 0 10px 0; color: #e6a23c;">第三部分：小测填写情况</h5>';
      // 得分分布
      html += '<p><strong>得分分布：</strong></p><ul style="margin: 5px 0 15px 0; padding-left: 20px;">';
      for (let score = 0; score <=5; score++) {
        const count = stats.part3Stats.scoreDistribution[score];
        html += `<li>${score}分：${count}人</li>`;
      }
      html += '</ul>';
      // 每道题正确率
      html += '<p style="margin-top: 15px;"><strong>每道题正确率：</strong></p><ul style="margin: 5px 0 0 20px; padding-left: 20px;">';
      stats.part3Stats.questionCorrectRate.forEach(q => {
        html += `<li>第${q.sortOrder}题：${q.correctRate}%（${q.correctCount}/${q.totalCount}人答对）<br><span style="color: #666; font-size: 12px;">题目：${q.questionText}</span></li>`;
      });
      html += '</ul>';
      html += '</div>';
      
      html += '</div>';
      
      // 学生列表
      currentStatsStudents = Array.isArray(stats.students) ? stats.students : [];
      html += '<div class="stats-section"><h4>学生完成情况</h4>';
      html += '<p style="color: #666; margin-bottom: 12px;">教师评分会作为学生历史页的实际分优先展示，不会覆盖第三部分小测分。</p>';
      html += '<table class="stats-table"><thead><tr><th>班级</th><th>姓名</th><th>完成状态</th><th>第三部分得分</th><th>教师评分</th><th>最终实际分</th><th>备注</th><th>操作</th></tr></thead><tbody>';
      currentStatsStudents.forEach((s, index) => {
        const status = s.part4_answers ? '已完成全部' : s.part3_answers ? '完成到第三部分' : s.part2_answer ? '完成到第二部分' : s.part1_answers ? '完成到第一部分' : '未开始';
        const score = s.part3_score !== null ? `${s.part3_score}/5` : '未完成';
        const teacherScore = s.teacher_score !== null && s.teacher_score !== undefined ? s.teacher_score : '';
        const actualScore = s.actual_score !== null && s.actual_score !== undefined ? `${s.actual_score}/5` : '待评分';
        const sourceLabel = s.actual_score_source === 'teacher' ? '教师评分' : (s.actual_score_source === 'part3' ? '小测' : '无');
        const note = s.teacher_score_note || '';
        html += `
          <tr>
            <td>${escapeHtml(s.class_name)}</td>
            <td>${escapeHtml(s.student_name)}</td>
            <td>${status}</td>
            <td>${score}</td>
            <td><input id="teacher-score-${index}" type="number" min="0" max="5" step="1" value="${teacherScore}" style="width: 72px; padding: 6px 8px;"></td>
            <td>${actualScore}<br><span style="color: #888; font-size: 12px;">${sourceLabel}</span></td>
            <td><input id="teacher-score-note-${index}" type="text" value="${escapeHtml(note)}" placeholder="可选" style="width: 150px; padding: 6px 8px;"></td>
            <td>
              <button class="btn btn-sm" onclick="saveTeacherScore(${index})">保存评分</button>
              <button class="btn btn-sm" onclick="showStudentDetailByIndex(${index})">查看详情</button>
            </td>
          </tr>
        `;
      });
      html += '</tbody></table></div>';
      
      document.getElementById('statsContent').innerHTML = html;
    }
  } catch (error) {
    alert('加载统计失败: ' + error.message);
  }
}


// 编辑班级学生名单
async function editClassStudents(className) {
  currentEditClassName = className;
  const courseId = currentCourseId || 1;
  
  // 先获取现有学生名单
  try {
    const res = await fetch(`${API_BASE}/teacher/class-list/${courseId}/${encodeURIComponent(className)}`);
    const data = await safeFetchJson(res);
    let defaultNames = '';
    const students = Array.isArray(data.data.students) ? data.data.students : [];
    
    if (data.success && students.length > 0) {
      defaultNames = students.map(s => s.student_name).join('\n');
    }
    
    // 打开模态框
    document.getElementById('modalTitle').textContent = `编辑 ${className} 学生名单`;
    document.getElementById('studentNames').value = defaultNames;
    document.getElementById('studentModal').classList.remove('hidden');
  } catch (error) {
    alert('加载学生名单失败: ' + error.message);
  }
}

// 保存学生名单
async function saveStudentList() {
  if (!currentEditClassName) return;
  
  const namesText = document.getElementById('studentNames').value.trim();
  if (!namesText) {
    alert('请至少输入一个学生姓名');
    return;
  }
  
  const nameList = namesText.split(/\n/).map(n => n.trim()).filter(n => n);
  if (nameList.length === 0) {
    alert('请至少输入一个学生姓名');
    return;
  }
  
  try {
    const courseId = currentCourseId || 1;
    await importClass(courseId, currentEditClassName, nameList);
    alert('学生名单保存成功！');
    closeStudentModal();
    loadClassList(); // 刷新人数
  } catch (error) {
    alert('保存失败: ' + error.message);
  }
}

async function importClass(courseId, className, studentNames) {
  const res = await fetch(`${API_BASE}/teacher/class-list`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ courseId, className, studentNames })
  });

  const data = await safeFetchJson(res);
  if (!res.ok || !data.success) {
    throw new Error(data.error || data.message || `请求失败，状态码: ${res.status}`);
  }

  return data;
}

let currentEditingQuestion = null;

// 关闭题目编辑模态框
function closeQuestionModal() {
  document.getElementById('questionModal').classList.add('hidden');
  currentEditingQuestion = null;
  document.getElementById('editQuestionId').value = '';
  document.getElementById('editQuestionText').value = '';
  document.getElementById('editQuestionType').value = 'single';
  document.getElementById('optionsList').innerHTML = '';
  document.getElementById('editCorrectAnswer').value = '';
  document.getElementById('editExplanation').innerHTML = '';
}

// 新增选项
function addOption(value = '') {
  const optionsList = document.getElementById('optionsList');
  const optionIndex = optionsList.children.length + 1;
  const optionDiv = document.createElement('div');
  optionDiv.className = 'option-item';
  optionDiv.style = 'display: flex; gap: 10px; margin-bottom: 10px; align-items: center;';
  optionDiv.innerHTML = `
    <span style="min-width: 30px;">${optionIndex}.</span>
    <input type="text" class="option-input" value="${value.replace(/"/g, '&quot;')}" placeholder="请输入选项内容" style="flex: 1; padding: 8px; border: 1px solid #ddd; border-radius: 6px;">
    <button type="button" class="btn btn-sm btn-danger" onclick="removeOption(this)" style="padding: 6px 12px;">- 删除</button>
  `;
  optionsList.appendChild(optionDiv);
}

// 删除选项
function removeOption(btn) {
  const optionItem = btn.closest('.option-item');
  if (document.querySelectorAll('.option-item').length <= 1) {
    alert('至少需要保留一个选项！');
    return;
  }
  optionItem.remove();
  // 重新排序号
  document.querySelectorAll('.option-item').forEach((item, index) => {
    item.querySelector('span').textContent = `${index + 1}.`;
  });
}

// 编辑题目
async function editQuestion(id) {
  const courseId = document.getElementById('questionCourseSelect').value;
  const part = document.getElementById('questionPartSelect').value;
  if (!courseId || !part) return;
  
  try {
    // 先加载题目详情
    const res = await fetch(`${API_BASE}/teacher/questions/${courseId}/${part}`);
    const data = await safeFetchJson(res);
    
    if (data.success) {
      currentEditingQuestion = data.data.questions.find(q => q.id === id);
      if (!currentEditingQuestion) {
        alert('找不到该题目！');
        return;
      }
      // 第一部分第一道题禁止编辑
      if (currentEditingQuestion.part == 1 && currentEditingQuestion.sort_order == 1) {
        alert('这是系统默认题目，禁止编辑！');
        return;
      }
      
      // 填充表单
      document.getElementById('editQuestionId').value = id;
      document.getElementById('editQuestionText').value = currentEditingQuestion.question_text || '';
      document.getElementById('editQuestionType').value = currentEditingQuestion.question_type || 'single';
      document.getElementById('editCorrectAnswer').value = currentEditingQuestion.correct_answer || '';
      document.getElementById('editExplanation').innerHTML = currentEditingQuestion.explanation || '';
      document.getElementById('editAnnotationEnabled').checked = currentEditingQuestion.annotation_enabled !== false;
      
      // 处理选项
      const optionsList = document.getElementById('optionsList');
      optionsList.innerHTML = '';
      if (currentEditingQuestion.options && currentEditingQuestion.question_type !== 'text') {
        try {
          let options = JSON.parse(currentEditingQuestion.options);
          // 兼容双重转义的情况：如果解析后还是字符串，再解析一次
          if (typeof options === 'string') {
            options = JSON.parse(options);
          }
          if (Array.isArray(options)) {
            options.forEach(opt => addOption(opt));
          }
        } catch (e) {
          console.error('解析选项失败:', e);
        }
      }
      
      // 根据题目类型显示/隐藏相关字段
      const qType = currentEditingQuestion.question_type;
      if (qType === 'text') {
        document.getElementById('optionsGroup').style.display = 'none';
        document.getElementById('correctAnswerGroup').style.display = 'none';
      } else {
        document.getElementById('optionsGroup').style.display = 'block';
        if (qType === 'judge' && optionsList.children.length === 0) {
          // 判断题默认选项
          addOption('对');
          addOption('错');
        }
        if (qType === 'text') {
          document.getElementById('correctAnswerGroup').style.display = 'none';
        } else {
          document.getElementById('correctAnswerGroup').style.display = 'block';
        }
      }
      updateAnnotationControlVisibility();
      
      // 绑定类型切换事件
      document.getElementById('editQuestionType').onchange = function() {
        const qType = this.value;
        if (qType === 'text') {
          document.getElementById('optionsGroup').style.display = 'none';
          document.getElementById('correctAnswerGroup').style.display = 'none';
        } else {
          document.getElementById('optionsGroup').style.display = 'block';
          document.getElementById('correctAnswerGroup').style.display = 'block';
          // 如果是判断题且没有选项，自动添加对/错
          if (qType === 'judge' && document.querySelectorAll('.option-item').length === 0) {
            addOption('对');
            addOption('错');
          }
        }
        updateAnnotationControlVisibility();
      };
      
      // 打开模态框
      document.getElementById('questionModal').classList.remove('hidden');
    }
  } catch (error) {
    alert('加载题目详情失败: ' + error.message);
  }
}

// 答案解析富文本格式化
function formatExplanation(type) {
  const inputEl = document.getElementById('editExplanation');
  inputEl.focus();
  
  if (type === 'bold') {
    document.execCommand('bold', false, null);
  } else if (type === 'red') {
    document.execCommand('foreColor', false, '#ff4757');
  }
}

async function toggleQuestionEnabled(id, enabled) {
  try {
    const res = await fetch(`${API_BASE}/teacher/question/${id}/enabled`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled })
    });
    const data = await safeFetchJson(res);
    if (data.success) {
      await loadQuestions();
      alert(enabled ? '题目已开启' : '题目已关闭');
    } else {
      alert(data.error || '更新失败');
    }
  } catch (error) {
    alert('更新失败: ' + error.message);
  }
}

// 保存题目
async function saveQuestion() {
  const questionId = document.getElementById('editQuestionId').value;
  const questionText = document.getElementById('editQuestionText').value.trim();
  const questionType = document.getElementById('editQuestionType').value;
  const correctAnswer = document.getElementById('editCorrectAnswer').value.trim();
  const explanation = document.getElementById('editExplanation').innerHTML.trim();
  const annotationEnabled = document.getElementById('editAnnotationEnabled')?.checked !== false;
  
  if (!questionId || !questionText) {
    alert('请填写题目内容！');
    return;
  }
  
  // 收集选项
  let options = null;
  if (questionType !== 'text') {
    const optionInputs = document.querySelectorAll('.option-input');
    options = Array.from(optionInputs).map(input => input.value.trim()).filter(v => v);
    if (options.length < 2) {
      alert('至少需要填写2个选项！');
      return;
    }
    options = JSON.stringify(options);
  }
  
  try {
    const res = await fetch(`${API_BASE}/teacher/question/${questionId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        questionType,
        questionText,
        options,
        correctAnswer,
        explanation,
        annotationEnabled
      })
    });
    
    const data = await safeFetchJson(res);
    if (data.success) {
      alert('题目保存成功！');
      closeQuestionModal();
      loadQuestions(); // 刷新题目列表
    } else {
      alert(data.error || '保存失败，请重试');
    }
  } catch (error) {
    alert('保存失败: ' + error.message);
  }
}

// 加载当前阶段
async function loadCurrentStage() {
  if (loadCurrentStageInFlight) return;

  const courseId = document.getElementById('stageCourseSelect').value;
  const className = document.getElementById('stageClassSelect').value;
  if (!courseId || !className) {
    document.getElementById('completionContent').innerHTML = '<p style="margin: 5px 0;">请先选择班级和课程查看完成情况</p>';
    return;
  }

  loadCurrentStageInFlight = true;

  try {
    await loadPartSettings(courseId);
    
    // 自动保存班级和课程的绑定关系
    try {
      await fetch(`${API_BASE}/teacher/bind-class-course`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ className, courseId })
      });
    } catch (e) {
      console.error('绑定班级课程失败:', e);
    }

    try {
      const res = await fetch(`${API_BASE}/teacher/stage/${courseId}?className=${encodeURIComponent(className)}`);
      const data = await safeFetchJson(res);
      
      if (data.success) {
        document.getElementById('currentStageText').textContent = formatStageLabel(data.data.stage);
      }
    } catch (error) {
      console.error('加载当前阶段失败:', error);
    }

    // 加载课堂提示语
    try {
      const tipRes = await fetch(`${API_BASE}/teacher/tip?className=${encodeURIComponent(className)}`);
      const tipData = await safeFetchJson(tipRes);
      if (tipData.success) {
        const tipInput = document.getElementById('tipInput');
        if (tipInput) {
          tipInput.innerHTML = tipData.data.content || '';
        }
      }
    } catch (error) {
      console.error('加载课堂提示语失败:', error);
    }

    // 加载完成情况统计
    try {
      // 加载统计数据
      const statsRes = await fetch(`${API_BASE}/teacher/stats/${courseId}?className=${encodeURIComponent(className)}`);
      const statsData = await safeFetchJson(statsRes);
      
      // 加载班级名单总人数
      const classRes = await fetch(`${API_BASE}/teacher/class-list/${courseId}/${encodeURIComponent(className)}`);
      const classData = await safeFetchJson(classRes);
      
      if (statsData.success && classData.success) {
        const stats = statsData.data;
        const classStudents = (Array.isArray(classData.data.students) ? classData.data.students : []).map(s => s.student_name);
        const totalClass = classStudents.length;
        const totalSubmitted = (Array.isArray(stats.students) ? stats.students : []).length;
        const notSubmitted = totalClass - totalSubmitted;
        
        let html = '';
        html += `<p style="margin: 5px 0;"><strong>班级总人数：</strong>${totalClass}人，已答题：${totalSubmitted}人，未提交：${notSubmitted}人</p>`;
        html += `<div style="height: 1px; background: #eee; margin: 10px 0;"></div>`;
        for (let i = 1; i <= 4; i++) {
          const part = stats[`part${i}`];
          const rate = totalClass > 0 ? Math.round(part.completed / totalClass * 100) : 0;
          html += `<p style="margin: 5px 0;"><strong>第${i}部分：</strong>${part.completed}/${totalClass} 人完成 (${rate}%)</p>`;
        }
        html += `<p style="margin: 10px 0 0 0; color: #666; font-size: 14px;">👉 停留在课程管理页时，每 10 秒自动刷新一次</p>`;
        document.getElementById('completionContent').innerHTML = html;
      }
    } catch (error) {
      console.error('加载完成情况失败:', error);
      document.getElementById('completionContent').innerHTML = '<p style="margin: 5px 0; color: #f56c6c;">加载完成情况失败，请重试</p>';
    }
  } finally {
    loadCurrentStageInFlight = false;
  }
}

// 设置阶段
async function setStage(stage) {
  const courseId = document.getElementById('stageCourseSelect').value;
  const className = document.getElementById('stageClassSelect').value;
  if (!courseId || !className) {
    alert('请先选择课程和班级');
    return;
  }
  if (Number(stage) > 0 && !isPartEnabled(stage)) {
    alert(`第${stage}部分当前已关闭，请先在题目管理中开启后再切换`);
    return;
  }
  
  if (!confirm(`确定要将${className}切换到${formatStageLabel(stage)}吗？`)) return;
  
  try {
    const res = await fetch(`${API_BASE}/teacher/stage`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ courseId, stage, className })
    });
    const data = await safeFetchJson(res);
    
    if (data.success) {
      alert(`已成功切换到${formatStageLabel(stage)}！`);
      loadCurrentStage();
    } else {
      alert(data.error || '切换失败');
    }
  } catch (error) {
    alert('切换失败: ' + error.message);
  }
}

// 关闭学生详情模态框
function closeStudentDetailModal() {
  document.getElementById('studentDetailModal').classList.add('hidden');
  document.getElementById('studentDetailContent').innerHTML = '';
}

function showStudentDetailByIndex(index) {
  const student = currentStatsStudents[index];
  if (!student) {
    alert('未找到学生详情，请刷新后重试');
    return;
  }
  showStudentDetail(student);
}

// 展示学生答题详情
function showStudentDetail(student) {
  const actualScore = student.actual_score !== null && student.actual_score !== undefined ? `${student.actual_score}/5` : '待评分';
  const actualSource = student.actual_score_source === 'teacher' ? '教师评分' : (student.actual_score_source === 'part3' ? '第三部分小测' : '无');
  let html = '';
  html += `<div style="margin-bottom: 20px; padding: 15px; background: #f5f7fa; border-radius: 8px;">
    <h4 style="margin: 0 0 10px 0;">基本信息</h4>
    <p><strong>班级：</strong>${student.class_name}</p>
    <p><strong>姓名：</strong>${student.student_name}</p>
    <p><strong>最终实际分：</strong>${actualScore}（${actualSource}）</p>
    <p><strong>教师评分备注：</strong>${student.teacher_score_note || '无'}</p>
    <p><strong>提交时间：</strong>${student.submitted_at || '未完成全部提交'}</p>
  </div>`;

  // 第一部分
  if (student.part1_answers) {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #e8f4fd; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">📝 第一部分：上课前</h4>
      <p><strong>预测得分：</strong>${student.part1_answers.predictionScore}分</p>
      <p><strong>学习方法：</strong>${student.part1_answers.learningMethods ? student.part1_answers.learningMethods.join('、') : ''} ${student.part1_answers.customMethod ? `、${student.part1_answers.customMethod}` : ''}</p>
    </div>`;
  } else {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #fff3f3; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">📝 第一部分：上课前</h4>
      <p style="color: #f56c6c;">未完成</p>
    </div>`;
  }

  // 第二部分
  if (student.part2_answer) {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #e8f4fd; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">💬 第二部分：思考与讨论</h4>
      <div class="teacher-part2-rich">${formatPart2AnswerRich(student.part2_answer)}</div>
    </div>`;
  } else {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #fff3f3; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">💬 第二部分：思考与讨论</h4>
      <p style="color: #f56c6c;">未完成</p>
    </div>`;
  }

  // 第三部分
  if (student.part3_answers) {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #e8f4fd; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">✍️ 第三部分：小测验</h4>
      <p><strong>得分：</strong>${student.part3_score}/5</p>
      <h5 style="margin: 10px 0 5px 0;">答题详情：</h5>
      <ul style="margin: 0; padding-left: 20px;">
        ${Object.entries(student.part3_answers).map(([q, a]) => `<li>${q}: ${a}</li>`).join('')}
      </ul>
    </div>`;
  } else {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #fff3f3; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">✍️ 第三部分：小测验</h4>
      <p style="color: #f56c6c;">未完成</p>
    </div>`;
  }

  // 第四部分
  if (student.part4_answers) {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #e8f4fd; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">🌟 第四部分：课后反思</h4>
      <p><strong>得分对比：</strong>实际得分 ${student.part4_answers.scoreCompare.actual}，预测得分 ${student.part4_answers.scoreCompare.predicted}</p>
      <h5 style="margin: 10px 0 5px 0;">学习原因/方法：</h5>
      <p>${student.part4_answers.q2.answers.join('、')} ${student.part4_answers.q2.custom ? `、${student.part4_answers.q2.custom}` : ''}</p>
      <h5 style="margin: 10px 0 5px 0;">改进计划：</h5>
      <p>${student.part4_answers.q3.answers.join('、')} ${student.part4_answers.q3.custom ? `、${student.part4_answers.q3.custom}` : ''}</p>
    </div>`;
  } else if (student.part3_answers) {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #fffbe6; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">🌟 第四部分：课后反思</h4>
      <p style="color: #e6a23c;">未完成（已完成第三部分）</p>
    </div>`;
  } else {
    html += `<div style="margin-bottom: 20px; padding: 15px; background: #fff3f3; border-radius: 8px;">
      <h4 style="margin: 0 0 10px 0;">🌟 第四部分：课后反思</h4>
      <p style="color: #f56c6c;">未完成</p>
    </div>`;
  }

  document.getElementById('studentDetailTitle').textContent = `${student.class_name} - ${student.student_name} 答题详情`;
  document.getElementById('studentDetailContent').innerHTML = html;
  document.getElementById('studentDetailModal').classList.remove('hidden');
}

// 页面加载完成后绑定事件
document.addEventListener('DOMContentLoaded', () => {
  // 绑定进度控制下拉变化事件
  document.getElementById('stageCourseSelect').addEventListener('change', loadCurrentStage);
  document.getElementById('stageClassSelect').addEventListener('change', loadCurrentStage);
  document.getElementById('questionCourseSelect').addEventListener('change', loadQuestions);
  document.getElementById('questionPartSelect').addEventListener('change', loadQuestions);
  document.getElementById('statsCourseSelect').addEventListener('change', loadStatsClasses);

  // 处理提示语输入框回车换行，统一用<br>保证格式一致
  const tipInput = document.getElementById('tipInput');
  tipInput.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      document.execCommand('insertHTML', false, '<br><br>');
    }
  });

  // 处理答题指引输入框回车换行
  const guideInput = document.getElementById('part2GuideInput');
  guideInput.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      document.execCommand('insertHTML', false, '<br><br>');
    }
  });

  // 处理答案解析输入框回车换行
  const explanationInput = document.getElementById('editExplanation');
  explanationInput.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      document.execCommand('insertHTML', false, '<br><br>');
    }
  });

  if (isCourseTabActive()) {
    startCourseAutoRefresh();
  }
});
// 导出统计数据
async function exportStats() {
  const courseId = document.getElementById("statsCourseSelect").value;
  const className = document.getElementById("statsClassSelect").value;
  if (!courseId) {
    alert("请先选择课程");
    return;
  }
  const url = `${API_BASE}/teacher/stats/${courseId}/export${className ? `?className=${encodeURIComponent(className)}` : ""}`;
  window.open(url, "_blank");
}

async function exportAllStats() {
  const url = `${API_BASE}/teacher/stats/export-all`;
  window.open(url, "_blank");
}

async function exportPredictionSummary() {
  const className = document.getElementById("statsClassSelect").value;
  const url = `${API_BASE}/teacher/stats/export-prediction-summary${className ? `?className=${encodeURIComponent(className)}` : ""}`;
  window.open(url, "_blank");
}

async function saveTeacherScore(index) {
  const student = currentStatsStudents[index];
  const courseId = document.getElementById("statsCourseSelect").value;
  if (!student || !courseId) {
    alert('缺少学生或课程信息');
    return;
  }

  const scoreInput = document.getElementById(`teacher-score-${index}`);
  const noteInput = document.getElementById(`teacher-score-note-${index}`);
  const rawScore = scoreInput ? scoreInput.value.trim() : '';
  let teacherScore = null;
  if (rawScore !== '') {
    teacherScore = Number(rawScore);
    if (!Number.isInteger(teacherScore) || teacherScore < 0 || teacherScore > 5) {
      alert('教师评分需填写 0-5 的整数，清空则表示取消教师评分');
      return;
    }
  }

  try {
    const res = await fetch(`${API_BASE}/teacher/manual-score`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        courseId: Number(courseId),
        studentId: student.student_id,
        teacherScore,
        note: noteInput ? noteInput.value.trim() : ''
      })
    });
    const data = await safeFetchJson(res);
    if (!data.success) {
      throw new Error(data.error || '保存失败');
    }
    alert('教师评分已保存');
    await loadStats();
  } catch (error) {
    alert('保存教师评分失败: ' + error.message);
  }
}

// 格式化答题指引文本
function formatGuideText(type) {
  const inputEl = document.getElementById('part2GuideInput');
  inputEl.focus();
  
  if (type === 'bold') {
    document.execCommand('bold', false, null);
  } else if (type === 'red') {
    document.execCommand('foreColor', false, '#ff4757');
  }
}

// 清空答题指引
function clearPart2Guide() {
  document.getElementById('part2GuideInput').innerHTML = '';
}

// 加载第二部分答题指引
async function loadPart2Guide() {
  const courseId = document.getElementById('questionCourseSelect').value;
  if (!courseId) return;
  
  try {
    const res = await fetch(`${API_BASE}/teacher/part2-guide/${courseId}`);
    const data = await safeFetchJson(res);
    if (data.success) {
      document.getElementById('part2GuideInput').innerHTML = data.data.guide || '';
    }
  } catch (error) {
    console.error('加载答题指引失败:', error);
  }
}

// 保存第二部分答题指引
async function savePart2Guide() {
  const courseId = document.getElementById('questionCourseSelect').value;
  const content = document.getElementById('part2GuideInput').innerHTML.trim();
  
  if (!courseId) {
    alert('请先选择课程');
    return;
  }
  
  try {
    const res = await fetch(`${API_BASE}/teacher/part2-guide/${courseId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content })
    });
    const data = await safeFetchJson(res);
    if (data.success) {
      alert('答题指引保存成功！学生端将实时显示');
    } else {
      alert(data.error || '保存失败，请重试');
    }
  } catch (error) {
    alert('保存失败: ' + error.message);
  }
}

// 富文本格式化
function formatText(type) {
  const inputEl = document.getElementById('tipInput');
  inputEl.focus();
  
  if (type === 'bold') {
    document.execCommand('bold', false, null);
  } else if (type === 'red') {
    document.execCommand('foreColor', false, '#ff4757');
  }
}

// 清空提示语
function clearTip() {
  document.getElementById('tipInput').innerHTML = '';
  // 强制提交空字符串，避免 contenteditable 残留 <br>
  submitTipExplicit('');
}

// 提交提示语（显式传入内容，用于清空场景）
async function submitTipExplicit(explicitContent) {
  const courseId = document.getElementById('stageCourseSelect').value;
  const className = document.getElementById('stageClassSelect').value;
  const content = explicitContent !== undefined ? explicitContent : document.getElementById('tipInput').innerHTML.trim();

  if (!courseId || !className) {
    alert('请先选择课程和班级');
    return;
  }

  try {
    const res = await fetch(`${API_BASE}/teacher/tip`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ courseId, className, content })
    });
    const data = await safeFetchJson(res);

    if (data.success) {
      alert('提示语提交成功！学生端将实时显示');
    } else {
      alert(data.error || '提交失败，请重试');
    }
  } catch (error) {
    alert('提交失败: ' + error.message);
  }
}

// 提交提示语
async function submitTip() {
  await submitTipExplicit();
}

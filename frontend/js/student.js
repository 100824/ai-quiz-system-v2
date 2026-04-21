// 学生端 JavaScript
let currentCourseId = null;
let currentStudentId = null;
let currentStudentName = null;
let currentClassName = null;
let currentStage = 1;
let surveyData = null;
let part3Score = null;
let predictionScore = null;
let pollingInterval = null;
let currentClassStudents = []; // 当前选择班级的学生名单
let historyScores = []; // 学生历史课程分数对比数据
let currentLessonNumber = 1; // 当前课程的课程序号
let part2Explanation = null; // 第二部分题目解析

const API_BASE = '/api';

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
}

// 初始化
document.addEventListener('DOMContentLoaded', () => {
  // 清除缓存
  localStorage.clear();
  sessionStorage.clear();
  
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
      const res = await fetch(`${API_BASE}/teacher/class-list/1/${encodeURIComponent(className)}`);
      
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

// 开始问卷
async function startSurvey() {
  const studentName = document.getElementById('student-name').value.trim();
  const className = document.getElementById('class-select').value;
  
  if (!studentName) {
    alert('请输入你的姓名！');
    return;
  }
  
  if (!className) {
    alert('请选择你的班级！');
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
      alert(errMsg);
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
      
      // 更新标题
      document.getElementById('course-title').textContent = `🎓 ${statusResult.data.courseName} - 问卷调查`;
      document.getElementById('lesson-name').textContent = statusResult.data.courseName;
      
      // 加载题目（加载成功后再切换页面）
      await loadAllQuestions();
      
      // 切换到问卷页面
      document.getElementById('login-page').classList.add('hidden');
      document.getElementById('survey-page').classList.remove('hidden');
      
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
    alert(`错误详情：${error.message || '网络错误，请重试！'}`);
  } finally {
    showLoading(false);
  }
}

// 加载所有部分的题目
async function loadAllQuestions() {
  for (let part = 1; part <= 3; part++) {
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
    let options = [];
    if (q.options) {
      try {
        options = JSON.parse(q.options);
        // 兼容双重转义的情况
        if (typeof options === 'string') {
          options = JSON.parse(options);
        }
        if (!Array.isArray(options)) {
          options = [];
        }
      } catch (e) {
        console.error('解析题目选项失败:', e);
        options = [];
      }
    }
    
    if (part === 1) {
      // 第一部分：评分题+多选题
      if (qNum === 1) {
        // 第一题是评分题
        html += `
          <h3>题目一：猜一猜 🤔</h3>
          <p style="margin-bottom: 20px; color: #666;">${q.question_text}</p>
          <div class="rating-options" id="prediction-score">
            ${options.map((opt, i) => `
              <div class="rating-option" data-score="${i}">
                <div class="emoji">${['😰', '😟', '😅', '🤔', '😊', '🌟'][i]}</div>
                <div class="score">${i}分</div>
                <div class="desc">${opt.split(' - ')[1]}</div>
              </div>
            `).join('')}
          </div>
        `;
      } else {
        // 第二题是多选题
        html += `
          <h3 style="margin-top: 30px;">题目二：我的计划 📋</h3>
          <p style="margin-bottom: 20px; color: #666;">${q.question_text}</p>
          <div class="options" id="learning-methods">
            ${options.map((opt, i) => `
              <div class="option">
                <input type="checkbox" id="method${i}" value="${opt}">
                <label for="method${i}">${opt}</label>
              </div>
            `).join('')}
            <div class="option">
              <input type="checkbox" id="method-custom-check">
              <label for="method-custom-check">其他：</label>
              <input type="text" id="method-custom" placeholder="请写下你的方法..." style="margin-left: 10px; flex: 1;">
            </div>
          </div>
        `;
      }
    } else if (part === 2) {
      // 加载答题指引
      loadPart2Guide();
      // 第二部分：开放题
      part2Explanation = q.explanation || ''; // 保存解析到全局变量
      html += `
        <h3>${q.question_text}</h3>
        <p style="margin-bottom: 20px; color: #666;">请写下你的思考，越详细越好哦！</p>
        <div class="input-group">
          <textarea id="part2-answer" rows="8" placeholder="请在这里写下你的答案..."></textarea>
        </div>
        <!-- 解析区域，提交后显示 -->
        <div id="part2-results" class="hidden" style="margin-top: 30px; padding: 20px; background: #f0f9ff; border-radius: 8px;">
          <h4 style="color: #1890ff; margin-bottom: 10px;">💡 参考解析</h4>
          <p id="part2-explanation-content" style="line-height: 1.6; margin: 0;"></p>
        </div>
      `;
    } else if (part === 3) {
      // 第三部分：选择题/判断题
      html += `
        <h3>${qNum}. ${q.question_text}</h3>
        <div class="options" id="q${qNum}">
          ${options.map((opt, i) => `
            <div class="option">
              <input type="radio" id="q${qNum}-${i}" name="q${qNum}" value="${opt.split('.')[0]}">
              <label for="q${qNum}-${i}">${opt}</label>
            </div>
          `).join('')}
        </div>
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
    document.getElementById('part2-answer').value = surveyData.part2_answer;
    // 显示解析
    if (part2Explanation) {
      document.getElementById('part2-explanation-content').innerHTML = part2Explanation;
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
}

// 渲染历史分数对比
function renderHistoryScores() {
  const container = document.getElementById('history-scores-content');
  if (!container || historyScores.length === 0) {
    container.innerHTML = '<p style="text-align: center; color: #999; margin: 0;">暂无历史课程数据</p>';
    return;
  }

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

  // 渲染所有分数
  let html = '';
  allScores.forEach(item => {
    html += `
      <div style="display: flex; justify-content: space-between; align-items: center; padding: 8px 0; border-bottom: 1px solid #eaecef;">
        <div style="font-weight: 500;">${item.courseName}</div>
        <div style="text-align: right;">
          预测：<strong>${item.predictedScore}分</strong> / 实际：<strong>${item.actualScore}分</strong>
        </div>
      </div>
    `;
  });

  container.innerHTML = html;
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
    // 渲染历史分数对比
    renderHistoryScores();
    // 当教师已经开启第四部分时，隐藏等待提示
    if (currentStage >= 4) {
      document.getElementById('waiting-part4').classList.add('hidden');
    } else {
      document.getElementById('waiting-part4').classList.remove('hidden');
    }
  }
  
  // 计算学生当前需要完成的阶段（按提交顺序递增，必须按1→2→3→4依次完成）
  let currentStudentStage = 1;
  if (part1Submitted) currentStudentStage = 2;
  if (part1Submitted && part2Submitted) currentStudentStage = 3;
  if (part1Submitted && part2Submitted && part3Submitted) currentStudentStage = 4;

  // 显示当前学生需要填写的部分：如果教师开启的阶段 >= 学生当前要完成的阶段，就显示该阶段让学生填写
  if (currentStudentStage <= currentStage) {
    if (currentStudentStage === 1 && !part1Submitted) {
      document.getElementById('part1').classList.remove('hidden');
    } else if (currentStudentStage === 2 && !part2Submitted) {
      document.getElementById('part2').classList.remove('hidden');
    } else if (currentStudentStage === 3 && !part3Submitted) {
      document.getElementById('part3').classList.remove('hidden');
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
    } else if (currentStudentStage === 3 && currentStage === 3) {
      document.getElementById('waiting-part3').classList.remove('hidden');
    }
  }
}

// 生成第四部分内容
function generatePart4Content() {
  const container = document.getElementById('part4-content');
  if (!container || !part3Score || predictionScore === null) return;
  
  // 计算对比结果
  let reflection = '';
  let feedbackText = '';
  let feedbackClass = '';
  
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
  
  let html = `
    <h3>1. 我的学习成果 📊</h3>
    <div class="score-compare">
      <div class="score-item">
        <div class="label">小测得分</div>
        <div class="value">${part3Score}</div>
      </div>
      <div class="score-item">
        <div class="label">预测分数</div>
        <div class="value">${predictionScore}</div>
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
  } else {
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
    alert('请先给自己打分！');
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
    alert('请至少选择一种学习方法！');
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
      
      alert('第一部分提交成功！🎉');
    } else {
      alert(result.error || '提交失败，请重试');
    }
  } catch (error) {
    console.error('提交失败:', error);
    alert(`提交失败: ${error.message || '请重试'}`);
  } finally {
    showLoading(false);
  }
}

// 提交第二部分
async function submitPart2() {
  const answer = document.getElementById('part2-answer').value.trim();
  if (!answer) {
    alert('请填写你的答案！');
    return;
  }
  
  try {
    showLoading(true);
    
    const res = await fetch(`${API_BASE}/student/${encodeURIComponent(currentStudentId)}/part2`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        courseId: currentCourseId,
        answer
      })
    });
    
    if (!res.ok) {
      throw new Error(`提交请求失败，状态码: ${res.status}`);
    }
    
    const result = await safeFetchJson(res);
    
    if (result.success) {
      // 更新本地数据
      surveyData.part2_answer = answer;
      
      // 显示解析
      if (result.data.explanation) {
        part2Explanation = result.data.explanation;
      }
      if (part2Explanation) {
        document.getElementById('part2-explanation-content').innerHTML = part2Explanation;
        document.getElementById('part2-results').classList.remove('hidden');
      }
      
      // 锁定第二部分
      lockPart(2);
      
      // 更新UI
      updateUI();
      
      alert('第二部分提交成功！🎉');
    } else {
      alert(result.error || '提交失败，请重试');
    }
  } catch (error) {
    console.error('提交失败:', error);
    alert(`提交失败: ${error.message || '请重试'}`);
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
    alert('请回答所有问题！');
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
      surveyData.part3_answers = answers;
      surveyData.part3_score = part3Score;
      
      // 锁定第三部分
      lockPart(3);
      
      // 显示结果
      document.getElementById('part3-results').classList.remove('hidden');
      document.getElementById('total-score').textContent = `${part3Score}/${result.data.total}`;
      // 渲染历史分数对比
      renderHistoryScores();
      
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
            <p>你的答案：<strong>${res.studentAnswer}</strong></p>
            <p>正确答案：<strong>${res.correctAnswer}</strong></p>
            <p class="explanation">解析：<span>${res.explanation || ''}</span></p>
          </div>
        `;
      });
      resultsContainer.innerHTML = html;
      
      // 将解析内容用innerHTML渲染，支持加粗、标红、换行
      result.data.results.forEach((res, index) => {
        if (res.explanation) {
          const explanationSpan = resultsContainer.querySelectorAll('.explanation span')[index];
          if (explanationSpan) {
            explanationSpan.innerHTML = res.explanation;
          }
        }
      });
      
      // 更新UI
      updateUI();
      
      alert(`第三部分提交成功！你得了 ${part3Score}/5 分！🎉`);
    } else {
      alert(result.error || '提交失败，请重试');
    }
  } catch (error) {
    console.error('提交失败:', error);
    alert(`提交失败: ${error.message || '请重试'}`);
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
    alert('请回答第二题！');
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
    alert('请回答第三题！');
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
            actual: part3Score,
            predicted: predictionScore
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
      surveyData.part4_answers = true;
      
      // 锁定第四部分
      lockPart(4);
      
      // 显示完成页面
      document.getElementById('completed-page').classList.remove('hidden');
      
      alert('太棒了！你完成了所有问卷！🎉');
    } else {
      alert(result.error || '提交失败，请重试');
    }
  } catch (error) {
    console.error('提交失败:', error);
    alert(`提交失败: ${error.message || '请重试'}`);
  } finally {
    showLoading(false);
  }
}

// 加载提示语
async function loadTip() {
  if (!currentClassName) return;
  
  try {
    const res = await fetch(`${API_BASE}/student/tip?className=${encodeURIComponent(currentClassName)}`);
    if (!res.ok) return;
    
    const result = await safeFetchJson(res);
    if (result.success && result.data.content) {
      document.getElementById('tipContent').innerHTML = result.data.content;
    } else {
      document.getElementById('tipContent').innerHTML = '欢迎进入课堂，等待老师发布提示~';
    }
  } catch (error) {
    console.error('加载提示语失败:', error);
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
        
        if (newStage !== currentStage || JSON.stringify(newSurvey) !== JSON.stringify(surveyData) || JSON.stringify(newHistoryScores) !== JSON.stringify(historyScores)) {
          currentStage = newStage;
          surveyData = newSurvey;
          historyScores = newHistoryScores;
          currentLessonNumber = result.data.lessonNumber || 1;
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

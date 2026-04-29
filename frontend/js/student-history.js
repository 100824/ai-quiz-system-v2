const API_BASE = window.APP_CONFIG?.apiBase || `${window.location.protocol}//${window.location.hostname}:8080/api`;

function getQueryParam(name) {
  return new URLSearchParams(window.location.search).get(name) || '';
}

function safeText(value, fallback = '未提交') {
  const text = String(value ?? '').trim();
  return text || fallback;
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderRichContent(content) {
  const text = String(content ?? '').trim();
  if (!text) return '未提供';
  return text
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/\n/g, '<br>');
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

function formatDateTime(value) {
  const text = String(value ?? '').trim();
  if (!text) return '未知时间';
  return text.replace('T', ' ').slice(0, 16);
}

function formatPart2Answer(answer) {
  if (!answer) return '未提交';
  if (typeof answer !== 'string') return safeText(answer);
  try {
    const parsed = JSON.parse(answer);
    if (Array.isArray(parsed.responses) && parsed.responses.length > 0) {
      return parsed.responses.map((item) => formatPart2ResponseLabel(item)).join('；');
    }
    if (Array.isArray(parsed.labels) && parsed.labels.length > 0) {
      return parsed.labels.join('、');
    }
    if (parsed.label) return parsed.label;
    if (Array.isArray(parsed.values) && parsed.values.length > 0) {
      return parsed.values.join('、');
    }
    if (parsed.value) return parsed.value;
  } catch (error) {
    return safeText(answer);
  }
  return safeText(answer);
}

function formatPart2ResponseLabel(item) {
  if (Array.isArray(item?.labels) && item.labels.length > 0) {
    return item.labels.join('、');
  }
  if (item?.label) return item.label;
  if (Array.isArray(item?.values) && item.values.length > 0) {
    return item.values.join('、');
  }
  if (item?.value) return item.value;
  return '未提交';
}

function parsePart2Responses(answer) {
  if (!answer || typeof answer !== 'string') return [];
  try {
    const parsed = JSON.parse(answer);
    if (Array.isArray(parsed.responses) && parsed.responses.length > 0) {
      return parsed.responses;
    }
    return [parsed];
  } catch (error) {
    return [{ questionType: 'text', value: answer, label: answer }];
  }
}

function formatPart2AnswerRich(answer) {
  if (!answer) return '未提交';
  if (typeof answer !== 'string') return safeText(answer);
  try {
    const parsed = JSON.parse(answer);
    if (Array.isArray(parsed.responses) && parsed.responses.length > 0) {
      return parsed.responses.map((item) => {
        const value = item.questionType === 'text' && item.value
          ? item.value
          : escapeHtml(formatPart2ResponseLabel(item));
        return `<div><strong>${escapeHtml(item.questionText || '第二部分题目')}</strong><br>${value}</div>`;
      }).join('<br>');
    }
    if (parsed.questionType === 'text' && parsed.value) {
      return parsed.value;
    }
  } catch (error) {
    return escapeHtml(safeText(answer));
  }
  return escapeHtml(formatPart2Answer(answer));
}

function formatChoiceLabel(options, value) {
  const normalized = String(value ?? '').trim();
  if (!normalized) return '未提交';
  const exact = options.find((option) => option === normalized);
  if (exact) return exact;
  const prefixed = options.find((option) => option.startsWith(`${normalized}.`) || option.startsWith(`${normalized}、`));
  return prefixed || normalized;
}

function buildStatusTag(completed) {
  if (completed) {
    return '<span class="history-status history-status--done">已完成</span>';
  }
  return '<span class="history-status history-status--pending">未完成</span>';
}

function buildAnswerRow(label, value, extraClass = '') {
  return `
    <div class="history-answer-row">
      <div class="history-answer-label">${label}</div>
      <div class="history-answer-value ${extraClass}">${value}</div>
    </div>
  `;
}

function buildOptionsBlock(options) {
  if (!options.length) return '';
  return `
    <div class="history-options">
      <strong>选项：</strong>${options.map((option) => escapeHtml(option)).join(' / ')}
    </div>
  `;
}

function buildAnalysisBlock(content) {
  const hasContent = String(content ?? '').trim();
  if (!hasContent) return '';
  return `
    <div class="history-analysis">
      <p class="history-analysis__label">答案解析</p>
      <div class="history-analysis__content">${renderRichContent(content)}</div>
    </div>
  `;
}

function buildQuestionCard(title, questionText, rowsHtml, options, explanation, variant = '') {
  const variantClass = variant ? ` history-question-card--${variant}` : '';
  return `
    <div class="history-question-card${variantClass}">
      <p class="history-question-card__title">${title}</p>
      <p class="history-question-card__text">${escapeHtml(questionText)}</p>
      <div class="history-answer-grid">
        ${rowsHtml}
      </div>
      ${buildOptionsBlock(options)}
      ${buildAnalysisBlock(explanation)}
    </div>
  `;
}

function buildPartCard(title, completed, contentHtml, scoreLabel = '') {
  return `
    <section class="history-part-card">
      <div class="history-part-card__head">
        <h4 class="history-part-card__title">${title}</h4>
        ${buildStatusTag(completed)}
      </div>
      ${scoreLabel ? `<p class="history-part-card__score">${scoreLabel}</p>` : ''}
      <div class="history-question-list">
        ${contentHtml}
      </div>
    </section>
  `;
}

function renderPart1(part1, questions) {
  if (!part1) {
    return '<p class="history-empty-part">这一部分未完成。</p>';
  }

  const cards = [];
  const q1 = questions[0];
  const q2 = questions[1];

  if (q1) {
    cards.push(buildQuestionCard(
      '题目一',
      q1.question_text,
      buildAnswerRow('我的答案', `${escapeHtml(safeText(part1.predictionScore, '未提交'))} 分`),
      parseQuestionOptions(q1.options),
      q1.explanation,
      'cool'
    ));
  }

  if (q2) {
    const methods = Array.isArray(part1.learningMethods) && part1.learningMethods.length > 0
      ? part1.learningMethods.join('、')
      : '未提交';
    const extra = part1.customMethod ? `；其他：${escapeHtml(part1.customMethod)}` : '';
    cards.push(buildQuestionCard(
      '题目二',
      q2.question_text,
      buildAnswerRow('我的答案', `${escapeHtml(methods)}${extra}`),
      parseQuestionOptions(q2.options),
      q2.explanation,
      'cool'
    ));
  }

  return cards.join('');
}

function renderPart2(part2, questions) {
  if (!part2) {
    return '<p class="history-empty-part">这一部分未完成。</p>';
  }

  if (!questions.length) {
    return buildQuestionCard(
      '题目详情',
      '第二部分',
      buildAnswerRow('我的答案', escapeHtml(formatPart2Answer(part2))),
      [],
      '',
      'cool'
    );
  }

  const responses = parsePart2Responses(part2);
  return questions.map((q, index) => {
    const matched = responses.find((item) => item.questionId === q.id) || responses[index] || null;
    const value = matched
      ? (matched.questionType === 'text' && matched.value ? matched.value : escapeHtml(formatPart2ResponseLabel(matched)))
      : '未提交';
    return buildQuestionCard(
      `题目 ${index + 1}`,
      q.question_text,
      buildAnswerRow('我的答案', value),
      parseQuestionOptions(q.options),
      q.explanation,
      'cool'
    );
  }).join('');
}

function renderPart3(part3, score, questions) {
  if (!part3) {
    return '<p class="history-empty-part">这一部分未完成。</p>';
  }

  const cards = questions.map((question, index) => {
    const key = `q${index + 1}`;
    const options = parseQuestionOptions(question.options);
    const studentAnswer = formatChoiceLabel(options, part3[key]);
    const correctAnswer = formatChoiceLabel(options, question.correct_answer);
    const isCorrect = studentAnswer === correctAnswer && studentAnswer !== '未提交';

    return buildQuestionCard(
      `第 ${index + 1} 题`,
      question.question_text,
      [
        buildAnswerRow('我的答案', escapeHtml(studentAnswer), isCorrect ? 'history-answer-value--correct' : 'history-answer-value--wrong'),
        buildAnswerRow('正确答案', escapeHtml(correctAnswer), 'history-answer-value--correct')
      ].join(''),
      options,
      question.explanation,
      'warm'
    );
  });

  return buildPartCard(
    '第三部分：小测验',
    true,
    cards.join(''),
    `小测得分：${score !== null && score !== undefined ? `${score}` : '未评分'}`
  );
}

function renderPart4(part4) {
  if (!part4) {
    return '<p class="history-empty-part">这一部分未完成。</p>';
  }

  const q2Answers = Array.isArray(part4.q2?.answers) && part4.q2.answers.length > 0
    ? part4.q2.answers.join('、')
    : '未提交';
  const q3Answers = Array.isArray(part4.q3?.answers) && part4.q3.answers.length > 0
    ? part4.q3.answers.join('、')
    : '未提交';

  const cards = [
    buildQuestionCard(
      '第 2 题',
      '这节课哪些做法对你最有帮助？',
      buildAnswerRow(
        '我的答案',
        `${escapeHtml(q2Answers)}${part4.q2?.custom ? `；其他：${escapeHtml(part4.q2.custom)}` : ''}`
      ),
      [],
      '',
      'purple'
    ),
    buildQuestionCard(
      '第 3 题',
      '你之后准备怎样继续改进？',
      buildAnswerRow(
        '我的答案',
        `${escapeHtml(q3Answers)}${part4.q3?.custom ? `；其他：${escapeHtml(part4.q3.custom)}` : ''}`
      ),
      [],
      '',
      'purple'
    )
  ];

  return cards.join('');
}

function renderHistoryItem(item, questionMap) {
  const submittedAt = item.submitted_at || item.updated_at || '';
  const partQuestions = questionMap[item.course_id] || { 1: [], 2: [], 3: [] };
  const part1Done = !!item.part1_answers;
  const part2Done = !!item.part2_answer;
  const part3Done = !!item.part3_answers;
  const part4Done = !!item.part4_answers;

  return `
    <article class="card history-course-card">
      <div class="history-course-card__inner">
        <div class="history-course-card__header">
          <div>
            <h3 class="history-course-card__title">第 ${item.lesson_number} 课 · ${escapeHtml(item.course_name)}</h3>
            <p class="history-course-card__meta">
              班级：${escapeHtml(safeText(item.class_name))} ｜ 提交时间：${escapeHtml(formatDateTime(submittedAt))}
            </p>
          </div>
          <div class="history-pill-row">
            <span class="history-pill">课程编号 ${item.course_id}</span>
            <span class="history-pill history-pill--accent">
              ${item.part3_score !== null && item.part3_score !== undefined ? `小测 ${item.part3_score}` : '未测评'}
            </span>
          </div>
        </div>

        <div class="history-parts-grid">
          ${buildPartCard('第一部分：上课前', part1Done, renderPart1(item.part1_answers, partQuestions[1] || []))}
          ${buildPartCard('第二部分：思考与讨论', part2Done, renderPart2(item.part2_answer, partQuestions[2] || []))}
          ${part3Done ? renderPart3(item.part3_answers, item.part3_score, partQuestions[3] || []) : buildPartCard('第三部分：小测验', false, '<p class="history-empty-part">这一部分未完成。</p>')}
          ${buildPartCard('第四部分：课后反思', part4Done, renderPart4(item.part4_answers))}
        </div>
      </div>
    </article>
  `;
}

function renderSummary(items) {
  const totalLessons = items.length;
  const completedLessons = items.filter((item) => item.part4_answers || item.part3_answers || item.part2_answer || item.part1_answers).length;
  const scoredLessons = items.filter((item) => item.part3_score !== null && item.part3_score !== undefined);
  const averageScore = scoredLessons.length
    ? (scoredLessons.reduce((sum, item) => sum + Number(item.part3_score || 0), 0) / scoredLessons.length).toFixed(1)
    : '--';
  const latestLesson = items[0];

  return `
    <div class="history-summary__item">
      <p class="history-summary__label">已记录课堂</p>
      <p class="history-summary__value">${totalLessons}</p>
    </div>
    <div class="history-summary__item">
      <p class="history-summary__label">已完成课堂</p>
      <p class="history-summary__value">${completedLessons}</p>
    </div>
    <div class="history-summary__item">
      <p class="history-summary__label">小测平均分</p>
      <p class="history-summary__value">${averageScore}</p>
    </div>
    <div class="history-summary__item">
      <p class="history-summary__label">最近一课</p>
      <p class="history-summary__value">${latestLesson ? `第${latestLesson.lesson_number}课` : '--'}</p>
    </div>
  `;
}


async function safeFetchJson(response) {
  const text = await response.text();
  if (!text) throw new Error('服务器返回空响应');
  return JSON.parse(text);
}

async function loadHistoryDetails() {
  const studentId = getQueryParam('studentId');
  const courseId = getQueryParam('courseId');
  const studentName = getQueryParam('studentName');
  const className = getQueryParam('className');

  document.getElementById('history-page-subtitle').textContent = `${className || '未知班级'} · ${studentName || '未知学生'}`;

  if (!studentId || !courseId) {
    throw new Error('缺少学生或课程参数');
  }

  const res = await fetch(`${API_BASE}/student-history/${encodeURIComponent(studentId)}?courseId=${encodeURIComponent(courseId)}`);
  if (!res.ok) {
    throw new Error(`请求失败，状态码: ${res.status}`);
  }
  const result = await safeFetchJson(res);
  return result.data?.items || [];
}

async function loadCourseQuestionMap(courseIds) {
  const questionMap = {};
  await Promise.all(
    courseIds.flatMap((courseId) => [1, 2, 3].map(async (part) => {
      const res = await fetch(`${API_BASE}/student/questions/${part}?courseId=${encodeURIComponent(courseId)}`);
      if (!res.ok) {
        throw new Error(`加载第${courseId}课第${part}部分题目失败`);
      }
      const result = await safeFetchJson(res);
      if (!questionMap[courseId]) {
        questionMap[courseId] = { 1: [], 2: [], 3: [] };
      }
      questionMap[courseId][part] = result.data?.questions || [];
    }))
  );
  return questionMap;
}

document.addEventListener('DOMContentLoaded', async () => {
  try {
    const items = await loadHistoryDetails();
    document.getElementById('history-loading').classList.add('hidden');

    if (!items.length) {
      document.getElementById('history-empty').classList.remove('hidden');
      return;
    }

    const courseIds = [...new Set(items.map((item) => item.course_id))];
    const questionMap = await loadCourseQuestionMap(courseIds);

    const summary = document.getElementById('history-summary');
    summary.innerHTML = renderSummary(items);
    summary.classList.remove('hidden');

    // 直接渲染课程列表
    const historyList = document.getElementById('history-list');
    historyList.innerHTML = items.map((item) => renderHistoryItem(item, questionMap)).join('');
  } catch (error) {
    document.getElementById('history-loading').classList.add('hidden');
    document.getElementById('history-empty').classList.remove('hidden');
    document.getElementById('history-empty').innerHTML = `
      <div class="waiting-message">
        <div class="emoji">⚠️</div>
        <p>加载历史答题数据失败：${safeText(error.message, '请稍后重试')}</p>
      </div>
    `;
  }
});

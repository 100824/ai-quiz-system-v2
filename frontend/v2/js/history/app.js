import { api, apiBase } from '../core/api.js';
import { renderAnnotatedAnswer } from '../core/annotated-answer.js';
import { renderMarkdown } from '../core/markdown.js';
import { renderRichText } from '../core/rich-text.js?v=2026071101';

const $ = (id) => document.getElementById(id);

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function intText(value, fallback) {
  return value === null || value === undefined ? fallback : String(value);
}

function scoreText(value, fallback) {
  return value === null || value === undefined ? fallback : `${value} 分`;
}

function renderDetail(value) {
  return String(value ?? '').replace(/\n/g, '<br>');
}

function renderChatMessages(messages = []) {
  const normalized = (Array.isArray(messages) ? messages : [])
    .map((item) => ({
      role: String(item?.role || '').trim(),
      content: String(item?.content || '').trim()
    }))
    .filter((item) => item.content && (item.role === 'user' || item.role === 'assistant'));
  if (!normalized.length) return '<div class="history-empty-part">暂无 AI 对话记录。</div>';
  return `
    <div class="ai-chat-transcript">
      ${normalized.map((item) => `
        <div class="ai-chat-message ai-chat-message--${item.role === 'user' ? 'user' : 'assistant'}">
          <div class="ai-chat-message__role">${item.role === 'user' ? '我' : 'AI 学习助手'}</div>
          <div class="ai-chat-message__content">${renderMarkdown(item.content)}</div>
        </div>
      `).join('')}
    </div>
  `;
}

function scoreSourceLabel(value) {
  if (value === 'teacher') return '教师评分';
  if (value === 'quiz') return '小测';
  if (value === 'none') return '无';
  return value || '无';
}

function formatTime(value) {
  const text = String(value ?? '').trim();
  if (!text) return '-';
  return text.replace('T', ' ').slice(0, 16);
}

function courseModeLabel(mode) {
  const normalized = String(mode ?? '').trim();
  if (normalized === 'reflection') return '反思模式';
  if (normalized === 'free') return '自由模式';
  return normalized ? normalized : '其他模式';
}

// ===================== 汇总统计 =====================

function renderSummary(records) {
  const total = records.length;
  const completed = records.filter((r) => r.status === 'completed').length;
  const scored = records.filter((r) => r.actualScore !== null && r.actualScore !== undefined);
  const average = scored.length
    ? (scored.reduce((sum, r) => sum + Number(r.actualScore), 0) / scored.length).toFixed(1)
    : '--';

  const reflectionRecords = records.filter((r) => r.mode === 'reflection');
  const compareItems = reflectionRecords.filter((r) => r.predictedScore !== null && r.actualScore !== null);
  const compareSummary = compareItems.length ? compareItems.reduce((acc, r) => {
    const p = Number(r.predictedScore);
    const a = Number(r.actualScore);
    if (p === a) acc.equal++;
    else if (p > a) acc.high++;
    else acc.low++;
    return acc;
  }, { high: 0, equal: 0, low: 0 }) : null;

  return `
    <div class="history-summary__item">
      <p class="history-summary__label">已记录课堂</p>
      <p class="history-summary__value">${total}</p>
    </div>
    <div class="history-summary__item">
      <p class="history-summary__label">已完成课堂</p>
      <p class="history-summary__value">${completed}</p>
    </div>
    <div class="history-summary__item">
      <p class="history-summary__label">实际平均分</p>
      <p class="history-summary__value">${average}</p>
    </div>
    ${compareSummary ? `
      <div class="history-summary__item">
        <p class="history-summary__label">预测分对比</p>
        <p class="history-summary__value history-summary__value--stack">
          偏高 ${compareSummary.high} 次 ｜ 猜中 ${compareSummary.equal} 次 ｜ 偏低 ${compareSummary.low} 次
        </p>
      </div>
    ` : `
      <div class="history-summary__item">
        <p class="history-summary__label">模式概览</p>
        <p class="history-summary__value history-summary__value--stack">
          反思 ${reflectionRecords.length} 课 ｜ 其他 ${Math.max(0, total - reflectionRecords.length)} 课
        </p>
      </div>
    `}
  `;
}

// ===================== 分数对比表 =====================

function renderScoreCompare(records) {
  const items = Array.isArray(records) ? records : [];
  if (!items.length) return '';
  const hasReflectionRecord = items.some((r) => r.mode === 'reflection');

  const rows = items.map((r) => {
    const isReflection = r.mode === 'reflection';
    const hasCompare = isReflection
      && r.predictedScore !== null && r.predictedScore !== undefined
      && r.actualScore !== null && r.actualScore !== undefined;
    const p = hasCompare ? Number(r.predictedScore) : null;
    const a = hasCompare ? Number(r.actualScore) : null;
    const diff = hasCompare ? a - p : null;
    let diffText = '';
    let diffClass = 'history-diff--neutral';
    if (hasCompare) {
      if (diff > 0) { diffText = `+${diff} 偏低`; diffClass = 'history-diff--low'; }
      else if (diff < 0) { diffText = `${diff} 偏高`; diffClass = 'history-diff--high'; }
      else { diffText = '猜中'; diffClass = 'history-diff--equal'; }
    }

    return `
      <div class="history-score-row" data-submission-id="${r.submissionId || ''}" role="button" tabindex="0" aria-expanded="false">
        <div class="history-score-course">
          ${escapeHtml(r.courseTitle)}
          ${hasCompare ? '' : `<span class="history-score-course__meta">${escapeHtml(courseModeLabel(r.mode))}</span>`}
        </div>
        <div class="history-score-predicted">
          <span class="history-score-label">预测</span>
          <strong>${hasCompare ? `${p} 分` : '&nbsp;'}</strong>
        </div>
        <div class="history-score-actual">
          <span class="history-score-label">实际 · ${scoreSourceLabel(r.actualScoreSource)}</span>
          <strong>${hasCompare ? `${a} 分` : '&nbsp;'}</strong>
        </div>
        <div class="history-score-diff ${diffClass}">${diffText}</div>
        <div class="history-score-toggle">▼</div>
      </div>
      <div class="history-detail-panel hidden" data-submission-id="${r.submissionId || ''}"></div>
    `;
  }).join('');

  return `
    <h3 class="history-score-compare__title">📊 ${hasReflectionRecord ? '历史与预测分数对比' : '历史课堂记录'}</h3>
    <div class="history-score-list">${rows}</div>
  `;
}

// ===================== 详细答题面板 =====================

function renderQuestionStatus(isCorrect) {
  return isCorrect
    ? '<span class="history-status history-status--done">正确</span>'
    : '<span class="history-status history-status--wrong">错误</span>';
}

function renderDetailQuestion(question, index) {
  if (question.questionType === 'ai_chat' || question.chatMessages?.length) {
    return `
      <div class="history-question-card history-question-card--cool">
        <p class="history-question-card__title">第 ${index + 1} 题 · AI 对话题</p>
        <p class="history-question-card__text">${renderRichText(question.questionText || '')}</p>
        ${renderChatMessages(question.chatMessages || [])}
      </div>
    `;
  }
  if (question.questionType === 'open_text') {
    const explanationText = question.explanation ? renderDetail(question.explanation) : '';
    return `
      <div class="history-question-card history-question-card--open-text">
        <p class="history-question-card__title">第 ${index + 1} 题 · 颜色标注开放题</p>
        <p class="history-question-card__text">${renderRichText(question.questionText || '')}</p>
        <div class="history-answer-grid">
          <div class="history-answer-row history-answer-row--stacked">
            <div class="history-answer-label">我的答案</div>
            <div class="history-answer-value annotated-answer-content">${renderAnnotatedAnswer(question.answer)}</div>
          </div>
          ${explanationText ? `
          <div class="history-analysis">
            <p class="history-analysis__label">参考解析</p>
            <div class="history-analysis__content">${explanationText}</div>
          </div>` : ''}
        </div>
      </div>
    `;
  }
  const answerText = question.answer ? renderDetail(question.answer) : '未提交';
  const correctText = question.correctAnswer ? renderDetail(question.correctAnswer) : '-';
  const explanationText = question.explanation ? renderDetail(question.explanation) : '';

  return `
    <div class="history-question-card ${question.isCorrect ? 'history-question-card--cool' : 'history-question-card--warm'}">
      <p class="history-question-card__title">第 ${index + 1} 题 · ${escapeHtml(question.questionType || '')}</p>
      <p class="history-question-card__text">${renderRichText(question.questionText || '')}</p>
      <div class="history-answer-grid">
        <div class="history-answer-row">
          <div class="history-answer-label">我的答案</div>
          <div class="history-answer-value ${question.isCorrect ? 'history-answer-value--correct' : 'history-answer-value--wrong'}">${answerText}</div>
        </div>
        <div class="history-answer-row">
          <div class="history-answer-label">正确答案</div>
          <div class="history-answer-value history-answer-value--correct">${correctText}</div>
        </div>
        ${explanationText ? `
        <div class="history-analysis">
          <p class="history-analysis__label">答案解析</p>
          <div class="history-analysis__content">${renderDetail(question.explanation)}</div>
        </div>` : ''}
      </div>
    </div>
  `;
}

function renderDetailSection(section, index) {
  const attempts = Array.isArray(section.attempts) ? section.attempts : [];
  const questionsHtml = attempts.length
    ? attempts.map((attempt) => `
      <div class="history-detail-attempt">
        <div class="history-detail-attempt__head">
          <span>第 ${attempt.attemptNo || 1} 次提交</span>
          <span class="badge">${attempt.score !== null && attempt.score !== undefined ? `${attempt.score} 分` : '未评分'}</span>
        </div>
        <div class="meta" style="margin-bottom: 12px;">提交时间：${escapeHtml(formatTime(attempt.submittedAt))}</div>
        <div class="history-question-list">
          ${(Array.isArray(attempt.questions) ? attempt.questions : []).map((q, qi) => renderDetailQuestion(q, qi)).join('')}
        </div>
      </div>
    `).join('')
    : '<p class="history-empty-part">该部分暂无答题记录。</p>';

  return `
    <section class="history-part-card">
      <div class="history-part-card__head">
        <h4 class="history-part-card__title">第${index + 1}部分：${escapeHtml(section.title || '')}</h4>
        <span class="history-status ${section.completed ? 'history-status--done' : 'history-status--pending'}">${section.completed ? '已完成' : '未完成'}</span>
      </div>
      ${questionsHtml}
    </section>
  `;
}

function renderStudentDetail(detail) {
  if (!detail) return '<p class="history-empty-part">未找到答题详情。</p>';

  const sections = Array.isArray(detail.sections) ? detail.sections : [];
  const hasAnyAnswer = sections.some((s) => {
    const attempts = Array.isArray(s.attempts) ? s.attempts : [];
    return attempts.some((a) => Array.isArray(a.questions) && a.questions.length > 0);
  });

  if (!hasAnyAnswer) {
    return '<div class="history-detail-empty"><p class="history-empty-part">该学生尚未作答任何内容。</p></div>';
  }

  return `
    <div class="history-detail-header">
      <p><strong>课堂：</strong>${escapeHtml(detail.courseTitle || '-')}</p>
      <p><strong>班级：</strong>${escapeHtml(detail.className || '-')}</p>
      <p><strong>状态：</strong>${escapeHtml(detail.statusText || '-')}</p>
      ${detail.mode === 'reflection' ? `
      <p>
        <strong>预测分：</strong>${scoreText(detail.predictedScore, '未填写')}
        ｜ <strong>小测分：</strong>${scoreText(detail.quizScore, '待评分')}
        ｜ <strong>教师评分：</strong>${scoreText(detail.teacherScore, '未评分')}
        ｜ <strong>实际分：</strong>${scoreText(detail.actualScore, '待评分')}（${scoreSourceLabel(detail.actualScoreSource)}）
      </p>
      ` : `
      <p><strong>课堂模式：</strong>${escapeHtml(courseModeLabel(detail.mode))}</p>
      `}
      ${detail.teacherScoreNote ? `<p><strong>评分备注：</strong>${escapeHtml(detail.teacherScoreNote)}</p>` : ''}
      <p><strong>开始时间：</strong>${escapeHtml(formatTime(detail.startedAt))} ${detail.completedAt ? `｜完成时间：${escapeHtml(formatTime(detail.completedAt))}` : ''}</p>
    </div>
    <div class="history-parts-grid">
      ${sections.map((s, i) => renderDetailSection(s, i)).join('')}
    </div>
  `;
}

// ===================== 页面主流程 =====================

async function loadDetail(submissionId) {
  try {
    const data = await api(`/stats/student-detail?submissionId=${encodeURIComponent(submissionId)}`);
    return data.detail || null;
  } catch (error) {
    return null;
  }
}

document.addEventListener('DOMContentLoaded', async () => {
  const loadingEl = $('historyLoading');
  const emptyEl = $('historyEmpty');
  const summaryEl = $('historySummary');
  const scoreCompareEl = $('historyScoreCompare');

  // 最外层 try-catch：确保加载中状态一定能被关闭
  try {
    const params = new URLSearchParams(window.location.search);
    const classId = params.get('classId');
    const studentId = params.get('studentId');
    const className = params.get('className') || '';
    const studentName = params.get('studentName') || '';

    // 更新标题
    document.title = `${studentName} - 历史答题记录`;
    const subtitleEl = $('historySubtitle');
    if (subtitleEl) subtitleEl.textContent = `${className || '未知班级'} · ${studentName || '未知学生'}`;

    if (!classId || !studentId) {
      if (loadingEl) loadingEl.classList.add('hidden');
      if (emptyEl) {
        emptyEl.classList.remove('hidden');
        const inner = emptyEl.querySelector('.empty');
        if (inner) {
          inner.innerHTML = `
            <div style="font-size:32px;margin-bottom:12px;">📋</div>
            <p>请先从学生端选择班级并输入姓名，再点击「查看历史数据」。</p>
            <div style="margin-top:16px;"><a href="./student.html">返回学生端</a></div>
          `;
        }
      }
      return;
    }

    try {
      const data = await api(`/student-history?classId=${encodeURIComponent(classId)}&studentId=${encodeURIComponent(studentId)}`);
      const records = data.records || [];

      if (loadingEl) loadingEl.classList.add('hidden');

      if (!records.length) {
        if (emptyEl) emptyEl.classList.remove('hidden');
        return;
      }

      // 渲染汇总
      if (summaryEl) {
        summaryEl.innerHTML = renderSummary(records);
        summaryEl.classList.remove('hidden');
      }

      // 渲染分数对比表
      const scoreCompareHtml = renderScoreCompare(records);
      if (scoreCompareHtml && scoreCompareEl) {
        scoreCompareEl.innerHTML = scoreCompareHtml;
        scoreCompareEl.classList.remove('hidden');

        // 绑定展开/折叠
        scoreCompareEl.querySelectorAll('.history-score-row').forEach((row) => {
          row.addEventListener('click', async () => {
            const submissionId = row.getAttribute('data-submission-id');
            const panel = scoreCompareEl.querySelector(`.history-detail-panel[data-submission-id="${submissionId}"]`);
            const toggle = row.querySelector('.history-score-toggle');
            const isExpanded = row.getAttribute('aria-expanded') === 'true';

            if (!panel) return;

            if (isExpanded) {
              panel.classList.add('hidden');
              row.setAttribute('aria-expanded', 'false');
              if (toggle) toggle.style.transform = 'rotate(0deg)';
            } else {
              row.setAttribute('aria-expanded', 'true');
              if (toggle) toggle.style.transform = 'rotate(180deg)';
              if (!panel.innerHTML.trim()) {
                panel.innerHTML = '<div class="history-detail-loading">⏳ 正在加载答题详情...</div>';
                const detail = submissionId ? await loadDetail(submissionId) : null;
                panel.innerHTML = renderStudentDetail(detail);
              }
              panel.classList.remove('hidden');
            }
          });
        });
      } else if (scoreCompareEl) {
        scoreCompareEl.innerHTML = '';
        scoreCompareEl.classList.add('hidden');
      }
    } catch (apiError) {
      if (loadingEl) loadingEl.classList.add('hidden');
      if (emptyEl) {
        emptyEl.classList.remove('hidden');
        const inner = emptyEl.querySelector('.empty');
        if (inner) {
          inner.innerHTML = `
            <div style="font-size:32px;margin-bottom:12px;">⚠️</div>
            <p>加载历史答题数据失败：${escapeHtml(apiError.message)}</p>
          `;
        }
      }
    }
  } catch (error) {
    // 兜底：确保所有加载状态都被关闭，显示错误
    if (loadingEl) loadingEl.classList.add('hidden');
    if (emptyEl) {
      emptyEl.classList.remove('hidden');
      const inner = emptyEl.querySelector('.empty');
      if (inner) {
        inner.innerHTML = `
          <div style="font-size:32px;margin-bottom:12px;">⚠️</div>
          <p>页面加载失败：${escapeHtml(error.message || '未知错误')}</p>
        `;
      }
    } else {
      // 如果连 emptyEl 都不存在（可能是缓存旧 HTML），直接修改 body
      const body = document.body;
      if (body) {
        body.innerHTML = `
          <div style="text-align:center;padding:60px 20px;font-family:sans-serif;">
            <div style="font-size:32px;margin-bottom:12px;">⚠️</div>
            <p style="font-size:16px;color:#f56c6c;">页面加载失败：${escapeHtml(error.message || '未知错误')}</p>
            <p style="font-size:14px;color:#8fa4c0;">请尝试清除浏览器缓存后刷新页面。</p>
          </div>
        `;
      }
    }
  }
});

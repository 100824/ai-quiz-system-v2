import { api } from '../core/api.js';
import { escapeHtml as esc, versionStatus, comparable, planText, copyText } from './shared.js?v=2026110201';

const $ = (id) => document.getElementById(id);
const state = { students: [], student: null, classId: 0, plans: [], plan: null, versions: [], selectedVersion: 1, timer: null, request: 0, rosterRequest: 0, submitting: false };
function message(id, text, error = false) {
  const node = $(id); if (!node) return;
  node.textContent = text; node.className = error ? 'error-message' : 'success-message';
}
const query = () => `classId=${state.classId}&studentId=${state.student.id}`;

async function roster() {
  const request = ++state.rosterRequest;
  state.students = []; $('designName').value = ''; $('designNames').innerHTML = ''; $('designEnter').disabled = true;
  state.classId = Number($('designClass').value);
  if (!state.classId) return;
  try {
    const data = await api(`/classes/${state.classId}/students`);
    if (request !== state.rosterRequest) return;
    state.students = data.students || [];
    $('designNames').innerHTML = state.students.map((s) => `<option value="${esc(s.name)}"></option>`).join('');
    $('designEnter').disabled = false;
  } catch (error) { if (request === state.rosterRequest) message('designLoginMessage', error.message, true); }
}

async function loadPlans() {
  const data = await api(`/classes/${state.classId}/design-plans?studentId=${state.student.id}`);
  state.plans = data.plans;
  $('designPlanSelect').innerHTML = (state.plans.length > 1 ? '<option value="" disabled selected>请选择要填写或查看的方案</option>' : '') + state.plans.map((p) => `<option value="${p.id}">${esc(p.title)} · ${versionStatus(p.versionCount)}</option>`).join('');
  $('designPlanSelect').disabled = !state.plans.length;
  if (state.plans.length === 1) await openPlan();
  else if (state.plans.length > 1) $('designContent').innerHTML = '<div class="panel empty">请在上方选择一个方案。</div>';
  else { $('designContent').innerHTML = '<div class="panel empty">当前班级尚未绑定方案，请联系老师。</div>'; }
}

async function openPlan() {
  clearInterval(state.timer); state.timer = null;
  const request = ++state.request;
  state.plan = null; state.versions = [];
  $('designContent').innerHTML = '<div class="panel">正在加载方案…</div>';
  $('designNotice').textContent = '';
  try {
    const data = await api(`/design-plans/${Number($('designPlanSelect').value)}/student?${query()}`);
    if (request !== state.request) return;
    state.plan = data.plan; state.versions = data.versions;
    state.selectedVersion = state.versions.length === 2 ? 2 : 1;
    render();
    return true;
  } catch (error) {
    if (request === state.request) {
      message('designContent', `加载失败：${error.message}。`, true);
      const retry = document.createElement('button');
      retry.type = 'button'; retry.textContent = '重新加载方案'; retry.onclick = openPlan;
      $('designContent').append(retry);
    }
    return false;
  }
}

function remainingSeconds() {
  const submittedAt = state.versions[0]?.submittedAt;
  if (!submittedAt) return 0;
  return Math.max(0, Math.ceil((Date.parse(submittedAt) + 120000 - Date.now()) / 1000));
}

function render() {
  clearInterval(state.timer); state.timer = null;
  const first = state.versions[0];
  const complete = state.versions.length === 2;
  const wait = !!first && !complete && remainingSeconds() > 0;
  const selected = complete ? state.versions.find((v) => v.version === state.selectedVersion) : first;
  const questions = selected?.questions || state.plan.questions;
  const editable = !complete && !wait;
  const stage = complete ? 3 : first ? (wait ? 1 : 2) : 0;
  $('designContent').className = '';
  $('designContent').innerHTML = `<ol class="design-stepper" aria-label="方案设计进度">${['填写第一版', '与 AI 一起优化', '提交第二版'].map((label, i) => `<li class="${i < stage ? 'is-done' : i === stage ? 'is-current' : ''}" ${i === stage ? 'aria-current="step"' : ''}><span class="design-step-number">${i + 1}</span><span>${label}</span></li>`).join('')}</ol>
    <div class="panel design-document">
    <div class="design-document-heading"><h2>${esc(selected?.title || state.plan.title)}</h2>${wait ? '' : `<span class="design-state-badge ${complete ? 'is-complete' : ''}">${complete ? '已完成' : first ? '第二版' : '第一版'}</span>`}</div>
    ${!first ? '<p class="design-document-intro">请完成所有问题后提交。</p>' : editable ? '<p class="design-document-intro">请至少修改一道题的答案后提交。</p>' : ''}
    ${complete ? `<div class="design-version-picker"><label for="designVersion">查看我的方案</label><select id="designVersion"><option value="1" ${state.selectedVersion === 1 ? 'selected' : ''}>第一版方案</option><option value="2" ${state.selectedVersion === 2 ? 'selected' : ''}>第二版方案</option></select><span>仅供查看，不可修改</span></div>` : ''}
    ${first && !complete ? `<div class="design-wait"><div><h3>请与 AI 一起优化方案</h3><p>复制方案，到外部 AI 页面讨论改进。</p><p id="designCountdown" role="status"></p></div><button type="button" id="copyFirst" class="secondary">复制第一版整个方案</button></div>` : ''}
    <form id="designAnswersForm">
      ${questions.map((q, i) => `<div class="student-question-card design-question"><div class="design-question-heading"><span class="design-question-number">${String(i + 1).padStart(2, '0')}</span><h3>${esc(q)}</h3>${editable ? '<span class="design-required">必填</span>' : ''}</div>${editable ? `<textarea class="design-answer" data-answer="${i}" aria-label="第${i + 1}题答案" required placeholder="写下你的思考、具体做法或理由…">${esc(first?.answers[i] || '')}</textarea>` : `<div class="design-readonly">${esc(selected.answers[i])}</div>`}</div>`).join('')}
      <div class="design-actions">${editable ? `<button id="designSubmit" type="submit">提交第${first ? '二' : '一'}版方案</button>${first ? '<p>提交后不可修改。</p>' : ''}` : ''}${complete ? '<button type="button" id="copyFinal">复制整个方案</button>' : ''}</div>
      <p id="designSubmitMessage" role="status"></p>
    </form>
  </div>`;
  $('designAnswersForm').addEventListener('submit', submit);
  $('copyFirst')?.addEventListener('click', () => copy(first));
  $('copyFinal')?.addEventListener('click', () => copy(selected));
  $('designVersion')?.addEventListener('change', (event) => { state.selectedVersion = Number(event.target.value); render(); });
  if (first && !complete) {
    const tick = () => {
      const seconds = remainingSeconds();
      if ($('designCountdown')) $('designCountdown').textContent = seconds ? `${seconds} 秒后开放第二版编辑` : '';
      if (!seconds && wait) render();
    };
    tick();
    if (wait) state.timer = setInterval(tick, 500);
  }
}

async function copy(version) {
  try { await copyText(planText(version)); message('designNotice', '复制成功，已包含全部题目和答案。'); }
  catch (error) { message('designNotice', `复制失败：${error.message}`, true); }
}

async function submit(event) {
  event.preventDefault();
  if (state.submitting) return;
  const answers = Array.from(document.querySelectorAll('[data-answer]'), (node) => node.value.trim());
  const first = state.versions[0];
  const blank = answers.findIndex((a) => !a);
  if (blank >= 0) { message('designSubmitMessage', `第${blank + 1}题不能为空。`, true); return; }
  if (first && !answers.some((a, i) => comparable(a) !== comparable(first.answers[i]))) { message('designSubmitMessage', '请至少优化修改一道题的答案，仅调整空白不算修改。', true); return; }
  const button = $('designSubmit');
  const inputs = Array.from(document.querySelectorAll('[data-answer]'));
  inputs.forEach((node) => { node.disabled = true; });
  state.submitting = true; button.disabled = true; button.textContent = '提交中…'; $('designPlanSelect').disabled = true; $('designLogout').disabled = true;
  let saved = false;
  const title = state.plan.title;
  try {
    await api(`/design-plans/${state.plan.id}/versions`, { method: 'POST', body: JSON.stringify({ classId: state.classId, studentId: state.student.id, version: first ? 2 : 1, answers }) });
    saved = true;
    // Reload authoritative timestamps and versions. A failed reload must not offer a second submission.
    const loaded = await openPlan();
    const option = $('designPlanSelect').selectedOptions[0];
    option.textContent = `${title} · ${versionStatus(first ? 2 : 1)}`;
    message('designNotice', `第${first ? '二' : '一'}版方案提交成功。${loaded ? '' : '读取保存结果失败，请重新选择方案重试。'}`);
  } catch (error) {
    message('designSubmitMessage', `提交失败：${error.message}`, true);
  } finally {
    state.submitting = false; $('designPlanSelect').disabled = false; $('designLogout').disabled = false;
    if (!saved && button.isConnected) {
      button.disabled = false; button.textContent = `提交第${first ? '二' : '一'}版方案`;
      inputs.forEach((node) => { node.disabled = false; });
    }
  }
}

$('designClass').addEventListener('change', roster);
$('designPlanSelect').addEventListener('change', openPlan);
$('designLoginForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const student = state.students.find((s) => s.name === $('designName').value.trim());
  if (!student) { message('designLoginMessage', '请输入该班级名单中的完整姓名。', true); return; }
  $('designEnter').disabled = true; $('designClass').disabled = true;
  state.student = student;
  try {
    await loadPlans();
    $('designIdentity').innerHTML = `<span>${esc($('designClass').selectedOptions[0].textContent)} · ${esc(student.name)}</span><button type="button" class="secondary" id="designLogout">切换学生</button>`;
    $('designLogout').addEventListener('click', () => {
      clearInterval(state.timer); ++state.request; state.student = null;
      $('designWorkspace').classList.add('hidden'); $('designIdentity').classList.add('hidden'); $('designLogin').classList.remove('hidden'); $('designContent').innerHTML = ''; $('designName').value = ''; $('designLoginMessage').textContent = '';
    });
    $('designIdentity').classList.remove('hidden'); $('designWorkspace').classList.remove('hidden'); $('designLogin').classList.add('hidden');
  } catch (error) { message('designLoginMessage', error.message, true); }
  finally { $('designEnter').disabled = false; $('designClass').disabled = false; }
});

async function init() {
  try {
    const [data, defaults] = await Promise.all([api('/classes'), api('/default-class')]);
    const classes = (data.classes || []).sort((a,b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true }));
    $('designClass').innerHTML = classes.map((c) => `<option value="${c.id}">${esc(c.name)}</option>`).join('');
    if (classes.some((c) => c.id === defaults.defaultClassId)) $('designClass').value = String(defaults.defaultClassId);
    if (!classes.length) message('designLoginMessage', '暂无班级，请联系老师。', true);
    await roster();
  } catch (error) { message('designLoginMessage', `加载失败：${error.message}，请刷新页面重试。`, true); }
}
void init();

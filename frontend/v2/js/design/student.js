import { api } from '../core/api.js';
import { escapeHtml as esc, comparable, planText, copyText } from './shared.js?v=2026110811';
import { mountDesignImages } from './images.js?v=2026110811';
import { designAnswersError, attachDesignCounters, designLimits } from './limits.js?v=2026110811';

const $ = (id) => document.getElementById(id);
const state = { students: [], student: null, classId: 0, activePlanId: 0, plan: null, versions: [], selectedVersion: 1, timer: null, request: 0, rosterRequest: 0, submitting: false, stage: 1, stopImages: null };
function message(id, text, error = false) {
  const node = $(id); if (!node) return;
  node.textContent = text; node.className = error ? 'error-message' : 'success-message';
}
const query = () => `classId=${state.classId}&studentId=${state.student.id}`;
const drafts=new Map();
const creationDrafts = new Map();
const creationKey = () => `${state.classId}:${state.student?.id}:${state.activePlanId}`;
function stopCreation() {
  const draft = state.stopImages?.draft?.();
  if (state.stopImages?.key) { if (draft) creationDrafts.set(state.stopImages.key,draft); else creationDrafts.delete(state.stopImages.key); }
  state.stopImages?.(); state.stopImages = null;
}
const draftKey=()=>`${state.classId}:${state.student?.id}:${state.activePlanId}:${state.versions.length+1}`;
function rememberDraft(){if(!state.submitting&&state.plan&&state.versions.length<2){const inputs=[...document.querySelectorAll('[data-answer]')];if(inputs.length)drafts.set(draftKey(),inputs.map((n)=>n.value))}}

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
  rememberDraft();
  stopCreation();
  const token=++state.request;
  const data = await api(`/classes/${state.classId}/design-active-plan?studentId=${state.student.id}`);
  if(token!==state.request)return;
  state.activePlanId = data.plan?.id || 0;
  $('designPlanTitle').textContent = data.plan?.title || '等待老师指定方案';
  if (state.activePlanId) await openPlan();
  else {
    clearInterval(state.timer);stopCreation();state.plan=null;
    $('designContent').innerHTML = '<div class="panel empty">等待老师指定当前方案。</div>';
    state.timer=setInterval(pollProgress,3000);
  }
}

async function openPlan() {
  clearInterval(state.timer); state.timer = null;
  stopCreation();
  const request = ++state.request;
  state.plan = null; state.versions = [];
  $('designContent').innerHTML = '<div class="panel">正在加载方案…</div>';
  $('designNotice').textContent = '';
  try {
    const data = await api(`/design-plans/${state.activePlanId}/student?${query()}`);
    if (request !== state.request) return;
    state.plan = data.plan; state.versions = data.versions;
    state.stage = data.stage || 1;
    state.selectedVersion = state.versions.length === 2 ? 2 : 1;
    render();
    state.timer = setInterval(pollProgress, 3000);
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

async function pollProgress() {
  if (!state.student || state.submitting) return;
  const request = state.request, id = state.activePlanId;
  try {
    const active=await api(`/classes/${state.classId}/design-active-plan?studentId=${state.student.id}`);
    if(request!==state.request || state.submitting)return;
    if((active.plan?.id||0)!==id){await loadPlans();return}
    if(!id)return;
    const typeChanged=(active.plan?.outputType || 'image')!==(state.plan?.outputType || 'image');
    const data = await api(`/design-plans/${id}/classes/${state.classId}/progress`);
    if (request !== state.request || state.submitting) return;
    if(typeChanged){ rememberDraft(); stopCreation(); state.plan={...state.plan,outputType:active.plan.outputType}; }
    const old = state.stage; state.stage = data.stage;
    // Do not replace an unfinished editor when the teacher opens a later stage.
    if (typeChanged || (state.versions.length === 1 && old < 2 && data.stage >= 2) || (state.versions.length === 2 && old < 3 && data.stage >= 3)) {
      render();
      $('designRevisionBanner')?.scrollIntoView({behavior:'smooth',block:'start'});
    }
  } catch (error) { message('designNotice', `进度读取失败：${error.message}`, true); }
}

function render() {
  stopCreation();
  const first = state.versions[0];
  const complete = state.versions.length === 2;
  const wait = !!first && !complete && state.stage < 2;
  const selected = complete ? state.versions.find((v) => v.version === state.selectedVersion) : first;
  const questions = selected?.questions || state.plan.questions;
  const editable = !complete && !wait;
  const revising = !!first && editable;
  if (complete && state.stage >= 3) {
    $('designContent').className = '';
    state.stopImages = mountDesignImages({ node: $('designContent'), planId: state.plan.id, classId: state.classId, studentId: state.student.id, versions: state.versions, title: state.versions[1].title,outputType:state.plan.outputType || 'image',draft:creationDrafts.get(creationKey()) });
    state.stopImages.key=creationKey();
    return;
  }
  const stage = complete ? 2 : first ? (wait ? 0 : 1) : 0;
  $('designContent').className = '';
  $('designContent').innerHTML = `<ol class="design-stepper" aria-label="方案设计进度">${['第一版方案', '第二版方案', state.plan.outputType==='html'?'网页创作':'图片创作'].map((label, i) => `<li class="${i < stage ? 'is-done' : i === stage ? 'is-current' : ''}" ${i === stage ? 'aria-current="step"' : ''}><span class="design-step-number">${i + 1}</span><span>${label}</span></li>`).join('')}</ol>
    <div class="panel design-document ${revising ? 'design-document--revision' : ''}">
    <div class="design-document-heading"><h2>${esc(selected?.title || state.plan.title)}</h2>${wait ? '' : `<span class="design-state-badge ${complete ? 'is-complete' : ''}">${complete ? '已完成' : first ? '第二版' : '第一版'}</span>`}</div>
    ${!first ? '<p class="design-document-intro">请完成所有问题后提交。</p>' : ''}
    ${revising ? '<div id="designRevisionBanner" class="design-revision-banner" role="status"><span class="design-revision-number" aria-hidden="true">02</span><div><h3>第二版方案：修改与完善</h3><p>下方已填入你的第一版内容，请在此基础上修改，至少完善一道题后提交。</p></div></div>' : ''}
    ${complete ? `<div class="design-version-picker"><label for="designVersion">查看我的方案</label><select id="designVersion"><option value="1" ${state.selectedVersion === 1 ? 'selected' : ''}>第一版方案</option><option value="2" ${state.selectedVersion === 2 ? 'selected' : ''}>第二版方案</option></select><span>仅供查看，不可修改</span></div>` : ''}
    ${first && !complete ? `<div class="${wait ? 'design-wait' : 'design-revision-tools'}">${wait ? '<div><h3>等待老师开始第二版</h3><p>可复制方案，与外部 AI 讨论改进。</p></div>' : ''}<button type="button" id="copyFirst" class="secondary">复制第一版整个方案</button></div>` : ''}
    ${complete ? `<div class="design-wait">等待老师开启${state.plan.outputType==='html'?'网页':'图片'}环节。</div>` : ''}
    <form id="designAnswersForm" novalidate>
      ${questions.map((q, i) => `<div class="student-question-card design-question"><div class="design-question-heading"><span class="design-question-number">${String(i + 1).padStart(2, '0')}</span><h3>${esc(q)}</h3>${editable ? '<span class="design-required">必填</span>' : ''}</div>${revising ? `<details class="design-original-answer"><summary>查看第一版原文</summary><div class="design-readonly">${esc(first.answers[i])}</div></details><span class="design-revision-field-label">第二版内容</span>` : ''}${editable ? `<textarea class="design-answer" data-answer="${i}" aria-label="第${i + 1}题答案" required placeholder="写下你的思考、具体做法或理由…">${esc(drafts.get(draftKey())?.[i] ?? first?.answers[i] ?? '')}</textarea>` : `<div class="design-readonly">${esc(selected.answers[i])}</div>`}</div>`).join('')}
      <p id="designSubmitMessage" role="alert" aria-live="polite"></p>
      <div class="design-actions">${editable ? `<button id="designSubmit" type="submit">提交第${first ? '二' : '一'}版方案</button>${first ? '<p>提交后不可修改。</p>' : ''}` : ''}${complete ? '<button type="button" id="copyFinal">复制整个方案</button>' : ''}</div>
    </form>
  </div>`;
  attachDesignCounters($('designAnswersForm'),'[data-answer]',designLimits.answer);
  $('designAnswersForm').addEventListener('submit', submit);
  $('copyFirst')?.addEventListener('click', () => copy(first));
  $('copyFinal')?.addEventListener('click', () => copy(selected));
  $('designVersion')?.addEventListener('change', (event) => { state.selectedVersion = Number(event.target.value); render(); });
}

async function copy(version) {
  try { await copyText(planText(version)); message('designNotice', '复制成功，已包含全部题目和答案。'); }
  catch (error) { message('designNotice', `复制失败：${error.message}`, true); }
}

function isNetworkFailure(error) {
  return error instanceof TypeError || /failed to fetch|networkerror|network request failed|load failed|fetch failed/i.test(error?.message || '');
}

async function saveVersionWithRecovery(planId, body, button) {
  let recovering=false;
  for(let attempt=0;attempt<2;attempt++) {
    try {
      await api(`/design-plans/${planId}/versions`, {method:'POST',body:JSON.stringify(body)});
      return;
    } catch(error) {
      if(!isNetworkFailure(error) && !recovering)throw error;
      recovering=true;
      button.textContent='网络重连中…';
      // A lost response may still have committed. Read back before another write.
      let latest;
      try { latest=await api(`/design-plans/${planId}/student?classId=${body.classId}&studentId=${body.studentId}`); }
      catch { throw new Error('暂时无法连接服务，输入已保留，请稍后重试。'); }
      if(!Array.isArray(latest.versions))throw new Error('无法确认提交状态，输入已保留，请稍后重试。');
      if(latest.versions.some((version)=>version.version===body.version))return;
      if(attempt===1 || !isNetworkFailure(error)) {
        if(isNetworkFailure(error))throw new Error('提交时连接中断，输入已保留，请点击提交重试。');
        throw error;
      }
      button.textContent='重新提交中…';
    }
  }
}

async function submit(event) {
  event.preventDefault();
  const form=event.currentTarget;
  if (state.submitting || !form.isConnected || !state.plan || !state.student) return;
  const inputs = Array.from(form.querySelectorAll('[data-answer]'));
  const answers = inputs.map((node) => node.value.trim());
  const showError=(text)=>{message('designSubmitMessage',text,true);$('designSubmitMessage')?.scrollIntoView({behavior:'smooth',block:'center'});};
  const first = state.versions[0];
  const invalid=designAnswersError(inputs.map(input=>input.value));
  if(invalid){showError(invalid);return;}
  const blank = answers.findIndex((a) => !a);
  if (blank >= 0) { showError(`第${blank + 1}题不能为空。`); return; }
  if (first && !answers.some((a, i) => comparable(a) !== comparable(first.answers[i]))) { showError('请至少优化修改一道题的答案，仅调整空白不算修改。'); return; }
  const button = $('designSubmit');
  inputs.forEach((node) => { node.disabled = true; });
  state.submitting = true; button.disabled = true; button.textContent = '提交中…';
  let saved = false;
  const title = state.plan.title;
  try {
    await saveVersionWithRecovery(state.plan.id, {classId:state.classId,studentId:state.student.id,version:first?2:1,answers},button);
    saved = true;
    drafts.delete(draftKey());
    // Reload authoritative timestamps and versions. A failed reload must not offer a second submission.
    const loaded = await openPlan();
    $('designPlanTitle').textContent = title;
    message('designNotice', `第${first ? '二' : '一'}版方案提交成功。${loaded ? '' : '请点击重新加载方案读取结果。'}`);
  } catch (error) {
    if(form.isConnected)showError(`提交失败：${error.message}`);
  } finally {
    state.submitting = false;
    if (!saved && button.isConnected) {
      button.disabled = false; button.textContent = `提交第${first ? '二' : '一'}版方案`;
      inputs.forEach((node) => { node.disabled = false; });
    }
  }
}

$('designClass').addEventListener('change', roster);
$('designLoginForm').addEventListener('submit', async (event) => {
  event.preventDefault();
  const student = state.students.find((s) => s.name === $('designName').value.trim());
  if (!student) { message('designLoginMessage', '请输入该班级名单中的完整姓名。', true); return; }
  $('designEnter').disabled = true; $('designClass').disabled = true;
  state.student = student;
  try {
    await loadPlans();
    $('designIdentity').innerHTML = `<span>${esc($('designClass').selectedOptions[0].textContent)} · ${esc(student.name)}</span>`;
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

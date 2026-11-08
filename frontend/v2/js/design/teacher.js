import { api, apiBase } from '../core/api.js';
import { escapeHtml as esc, versionStatus, localTime } from './shared.js?v=2026110201';
import { imageURL, jobLabel } from './images.js?v=2026110801';

const $ = (id) => document.getElementById(id);
let plans = [], classes = [], current = null, records = [], saving = false, recordsRequest = 0, loading = false, dirty = false;
let progressRequest = 0, progressClassId = '', currentStage = 1;
const emptyPlan = () => ({ id: 0, title: '', questions: [''], classIds: [], locked: false });
function feedback(text, error = false) {
  const node = $('dpMessage'); if (!node) return;
  node.textContent = text; node.className = error ? 'error-message' : 'success-message';
  const status = $('dpSaveStatus');
  if (status) {
    status.textContent = text === '方案保存成功。' ? '保存成功，已显示在上方“我的方案”列表。' : text;
    status.className = error ? 'error-message' : 'meta';
  }
}

export async function loadDesignTeacher() {
  if (loading) return;
  // Tab navigation hides the existing editor; keep its unsaved input intact.
  if (dirty && $('dpForm')) return;
  loading = true;
  try {
    const [p, c] = await Promise.all([api('/design-plans'), api('/classes')]);
    plans = p.plans; classes = c.classes || [];
    current = plans.find((p) => p.id === current?.id) || plans[0] || emptyPlan();
    render();
    await loadRecords();
  } catch (error) {
    $('design').innerHTML = `<div class="panel error-message">加载方案设计失败：${esc(error.message)}<button type="button" id="dpReload">重新加载</button></div>`;
    $('dpReload').onclick = loadDesignTeacher;
  } finally { loading = false; }
}

function render() {
  ++recordsRequest;
  records = [];
  $('design').innerHTML = `
    <div class="panel design-library">
      <div class="design-panel-heading"><div><h2>我的方案 <span class="design-count">${plans.length}</span></h2><p class="meta">已保存的方案都在这里，点击即可编辑或查看学生记录。</p></div><button id="dpNew" type="button">＋ 新建方案</button></div>
      <div class="design-plan-list">${plans.length ? plans.map((p) => `<article class="design-plan-card ${p.id === current.id ? 'is-selected' : ''}">
        <h3>${esc(p.title)}</h3><p class="meta">${p.questions.length} 道问题 · ${p.classIds.length ? `已绑定 ${p.classIds.length} 个班级` : '尚未绑定班级'}</p>
        <p class="meta">${esc(p.classIds.map((id) => classes.find((c) => c.id === id)?.name).filter(Boolean).join('、')) || '绑定班级后，学生才能进入填写。'}</p>
        <div class="design-card-actions"><button type="button" class="secondary" data-dp-open="${p.id}">查看 / 编辑</button><button type="button" class="secondary" data-dp-open="${p.id}" data-dp-records>学生记录</button></div>
      </article>`).join('') : '<div class="empty">还没有保存的方案。点击“新建方案”，填写后保存，方案就会出现在这里。</div>'}</div>
    </div>
    <div class="grid">
    <div id="dpEditor" class="panel span-5 design-config-card">
      <div class="design-panel-heading"><div><h2>${current.id ? '编辑方案' : '创建新方案'}</h2><p id="dpSaveStatus" class="meta">${current.id ? '已保存，可在上方“我的方案”中找到。' : '尚未保存：填写标题和问题后，点击“保存方案”。'}</p></div><button id="dpSaveTop" type="submit" form="dpForm">保存方案</button></div>
      <label class="design-switch-label" for="dpSelect">快速切换方案</label><select id="dpSelect" aria-label="选择方案"><option value="0" ${!current.id ? 'selected' : ''}>创建新方案（未保存）</option>${plans.map((p) => `<option value="${p.id}" ${p.id === current.id ? 'selected' : ''}>${esc(p.title)}</option>`).join('')}</select>
      <form id="dpForm" class="form-row">
        <label for="dpTitle">方案标题</label><input id="dpTitle" value="${esc(current.title)}" required>
        <p class="meta">${current.locked ? '已有学生提交第一版，题目和顺序已锁定。可调整标题与班级绑定；需修改问题请新建方案。' : '全部为开放式问题，学生按以下顺序填写。'}</p>
        <div id="dpQuestions"></div>
        ${current.locked ? '' : '<button id="dpAdd" type="button" class="secondary">添加问题</button>'}
        <label>绑定班级</label><p class="meta">绑定后，在下方设为该班学生当前方案。解绑不删除提交记录。</p>
        <div class="design-bindings">${classes.map((c) => `<label><input type="checkbox" data-dp-class="${c.id}" ${current.classIds.includes(c.id) ? 'checked' : ''}>${esc(c.name)}</label>`).join('')}</div>
        <div class="inline-row"><button id="dpSave" type="submit">保存方案</button><a href="./design.html" target="_blank" rel="noopener">打开学生方案设计页</a></div>
        <p id="dpMessage" role="status"></p>
      </form>
    </div>
    <div class="panel span-12 design-progress-panel">
      <h2>方案进度</h2><p class="meta">按班级控制，只向后开放。</p>
      <label for="dpProgressClass">选择绑定班级</label><select id="dpProgressClass" ${current.id && current.classIds.length ? '' : 'disabled'}>${current.classIds.length ? current.classIds.map((id) => `<option value="${id}" ${String(id) === progressClassId ? 'selected' : ''}>${esc(classes.find((c)=>c.id===id)?.name || `班级 ${id}`)}</option>`).join('') : '<option value="">请先保存方案并绑定班级</option>'}</select>
      <p id="dpActivePlanText" role="status"></p><button id="dpSetActive" type="button" class="secondary" disabled>设为该班学生当前方案</button>
      <p id="dpProgressText" role="status"></p><div class="inline-row"><button id="dpAdvance" type="button" disabled>开始第二版</button><button id="dpProgressRefresh" type="button" class="secondary">刷新进度</button></div>
    </div>
    <div id="dpRecordsPanel" class="panel span-7">
      <h2>学生方案记录</h2><p class="meta">${current.id ? `当前方案：${esc(current.title)}` : '请先保存新方案，或在“我的方案”中选择已有方案。'}</p>
      <div class="form-row"><label for="dpClassFilter">班级</label><select id="dpClassFilter"><option value="">全部班级（含历史提交）</option>${classes.map((c) => `<option value="${c.id}">${esc(c.name)}</option>`).join('')}</select>
      <label for="dpStatusFilter">提交状态</label><select id="dpStatusFilter"><option value="">全部状态</option><option value="0">未提交</option><option value="1">仅第一版</option><option value="2">两版完成</option><option value="images">已生成未提交</option><option value="final">最终作品已提交</option></select>
      <div class="inline-row"><button id="dpRefresh" type="button" class="secondary">刷新记录</button><button id="dpExport" type="button" ${current.id ? '' : 'disabled'}>导出文字 Excel</button><button id="dpImageExport" type="button" ${current.id ? '' : 'disabled'}>导出全部图片 ZIP</button></div></div>
      <p id="dpRecordMessage" role="status"></p><div id="dpRecords"></div>
    </div>
  </div>`;
  renderQuestions();
  const discardDraft = () => !dirty || confirm('有尚未保存的方案配置，确定放弃这些修改吗？');
  const choosePlan = async (id, target = '') => {
    if (saving) return;
    if (!discardDraft()) { $('dpSelect').value = String(current.id); return; }
    dirty = false; current = plans.find((p) => p.id === id) || emptyPlan(); render(); await loadRecords();
    if (target) $(target)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };
  $('dpSelect').onchange = () => { void choosePlan(Number($('dpSelect').value)); };
  $('dpNew').onclick = () => { void choosePlan(0, 'dpEditor'); };
  document.querySelectorAll('[data-dp-open]').forEach((button) => { button.onclick = () => { void choosePlan(Number(button.dataset.dpOpen), button.hasAttribute('data-dp-records') ? 'dpRecordsPanel' : 'dpEditor'); }; });
  $('dpForm').addEventListener('input', markDirty);
  $('dpForm').addEventListener('change', markDirty);
  $('dpAdd')?.addEventListener('click', () => { readQuestions(); current.questions.push(''); markDirty(); renderQuestions(); });
  $('dpForm').onsubmit = save;
  $('dpClassFilter').onchange = loadRecords;
  $('dpStatusFilter').onchange = renderRecords;
  $('dpRefresh').onclick = loadRecords;
  $('dpExport').onclick = () => download(false);
  $('dpImageExport').onclick = () => download(true);
  $('dpProgressClass').onchange = loadProgress;
  $('dpProgressRefresh').onclick = loadProgress;
  $('dpAdvance').onclick = advanceProgress;
  $('dpSetActive').onclick = async () => {
    const button=$('dpSetActive');if(button.disabled)return;button.disabled=true;
    const id=current.id,cid=progressClassId;
    try{await api(`/classes/${cid}/design-active-plan`,{method:'PUT',body:JSON.stringify({planId:id})});if(button.isConnected)await loadProgress()}
    catch(error){if(button.isConnected){$('dpActivePlanText').textContent=`设置失败：${error.message}`;button.disabled=false}}
  };
  void loadProgress();
}

async function loadProgress() {
  const request = ++progressRequest;
  progressClassId = $('dpProgressClass').value;
  $('dpAdvance').disabled = true;
  $('dpSetActive').disabled = true;
  if (!current.id || !progressClassId) { $('dpProgressText').textContent = '保存并绑定班级后可控制进度。'; return; }
  try {
    const data = await api(`/design-plans/${current.id}/classes/${progressClassId}/progress`);
    if (request !== progressRequest) return;
    currentStage = data.stage;
    const active=await api(`/classes/${progressClassId}/design-active-plan`);if(request!==progressRequest)return;
    $('dpActivePlanText').textContent=`学生当前方案：${active.plan?.title || '尚未指定'}`;
    $('dpSetActive').disabled=active.plan?.id===current.id;
    $('dpSetActive').textContent=active.plan?.id===current.id?'正在向该班学生展示':'设为该班学生当前方案';
    $('dpProgressText').textContent = `当前进度：${['','第一版方案','第二版方案','图片环节'][currentStage]}`;
    $('dpAdvance').textContent = currentStage === 1 ? '开始第二版' : currentStage === 2 ? '开启图片环节' : '已全部开放';
    $('dpAdvance').disabled = currentStage >= 3;
  } catch (error) { if (request === progressRequest) $('dpProgressText').textContent = `读取失败：${error.message}`; }
}

async function advanceProgress() {
  const button = $('dpAdvance'); if (button.disabled) return;
  const id = current.id, classId = progressClassId, request = ++progressRequest;
  button.disabled = true; $('dpProgressClass').disabled = true;
  try {
    await api(`/design-plans/${id}/classes/${classId}/progress`, {method:'PUT',body:JSON.stringify({stage:currentStage+1})});
    if (request === progressRequest) await loadProgress();
  } catch (error) { if (request === progressRequest) $('dpProgressText').textContent = `切换失败：${error.message}，可刷新进度后重试。`; }
  finally { if (button.isConnected) $('dpProgressClass').disabled = false; }
}

function markDirty() {
  dirty = true;
  if ($('dpSaveStatus')) $('dpSaveStatus').textContent = '有未保存的修改，请点击“保存方案”。';
}

function readQuestions() {
  current = { ...current, questions: Array.from(document.querySelectorAll('[data-dp-question]'), (node) => node.value) };
}

function renderQuestions() {
  $('dpQuestions').innerHTML = current.questions.map((q, i) => `<div class="design-question-edit"><span>问题 ${i + 1}</span><textarea data-dp-question="${i}" aria-label="第${i + 1}道问题" placeholder="请输入学生需要回答的问题" ${current.locked ? 'readonly' : 'required'}>${esc(q)}</textarea>${current.locked ? '' : `<div class="design-question-tools"><button type="button" data-dp-up="${i}" ${i === 0 ? 'disabled' : ''}>上移</button><button type="button" data-dp-down="${i}" ${i === current.questions.length - 1 ? 'disabled' : ''}>下移</button><button type="button" data-dp-remove="${i}" class="secondary">删除</button></div>`}</div>`).join('');
  $('dpQuestions').onclick = (event) => {
    const button = event.target.closest('button'); if (!button || current.locked) return;
    markDirty();
    readQuestions();
    if (button.dataset.dpRemove !== undefined) current.questions.splice(Number(button.dataset.dpRemove), 1);
    else {
      const i = Number(button.dataset.dpUp ?? button.dataset.dpDown);
      const j = i + (button.dataset.dpUp !== undefined ? -1 : 1);
      [current.questions[i], current.questions[j]] = [current.questions[j], current.questions[i]];
    }
    renderQuestions();
  };
}

async function save(event) {
  event.preventDefault(); if (saving) return;
  readQuestions();
  const payload = { title: $('dpTitle').value.trim(), questions: current.questions.map((q) => q.trim()), classIds: Array.from(document.querySelectorAll('[data-dp-class]:checked'), (node) => Number(node.dataset.dpClass)) };
  if (!payload.title || !payload.questions.length || payload.questions.some((q) => !q)) { feedback('请填写标题，并至少添加一道非空问题。', true); return; }
  saving = true;
  feedback('正在保存方案…');
  // Freeze the editor during the request so the saved plan cannot overwrite new input.
  const controls = [...$('dpForm').querySelectorAll('input,textarea,button'), ...document.querySelectorAll('[data-dp-open],#dpSaveTop')];
  const disabledBefore = controls.map((node) => node.disabled);
  controls.forEach((node) => { node.disabled = true; }); $('dpSelect').disabled = true; $('dpNew').disabled = true;
  try {
    const data = await api(`/design-plans${current.id ? `/${current.id}` : ''}`, { method: current.id ? 'PUT' : 'POST', body: JSON.stringify(payload) });
    current = data.plan;
    const index = plans.findIndex((p) => p.id === current.id);
    if (index >= 0) plans[index] = current; else plans.unshift(current);
    dirty = false;
    render(); feedback('方案保存成功。'); await loadRecords();
  } catch (error) { feedback(`保存失败：${error.message}`, true); }
  finally {
    saving = false;
    controls.forEach((node, i) => { if (node.isConnected) node.disabled = disabledBefore[i]; });
    $('dpSelect').disabled = false; $('dpNew').disabled = false;
  }
}

async function loadRecords() {
  const request = ++recordsRequest;
  records = [];
  if (!current.id) { records = []; $('dpRecords').innerHTML = '<div class="empty">保存方案后即可查看学生记录。</div>'; return; }
  $('dpRecordMessage').textContent = '正在读取记录…'; $('dpRecords').innerHTML = '';
  try {
    const cid = $('dpClassFilter').value;
    const data = await api(`/design-plans/${current.id}/records${cid ? `?classId=${cid}` : ''}`);
    if (request !== recordsRequest) return;
    records = data.students; $('dpRecordMessage').textContent = `共 ${records.length} 名学生`;
    renderRecords();
  } catch (error) { if (request === recordsRequest) $('dpRecordMessage').textContent = `读取失败：${error.message}`; }
}

function renderRecords() {
  const status = $('dpStatusFilter').value;
  const filtered = records.filter((s) => !status || (status === 'images' ? s.imageCount > 0 && !s.finalImageId : status === 'final' ? !!s.finalImageId : String(s.versions.length) === status));
  $('dpRecords').innerHTML = filtered.length ? filtered.map((s) => {
    const first = s.versions[0], second = s.versions[1];
    return `<details class="design-record" data-student-id="${s.studentId}"><summary>${esc(s.className)} · ${esc(s.name)} · ${s.finalImageId ? '最终作品已提交' : s.imageCount ? '已生成未提交' : versionStatus(s.versions.length)}</summary>
      ${first ? `<p class="meta">第一版：${esc(localTime(first.submittedAt))}；第二版：${esc(localTime(second?.submittedAt))}</p><p>${esc(first.title)}</p>${first.questions.map((q, i) => `<h4>${i + 1}. ${esc(q)}</h4><div class="design-compare"><div><strong>第一版</strong><div class="design-readonly">${esc(first.answers[i])}</div></div><div><strong>第二版</strong><div class="design-readonly">${esc(second?.answers[i] ?? '尚未提交')}</div></div></div>`).join('')}` : '<p class="meta">尚未提交第一版方案。</p>'}
      <div class="design-actions"><button type="button" class="secondary" data-image-student="${s.studentId}" ${s.imageCount || s.versions.length === 2 ? '' : 'disabled'}>查看图片与对话</button></div><div data-image-details="${s.studentId}"></div>
    </details>`;
  }).join('') : '<div class="empty">暂无符合条件的学生。</div>';
  $('dpRecords').querySelectorAll('[data-image-student]').forEach((button) => { button.onclick = () => showStudentImages(button); });
}

async function showStudentImages(button) {
  const studentId = Number(button.dataset.imageStudent), planId = current.id;
  const container = $('dpRecords').querySelector(`[data-image-details="${studentId}"]`);
  button.disabled = true;
  let images = [], messages = [], beforeVersion = 0, beforeMessage = 0;
  const fetchPage = async () => {
    try {
      const suffix = `${beforeVersion ? `?beforeVersion=${beforeVersion}` : '?'}${beforeMessage ? `&beforeMessageId=${beforeMessage}` : ''}`;
      const data = await api(`/design-plans/${planId}/records/${studentId}/images${suffix}`);
      if (!container.isConnected) return;
      images = [...new Map([...images,...data.images].map((i)=>[i.id,i])).values()];
      messages = [...new Map([...messages,...data.messages].map((m)=>[m.id,m])).values()];
      const imageUrl = (id, dl = false) => imageURL(planId,studentId,0,id,true,dl);
      container.innerHTML = `<p class="meta">${data.finalImageId ? `最终提交：${esc(localTime(data.finalizedAt))}` : '尚未提交最终作品'}${data.activeJob ? ` · ${esc(jobLabel(data.activeJob.status))}` : ''}</p><div class="design-image-gallery">${[...images].sort((a,b)=>a.version-b.version).map((i)=>`<article class="design-image-thumb"><a href="${imageUrl(i.id)}" target="_blank" rel="noopener"><img loading="lazy" src="${imageUrl(i.id)}" alt="第${i.version}版"></a><strong>第${i.version}版${i.id===data.finalImageId ? ' · 最终作品' : i.id===data.latestImage?.id ? ' · 最新图片' : ''}</strong><a href="${imageUrl(i.id,true)}">下载原图</a><small>${esc(localTime(i.createdAt))}</small></article>`).join('') || '<p class="meta">尚无图片。</p>'}</div><div>${[...messages].sort((a,b)=>a.id-b.id).map((m)=>`<div class="design-chat-message"><strong>${m.role==='user'?'学生':'图片生成结果'} · ${esc(jobLabel(m.status))}</strong><time>${esc(localTime(m.createdAt))}</time><p>${esc(m.content)}</p></div>`).join('')}</div>${data.imagesHasMore||data.messagesHasMore ? '<button type="button" class="secondary" data-more>加载更多图片和对话</button>' : ''}`;
      if (data.sourcePlan) container.insertAdjacentHTML('afterbegin', `<details class="design-source"><summary>生图使用的内容</summary>${data.sourcePlan.questions.map((q,i)=>`<h4>${i+1}. ${esc(q)}</h4><p class="design-readonly">${esc(data.sourcePlan.answers[i])}</p>`).join('')}</details>`);
      beforeVersion = images.length ? Math.min(...images.map((i)=>i.version)) : 1;
      beforeMessage = messages.length ? Math.min(...messages.map((m)=>m.id)) : 1;
      container.querySelector('[data-more]')?.addEventListener('click', fetchPage);
    } catch (error) { if (container.isConnected) { container.textContent = `读取失败：${error.message}`; button.disabled = false; } }
  };
  await fetchPage();
  if (button.isConnected) { button.disabled = false; button.textContent = '刷新图片与对话'; }
}

async function download(images = false) {
  const button = $(images ? 'dpImageExport' : 'dpExport'); button.disabled = true;
  try {
    const cid = $('dpClassFilter').value;
    const response = await fetch(`${apiBase()}/design-plans/${current.id}/${images ? 'images-export' : 'export'}${cid ? `?classId=${cid}` : ''}`);
    if (!response.ok) { const data = await response.json().catch(() => ({})); throw new Error(data.error || '导出请求失败'); }
    const blob = await response.blob(); const url = URL.createObjectURL(blob); const link = document.createElement('a');
    link.href = url; link.download = `方案设计-${current.title}.${images ? 'zip' : 'xlsx'}`; document.body.append(link); link.click(); link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    $('dpRecordMessage').textContent = '导出成功，文件包含当前方案及所选班级的全部提交状态。';
  } catch (error) { $('dpRecordMessage').textContent = `导出失败：${error.message}`; }
  finally { if (button.isConnected) button.disabled = false; }
}

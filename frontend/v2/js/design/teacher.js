import { api, apiBase } from '../core/api.js';
import { escapeHtml as esc, versionStatus, localTime } from './shared.js?v=2026110201';

const $ = (id) => document.getElementById(id);
let plans = [], classes = [], current = null, records = [], saving = false, recordsRequest = 0, loading = false, dirty = false;
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
        <label>绑定班级</label><p class="meta">选中班级的学生可以看到本方案。不绑定班级则暂不开放；解绑不删除提交记录。</p>
        <div class="design-bindings">${classes.map((c) => `<label><input type="checkbox" data-dp-class="${c.id}" ${current.classIds.includes(c.id) ? 'checked' : ''}>${esc(c.name)}</label>`).join('')}</div>
        <div class="inline-row"><button id="dpSave" type="submit">保存方案</button><a href="./design.html" target="_blank" rel="noopener">打开学生方案设计页</a></div>
        <p id="dpMessage" role="status"></p>
      </form>
    </div>
    <div id="dpRecordsPanel" class="panel span-7">
      <h2>学生方案记录</h2><p class="meta">${current.id ? `当前方案：${esc(current.title)}` : '请先保存新方案，或在“我的方案”中选择已有方案。'}</p>
      <div class="form-row"><label for="dpClassFilter">班级</label><select id="dpClassFilter"><option value="">全部班级（含历史提交）</option>${classes.map((c) => `<option value="${c.id}">${esc(c.name)}</option>`).join('')}</select>
      <label for="dpStatusFilter">提交状态</label><select id="dpStatusFilter"><option value="">全部状态</option><option value="0">未提交</option><option value="1">仅第一版</option><option value="2">两版完成</option></select>
      <div class="inline-row"><button id="dpRefresh" type="button" class="secondary">刷新记录</button><button id="dpExport" type="button" ${current.id ? '' : 'disabled'}>导出 Excel</button></div></div>
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
  $('dpExport').onclick = download;
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
  const filtered = records.filter((s) => !status || String(s.versions.length) === status);
  $('dpRecords').innerHTML = filtered.length ? filtered.map((s) => {
    const first = s.versions[0], second = s.versions[1];
    return `<details class="design-record"><summary>${esc(s.className)} · ${esc(s.name)} · ${versionStatus(s.versions.length)}</summary>
      ${first ? `<p class="meta">第一版：${esc(localTime(first.submittedAt))}；第二版：${esc(localTime(second?.submittedAt))}</p><p>${esc(first.title)}</p>${first.questions.map((q, i) => `<h4>${i + 1}. ${esc(q)}</h4><div class="design-compare"><div><strong>第一版</strong><div class="design-readonly">${esc(first.answers[i])}</div></div><div><strong>第二版</strong><div class="design-readonly">${esc(second?.answers[i] ?? '尚未提交')}</div></div></div>`).join('')}` : '<p class="meta">尚未提交第一版方案。</p>'}
    </details>`;
  }).join('') : '<div class="empty">暂无符合条件的学生。</div>';
}

async function download() {
  const button = $('dpExport'); button.disabled = true;
  try {
    const cid = $('dpClassFilter').value;
    const response = await fetch(`${apiBase()}/design-plans/${current.id}/export${cid ? `?classId=${cid}` : ''}`);
    if (!response.ok) { const data = await response.json().catch(() => ({})); throw new Error(data.error || '导出请求失败'); }
    const blob = await response.blob(); const url = URL.createObjectURL(blob); const link = document.createElement('a');
    link.href = url; link.download = `方案设计-${current.title}.xlsx`; document.body.append(link); link.click(); link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    $('dpRecordMessage').textContent = '导出成功，文件包含当前方案及所选班级的全部提交状态。';
  } catch (error) { $('dpRecordMessage').textContent = `导出失败：${error.message}`; }
  finally { if (button.isConnected) button.disabled = false; }
}

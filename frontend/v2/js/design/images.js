import { api, apiBase } from '../core/api.js';
import { escapeHtml as esc, localTime, planText, copyText } from './shared.js?v=2026110201';

export function imageURL(planId, studentId, classId, imageId, teacher = false, download = false) {
  return `${apiBase()}/design-plans/${planId}/${teacher ? `records/${studentId}/` : ''}images/${encodeURIComponent(imageId)}?${teacher ? '' : `classId=${classId}&studentId=${studentId}&`}${download ? 'download=1' : ''}`;
}
export const jobLabel = (status) => ({ queued: '排队中', running: '生成中', succeeded: '完成', failed: '失败', interrupted: '中断' })[status] || status;
export function requestKey() { return Array.from(crypto.getRandomValues(new Uint8Array(16)), (n) => n.toString(16).padStart(2, '0')).join(''); }
const merge = (old, added) => [...new Map([...old, ...added].map((item) => [item.id, item])).values()];

export function mountDesignImages({ node, planId, classId, studentId, versions, title }) {
  let generatingRequest=false;
  let stopped = false, fetching = false, posting = false, loaded = false, selected = '', pending = null, sentJob = null, state = null, images = [], messages = [], signature = '', sourceVersion = 2, imagesComplete = false, messagesComplete = false, imageCursor = 0, messageCursor = 0;
  const query = `classId=${classId}&studentId=${studentId}`;
  const prefix = `/design-plans/${planId}`;
  const $ = (id) => node.querySelector(`#${id}`);
  const url = (id, download = false) => imageURL(planId, studentId, classId, id, false, download);
  node.innerHTML = `<div class="panel design-image-panel">
    <div class="design-document-heading"><h2>${esc(title)}</h2><span class="design-state-badge">图片创作</span></div>
    <details class="design-source"><summary>查看两版方案</summary><div class="design-version-picker"><label for="imageSourceVersion">查看版本</label><select id="imageSourceVersion"><option value="2">第二版方案</option><option value="1">第一版方案</option></select><button type="button" id="imageCopyPlan" class="secondary">复制方案</button></div><div id="imageSourceText"></div></details>
    <p id="imageFeedback" role="status"></p><p id="imageTaskStatus" role="status">正在读取图片记录…</p><button id="imageReload" type="button" class="secondary hidden">重新读取</button>
    <div id="imageGenerating" class="design-generation-status hidden" role="status" aria-live="polite"><span class="design-generation-spinner" aria-hidden="true"></span><div><h3 id="imageGeneratingTitle">正在生成图片中…</h3><p>通常需要几十秒，请耐心等待。</p></div></div>
    <div id="imageLatestArea" class="design-latest hidden"><h3><span id="imagePreviewLabel">最新图片</span> <span id="imageLatestVersion"></span></h3><a id="imageLatestLink" target="_blank" rel="noopener"><img id="imageLatest" alt="方案图片"></a><a id="imageLatestDownload" class="design-file-button">下载原图</a></div>
    <form id="imageCompose" class="design-image-compose"><section id="imagePlanEditor"><h3>确认生图内容</h3><p class="meta">已填入第二版方案，可以在生成前修改。两版已提交方案会保留。</p><div id="imagePlanFields"></div><label class="design-image-confirm"><input id="imagePlanConfirmed" type="checkbox">确认使用以上内容生成图片</label></section><label for="imageInstruction" id="imageInstructionLabel">补充要求（选填）</label><textarea id="imageInstruction" maxlength="4000" rows="3" placeholder="例如：科技风，标题突出，文字清晰"></textarea><button id="imageGenerate" type="submit" disabled>生成图片</button></form>
    <section class="design-image-history"><h3>图片版本</h3><div id="imageGallery" class="design-image-gallery"></div><button id="imageMore" type="button" class="secondary hidden">加载更多图片</button><div class="design-actions"><button id="imageFinalize" type="button" disabled>提交最终作品</button><span id="imageFinalStatus"></span></div></section>
  </div>`;
  const feedback = (text, error = false) => { if (stopped) return; $('imageFeedback').textContent = text; $('imageFeedback').className = error ? 'error-message' : 'success-message'; };
  const renderSource = () => { const v = versions.find((item) => item.version === sourceVersion); $('imageSourceText').innerHTML = v.questions.map((q, i) => `<h4>${i + 1}. ${esc(q)}</h4><p class="design-readonly">${esc(v.answers[i])}</p>`).join(''); };
  const renderPlanEditor = (v) => {
    $('imagePlanFields').innerHTML = v.questions.map((q, i) => `<div class="design-question"><label for="imagePlanAnswer${i}">${i + 1}. ${esc(q)}</label><textarea id="imagePlanAnswer${i}" class="design-answer" data-image-answer rows="4" disabled>${esc(v.answers[i])}</textarea></div>`).join('');
    $('imagePlanFields').querySelectorAll('textarea').forEach((input) => { input.oninput = () => { $('imagePlanConfirmed').checked = false; }; });
  };
  renderPlanEditor(versions.find((v) => v.version === 2));
  renderSource();
  $('imageSourceVersion').onchange = (event) => { sourceVersion = Number(event.target.value); renderSource(); };
  $('imageCopyPlan').onclick = async () => { try { await copyText(planText(versions.find((v) => v.version === sourceVersion))); feedback('复制成功。'); } catch (e) { feedback(e.message, true); } };
  function draw() {
    if (stopped || !state) return;
    const busy = posting || !!state.activeJob;
    const generating=!!state.activeJob || generatingRequest;
    $('imageGenerating').classList.toggle('hidden',!generating);
    $('imageGeneratingTitle').textContent=state.activeJob?.status==='queued'?'图片生成已排队…':'正在生成图片中…';
    $('imageCompose').setAttribute('aria-busy',String(generating));
    node.querySelector('.design-image-history').classList.toggle('hidden',!state.imageCount);
    $('imageGenerate').disabled = !loaded || busy || !!state.finalImageId;
    $('imageInstruction').disabled = busy || !!state.finalImageId;
    $('imagePlanEditor').classList.toggle('hidden', !!state.latestImage);
    $('imagePlanEditor').querySelectorAll('textarea,input').forEach((input) => { input.disabled = busy || !!state.finalImageId; });
    $('imageCompose').classList.toggle('hidden', !!state.finalImageId || generating);
    $('imageGenerate').textContent = busy ? (state.activeJob ? jobLabel(state.activeJob.status) + '…' : '提交中…') : state.latestImage ? '发送并生成' : '生成图片';
    $('imageInstructionLabel').textContent = state.latestImage ? `修改最新图片（第${state.latestImage.version}版）` : '补充要求（选填）';
    $('imageTaskStatus').textContent = generating ? '' : state.finalImageId ? '最终作品已提交。' : '';
    const preview = state.finalImage || state.latestImage;
    if (preview) {
      $('imageLatestArea').classList.remove('hidden');
      if ($('imageLatest').dataset.id !== preview.id) { $('imageLatest').src = url(preview.id); $('imageLatest').dataset.id = preview.id; }
      $('imagePreviewLabel').textContent = state.finalImageId ? '最终作品' : '最新图片';
      $('imageLatestLink').href = url(preview.id); $('imageLatestDownload').href = url(preview.id, true); $('imageLatestVersion').textContent = `第${preview.version}版`;
    }
    const nextSignature = JSON.stringify([images.map((i) => i.id), messages.map((m) => [m.id, m.status]), state.finalImageId]);
    if (signature !== nextSignature) {
      signature = nextSignature;
      $('imageGallery').innerHTML = images.length ? [...images].sort((a,b) => a.version-b.version).map((i) => `<article class="design-image-thumb"><a href="${url(i.id)}" target="_blank" rel="noopener"><img loading="lazy" src="${url(i.id)}" alt="第${i.version}版图片"></a><label><input type="radio" name="finalImage" value="${i.id}" ${state.finalImageId ? 'disabled' : ''} ${i.id === (state.finalImageId || selected) ? 'checked' : ''}>第${i.version}版${i.id === state.finalImageId ? ' · 最终作品' : ''}</label><a class="design-file-button" href="${url(i.id, true)}">下载原图</a></article>`).join('') : '<p class="meta">尚未生成图片。</p>';
      $('imageGallery').querySelectorAll('input').forEach((input) => { input.onchange = () => { selected = input.value; draw(); }; });
    }
    $('imageFinalize').disabled = !selected || busy || !!state.finalImageId;
    $('imageGallery').querySelectorAll('input').forEach((input) => { input.disabled = posting || !!state.finalImageId; });
    $('imageFinalize').classList.toggle('hidden', !!state.finalImageId);
    const finalImage = images.find((i) => i.id === state.finalImageId);
    $('imageFinalStatus').textContent = state.finalImageId ? `已提交最终作品${finalImage ? `：第${finalImage.version}版` : ''}，${localTime(state.finalizedAt)}` : state.imageCount ? '选择一张图片作为最终作品。' : '';
  }
  async function refresh(kind = '') {
    if (stopped || fetching) return; fetching = true;
    try {
      const older = kind === 'images' && imageCursor ? `&beforeVersion=${imageCursor}` : kind === 'messages' && messageCursor ? `&beforeMessageId=${messageCursor}` : '';
      const next = await api(`${prefix}/images?${query}${older}`); if (stopped) return;
      if (!loaded) {
        if (next.sourcePlan) renderPlanEditor(next.sourcePlan);
        const previous = next.activeJob || (['failed','interrupted'].includes(next.lastJob?.status) ? next.lastJob : null);
        if (previous) $('imageInstruction').value = ['按第二版方案生成图片。','按确认内容生成图片。'].includes(previous.message || previous.content) ? '' : previous.message || previous.content;
      }
      if (sentJob) {
        const result = next.lastJob?.id===sentJob.id ? next.lastJob : null;
        if (result && result.status === 'succeeded') { $('imageInstruction').value = ''; sentJob = null; }
        else if (result && ['failed','interrupted'].includes(result.status)) { feedback(result.error || '图片生成失败，请重试。', true); sentJob = null; }
      }
      state = next; images = merge(images, [...next.images,...(next.finalImage ? [next.finalImage] : [])]); messages = merge(messages, next.messages); loaded = true;
      if (kind !== 'messages' && next.images.length) imageCursor = Math.min(imageCursor || Infinity,...next.images.map((i)=>i.version));
      if (kind !== 'images' && next.messages.length) messageCursor = Math.min(messageCursor || Infinity,...next.messages.map((m)=>m.id));
      if (kind !== 'messages' && !next.imagesHasMore) imagesComplete = true;
      if (kind !== 'images' && !next.messagesHasMore) messagesComplete = true;
      $('imageMore').classList.toggle('hidden', imagesComplete);
      $('imageReload').classList.add('hidden'); draw();
    } catch (error) { feedback(`读取失败：${error.message}`, true); $('imageReload').classList.remove('hidden'); }
    finally { fetching = false; }
  }
  $('imageReload').onclick = () => refresh(); $('imageMore').onclick = () => refresh('images');
  $('imageCompose').onsubmit = async (event) => {
    event.preventDefault(); if (!loaded || posting || state.activeJob || state.finalImageId) return;
    const text = $('imageInstruction').value.trim(); if (state.latestImage && !text) { feedback('请填写图片修改要求。', true); return; }
    const answers = state.latestImage ? undefined : [...$('imagePlanFields').querySelectorAll('textarea')].map((input) => input.value.trim());
    if (answers) {
      const empty = answers.findIndex((answer) => !answer);
      if (empty >= 0) { feedback(`第${empty + 1}题的生成内容不能为空。`, true); $('imagePlanFields').querySelectorAll('textarea')[empty].focus(); return; }
      if (!$('imagePlanConfirmed').checked) { feedback('请先确认用于生成图片的内容。', true); $('imagePlanConfirmed').focus(); return; }
    }
    const base = state.latestImage?.id || '';
    const source = JSON.stringify(answers);
    if (!pending || pending.message !== text || pending.base !== base || pending.source !== source) pending = { key: requestKey(), message: text, base, source };
    posting = true; generatingRequest=true; draw();
    node.querySelector('.design-source').open=false;
    $('imageGenerating').scrollIntoView({behavior:'smooth',block:'center'});
    try {
      const data = await api(`${prefix}/image-jobs`, { method: 'POST', body: JSON.stringify({ classId, studentId, requestKey: pending.key, message: text, answers }) });
      if (stopped) return; state.activeJob = ['queued','running'].includes(data.job.status) ? data.job : null; sentJob = {id:data.job.id,text}; pending = null;
      $('imageFeedback').textContent='';
      await refresh();
    } catch (error) { feedback(error.message, true); }
    finally { posting = false; generatingRequest=false; draw(); }
  };
  $('imageFinalize').onclick = async () => {
    if (!selected || posting || state.activeJob || state.finalImageId || !confirm('提交这张图片作为最终作品？提交后不可继续生成或更换。')) return;
    posting = true; draw();
    try { await api(`${prefix}/final-image`, { method: 'POST', body: JSON.stringify({ classId, studentId, imageId: selected }) }); if (!stopped) { feedback('最终作品提交成功。'); await refresh(); } }
    catch (error) { feedback(error.message, true); }
    finally { posting = false; draw(); }
  };
  void refresh(); const timer = setInterval(() => { if (!posting) void refresh(); }, 3000);
  return () => { stopped = true; clearInterval(timer); };
}

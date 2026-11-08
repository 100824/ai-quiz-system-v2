import { api, apiBase } from '../core/api.js';
import { htmlPreviewURL, htmlDownloadURL, mountHTMLPreviews } from './html-preview.js?v=2026110805';
import { escapeHtml as esc, localTime, planText, copyText } from './shared.js?v=2026110811';
import { designTextError, designAnswersError, attachDesignCounters, designLimits } from './limits.js?v=2026110811';

export function imageURL(planId, studentId, classId, imageId, teacher = false, download = false) {
  return `${apiBase()}/design-plans/${planId}/${teacher ? `records/${studentId}/` : ''}images/${encodeURIComponent(imageId)}?${teacher ? '' : `classId=${classId}&studentId=${studentId}&`}${download ? 'download=1' : ''}`;
}
export const jobLabel = (status) => ({ queued: '排队中', running: '生成中', succeeded: '完成', failed: '失败', interrupted: '中断' })[status] || status;
export function requestKey() { return Array.from(crypto.getRandomValues(new Uint8Array(16)), (n) => n.toString(16).padStart(2, '0')).join(''); }
const merge = (old, added) => [...new Map([...old, ...added].map((item) => [item.id, item])).values()];

export function mountDesignImages({ node, planId, classId, studentId, versions, title, outputType = 'image', draft = null }) {
  const htmlMode = outputType === 'html', word = htmlMode ? '网页' : '图片';
  let stopLatest = null, stopGallery = null;
  const previewContext = {planId, classId, studentId};
  let generatingRequest=false;
  let reasoningJobId = '', reasoningRunning = false;
  let stopped = false, fetching = false, posting = false, loaded = false, selected = '', pending = null, sentJob = null, state = null, images = [], messages = [], signature = '', sourceVersion = 2, imagesComplete = false, messagesComplete = false, imageCursor = 0, messageCursor = 0;
  const query = `classId=${classId}&studentId=${studentId}`;
  const prefix = `/design-plans/${planId}`;
  const $ = (id) => node.querySelector(`#${id}`);
  const url = (id, download = false) => htmlMode ? (download ? htmlDownloadURL(planId,studentId,classId,id) : htmlPreviewURL(planId,studentId,classId,id)) : imageURL(planId, studentId, classId, id, false, download);
  node.innerHTML = `<div class="panel design-image-panel">
    <div class="design-document-heading"><h2 id="creationTitle"></h2><span class="design-state-badge">图片创作</span></div>
    <details class="design-source"><summary>查看两版方案</summary><div class="design-version-picker"><label for="imageSourceVersion">查看版本</label><select id="imageSourceVersion"><option value="2">第二版方案</option><option value="1">第一版方案</option></select><button type="button" id="imageCopyPlan" class="secondary">复制方案</button></div><div id="imageSourceText"></div></details>
    <p id="imageFeedback" role="status"></p><p id="imageTaskStatus" role="status">正在读取图片记录…</p><button id="imageReload" type="button" class="secondary hidden">重新读取</button>
    <div id="imageGenerating" class="design-generation-status hidden" role="status" aria-live="polite"><span class="design-generation-spinner" aria-hidden="true"></span><div><h3 id="imageGeneratingTitle">正在生成图片中…</h3><p>通常需要几十秒，请耐心等待。</p></div></div>
    <details id="htmlThinking" class="design-thinking hidden"><summary><span>生成思路</span><span id="htmlThinkingStatus" class="design-thinking-status"></span></summary><p id="htmlThinkingHint" class="design-thinking-hint"></p><pre id="htmlThinkingText" aria-live="off"></pre></details>
    <div id="imageLatestArea" class="design-latest hidden"><h3><span id="imagePreviewLabel">最新图片</span> <span id="imageLatestVersion"></span></h3><a id="imageLatestLink" target="_blank" rel="noopener"><img id="imageLatest" alt="方案图片"></a><a id="imageLatestDownload" class="design-file-button">下载原图</a></div>
    <form id="imageCompose" class="design-image-compose"><section id="imagePlanEditor"><h3>确认生图内容</h3><p class="meta">已填入第二版方案，可以在生成前修改。两版已提交方案会保留。</p><div id="imagePlanFields"></div><label class="design-image-confirm"><input id="imagePlanConfirmed" type="checkbox">确认使用以上内容生成图片</label></section><label for="imageInstruction" id="imageInstructionLabel">补充要求（选填）</label><textarea id="imageInstruction" maxlength="4000" rows="3" placeholder="例如：科技风，标题突出，文字清晰"></textarea><button id="imageGenerate" type="submit" disabled>生成图片</button></form>
    <section class="design-image-history"><h3>图片版本</h3><div id="imageGallery" class="design-image-gallery"></div><button id="imageMore" type="button" class="secondary hidden">加载更多图片</button><div class="design-actions"><button id="imageFinalize" type="button" disabled>提交最终作品</button><span id="imageFinalStatus"></span></div></section>
  </div>`.replaceAll('图片', word).replaceAll('生图', htmlMode ? '生成' : '生图');
  $('creationTitle').textContent = title;
  if (htmlMode) {
    $('imageLatestLink').innerHTML = '<div id="imageLatest" class="design-html-thumb" data-html-version="">网页预览</div>';
    $('imageLatestDownload').textContent = '下载 HTML';
    const open = document.createElement('a'); open.id = 'imageLatestOpen'; open.className = 'design-file-button'; open.target = '_blank'; open.rel = 'noopener'; open.textContent = '打开网页';
    $('imageLatestArea').append(open);
  }
  const feedback = (text, error = false) => { if (stopped) return; $('imageFeedback').textContent = text; $('imageFeedback').className = error ? 'error-message' : 'success-message'; };
  const renderSource = () => { const v = versions.find((item) => item.version === sourceVersion); $('imageSourceText').innerHTML = v.questions.map((q, i) => `<h4>${i + 1}. ${esc(q)}</h4><p class="design-readonly">${esc(v.answers[i])}</p>`).join(''); };
  const renderPlanEditor = (v) => {
    $('imagePlanFields').innerHTML = v.questions.map((q, i) => `<div class="design-question"><label for="imagePlanAnswer${i}">${i + 1}. ${esc(q)}</label><textarea id="imagePlanAnswer${i}" class="design-answer" data-image-answer rows="4" disabled>${esc(v.answers[i])}</textarea></div>`).join('');
    $('imagePlanFields').querySelectorAll('textarea').forEach((input) => { input.oninput = () => { $('imagePlanConfirmed').checked = false; }; });
    attachDesignCounters($('imagePlanFields'),'textarea',designLimits.answer);
  };
  renderPlanEditor(versions.find((v) => v.version === 2));
  if (draft) { $('imagePlanFields').querySelectorAll('textarea').forEach((input,i)=>{ input.value=draft.answers?.[i] ?? input.value; input.dispatchEvent(new Event('input')); }); $('imageInstruction').value=draft.instruction || ''; }
  renderSource();
  attachDesignCounters(node,'#imageInstruction',designLimits.message);
  $('imageSourceVersion').onchange = (event) => { sourceVersion = Number(event.target.value); renderSource(); };
  $('imageCopyPlan').onclick = async () => { try { await copyText(planText(versions.find((v) => v.version === sourceVersion))); feedback('复制成功。'); } catch (e) { feedback(e.message, true); } };
  function draw() {
    if (stopped || !state) return;
    const busy = posting || !!state.activeJob;
    const generating=!!state.activeJob || generatingRequest;
    $('imageGenerating').classList.toggle('hidden',!generating);
    $('imageGeneratingTitle').textContent=state.activeJob?.status==='queued'?`${word}生成已排队…`:`正在生成${word}中…`;
    if (htmlMode) {
      const phases = {requesting:'正在连接网页生成模型…', thinking:'正在思考网页设计…', writing:'正在编写网页…', validating:'正在检查网页…', saving:'正在保存网页…'};
      const active = state.activeJob;
      if (active?.status === 'running' && phases[active.generationPhase]) $('imageGeneratingTitle').textContent = phases[active.generationPhase];
      const job = active || (generatingRequest ? null : state.lastJob);
      const content = job?.reasoning || '';
      const panel = $('htmlThinking'), text = $('htmlThinkingText');
      panel.classList.toggle('hidden', !generating && !job);
      const jobKey = job?.id || (generating ? 'pending' : '');
      if (jobKey !== reasoningJobId) { if (reasoningJobId !== 'pending' || !generating) panel.open = generating; reasoningJobId = jobKey; }
      else if (reasoningRunning && !active) panel.open = false;
      reasoningRunning = !!active;
      const phase = job?.generationPhase;
      const labels = {requesting:'等待模型响应',thinking:'思考中',writing:'正在编写网页',validating:'正在检查网页',saving:'正在保存网页'};
      $('htmlThinkingStatus').textContent = generating ? (job?.status === 'queued' ? '排队中' : labels[phase] || '正在提交') : job?.status === 'succeeded' ? '生成完成' : '生成未完成';
      let hint = '';
      if (!content) {
        if (!generating) hint = '本次模型未返回可展示的思路。';
        else if (job?.status === 'queued') hint = '任务排队中，开始生成后将在这里显示思路。';
        else if (['writing','validating','saving'].includes(phase)) hint = '模型未返回可展示的思路，网页生成仍在继续。';
        else hint = '正在等待模型返回思路…';
      } else if (!/[\u3400-\u9fff]/u.test(content) && /[A-Za-z]/.test(content)) hint = '模型返回的是英文原文。';
      $('htmlThinkingHint').textContent = hint;
      $('htmlThinkingHint').classList.toggle('hidden', !hint);
      text.classList.toggle('hidden', !content);
      if (text.textContent !== content) {
        const follow = text.scrollHeight - text.scrollTop - text.clientHeight < 40;
        text.textContent = content;
        if (follow) text.scrollTop = text.scrollHeight;
      }
    }
    $('imageCompose').setAttribute('aria-busy',String(generating));
    node.querySelector('.design-image-history').classList.toggle('hidden',!state.imageCount);
    $('imageGenerate').disabled = !loaded || busy || !!state.finalImageId;
    $('imageInstruction').disabled = busy || !!state.finalImageId;
    $('imagePlanEditor').classList.toggle('hidden', !!state.latestImage);
    $('imagePlanEditor').querySelectorAll('textarea,input').forEach((input) => { input.disabled = busy || !!state.finalImageId; });
    $('imageCompose').classList.toggle('hidden', !!state.finalImageId || generating);
    $('imageGenerate').textContent = busy ? (state.activeJob ? jobLabel(state.activeJob.status) + '…' : '提交中…') : state.latestImage ? '发送并生成' : `生成${word}`;
    $('imageInstructionLabel').textContent = state.latestImage ? `修改最新${word}（第${state.latestImage.version}版）` : '补充要求（选填）';
    $('imageTaskStatus').textContent = generating ? '' : state.finalImageId ? '最终作品已提交。' : '';
    const preview = state.finalImage || state.latestImage;
    if (preview) {
      $('imageLatestArea').classList.remove('hidden');
      if ($('imageLatest').dataset.id !== preview.id) { if(htmlMode) { stopLatest?.(); $('imageLatest').dataset.htmlVersion=preview.id; stopLatest=mountHTMLPreviews($('imageLatestArea'),previewContext); } else $('imageLatest').src = url(preview.id); $('imageLatest').dataset.id = preview.id; }
      $('imagePreviewLabel').textContent = state.finalImageId ? '最终作品' : `最新${word}`;
      if(htmlMode) $('imageLatestOpen').href=url(preview.id); $('imageLatestLink').href = url(preview.id); $('imageLatestDownload').href = url(preview.id, true); $('imageLatestVersion').textContent = `第${preview.version}版`;
    }
    const nextSignature = JSON.stringify([images.map((i) => i.id), messages.map((m) => [m.id, m.status]), state.finalImageId]);
    if (signature !== nextSignature) {
      signature = nextSignature; stopGallery?.();
      $('imageGallery').innerHTML = images.length ? [...images].sort((a,b) => a.version-b.version).map((i) => `<article class="design-image-thumb"><a href="${url(i.id)}" target="_blank" rel="noopener">${htmlMode ? `<div class="design-html-thumb" data-html-version="${esc(i.id)}">网页预览</div>` : `<img loading="lazy" src="${url(i.id)}" alt="第${i.version}版图片">`}</a><label><input type="radio" name="finalImage" value="${i.id}" ${state.finalImageId ? 'disabled' : ''} ${i.id === (state.finalImageId || selected) ? 'checked' : ''}>第${i.version}版${i.id === state.finalImageId ? ' · 最终作品' : ''}</label><a class="design-file-button" href="${url(i.id, true)}">${htmlMode ? '下载 HTML' : '下载原图'}</a>${htmlMode ? `<a class="design-file-button" target="_blank" rel="noopener" href="${url(i.id)}">打开网页</a>` : ''}</article>`).join('') : '<p class="meta">尚未生成图片。</p>';
      if (htmlMode) stopGallery=mountHTMLPreviews($('imageGallery'),previewContext);
      $('imageGallery').querySelectorAll('input').forEach((input) => { input.onchange = () => { selected = input.value; draw(); }; });
    }
    $('imageFinalize').disabled = !selected || busy || !!state.finalImageId;
    $('imageGallery').querySelectorAll('input').forEach((input) => { input.disabled = posting || !!state.finalImageId; });
    $('imageFinalize').classList.toggle('hidden', !!state.finalImageId);
    const finalImage = images.find((i) => i.id === state.finalImageId);
    $('imageFinalStatus').textContent = state.finalImageId ? `已提交最终作品${finalImage ? `：第${finalImage.version}版` : ''}，${localTime(state.finalizedAt)}` : state.imageCount ? `选择一个${word}版本作为最终作品。` : '';
  }
  async function refresh(kind = '') {
    if (stopped || fetching) return; fetching = true;
    try {
      const older = kind === 'images' && imageCursor ? `&beforeVersion=${imageCursor}` : kind === 'messages' && messageCursor ? `&beforeMessageId=${messageCursor}` : '';
      const raw = await api(`${prefix}/${htmlMode?'html':'images'}?${query}${older}`);
      const next = htmlMode ? {...raw, imageCount:raw.versionCount, images:raw.versions, imagesHasMore:raw.versionsHasMore, latestImage:raw.latestVersion, finalImage:raw.finalVersion, finalImageId:raw.finalVersionId} : raw; if (stopped) return;
      if (!loaded) {
        if (next.sourcePlan && !draft) renderPlanEditor(next.sourcePlan);
        const previous = next.activeJob || (['failed','interrupted'].includes(next.lastJob?.status) ? next.lastJob : null);
        if (previous && !draft) $('imageInstruction').value = ['按第二版方案生成图片。','按确认内容生成图片。','按第二版方案生成网页。','按确认内容生成网页。'].includes(previous.message || previous.content) ? '' : previous.message || previous.content;
      }
      if (sentJob) {
        const result = next.lastJob?.id===sentJob.id ? next.lastJob : null;
        if (result && result.status === 'succeeded') { $('imageInstruction').value = ''; sentJob = null; }
        else if (result && ['failed','interrupted'].includes(result.status)) { feedback(result.error || `${word}生成失败，请重试。`, true); sentJob = null; }
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
    const messageError=designTextError($('imageInstruction').value,'补充或修改要求',designLimits.message,!!state.latestImage);
    if(messageError){feedback(messageError,true);$('imageInstruction').focus();$('imageFeedback').scrollIntoView({behavior:'smooth',block:'center'});return;}
    const text = $('imageInstruction').value.trim(); if (state.latestImage && !text) { feedback(`请填写${word}修改要求。`, true); return; }
    const answers = state.latestImage ? undefined : [...$('imagePlanFields').querySelectorAll('textarea')].map((input) => input.value.trim());
    if (answers) {
      const error=designAnswersError([...$('imagePlanFields').querySelectorAll('textarea')].map(input=>input.value));
      if(error){feedback(error,true);$('imageFeedback').scrollIntoView({behavior:'smooth',block:'center'});return;}
      const empty = answers.findIndex((answer) => !answer);
      if (empty >= 0) { feedback(`第${empty + 1}题的生成内容不能为空。`, true); $('imagePlanFields').querySelectorAll('textarea')[empty].focus(); return; }
      if (!$('imagePlanConfirmed').checked) { feedback(`请先确认用于生成${word}的内容。`, true); $('imagePlanConfirmed').focus(); return; }
    }
    const base = state.latestImage?.id || '';
    const source = JSON.stringify(answers);
    if (!pending || pending.message !== text || pending.base !== base || pending.source !== source) pending = { key: requestKey(), message: text, base, source };
    posting = true; generatingRequest=true; draw();
    node.querySelector('.design-source').open=false;
    $('imageGenerating').scrollIntoView({behavior:'smooth',block:'center'});
    try {
      const data = await api(`${prefix}/${htmlMode?'html-jobs':'image-jobs'}`, { method: 'POST', body: JSON.stringify({ classId, studentId, requestKey: pending.key, message: text, answers }) });
      if (stopped) return; state.activeJob = ['queued','running'].includes(data.job.status) ? data.job : null; sentJob = {id:data.job.id,text}; pending = null;
      $('imageFeedback').textContent='';
      await refresh();
    } catch (error) { feedback(error.message, true); }
    finally { posting = false; generatingRequest=false; draw(); }
  };
  $('imageFinalize').onclick = async () => {
    if (!selected || posting || state.activeJob || state.finalImageId || !confirm(`提交这个${word}版本作为最终作品？提交后不可继续生成或更换。`)) return;
    posting = true; draw();
    try { await api(`${prefix}/${htmlMode?'final-html':'final-image'}`, { method: 'POST', body: JSON.stringify({ classId, studentId, ...(htmlMode ? {versionId:selected} : {imageId:selected}) }) }); if (!stopped) { feedback('最终作品提交成功。'); await refresh(); } }
    catch (error) { feedback(error.message, true); }
    finally { posting = false; draw(); }
  };
  void refresh(); const timer = setInterval(() => { if (!posting) void refresh(); }, 3000);
  const stop = () => { stopped = true; clearInterval(timer); stopLatest?.(); stopGallery?.(); };
  stop.draft = () => state?.latestImage ? null : {answers:[...$('imagePlanFields').querySelectorAll('textarea')].map((input)=>input.value),instruction:$('imageInstruction').value};
  return stop;
}

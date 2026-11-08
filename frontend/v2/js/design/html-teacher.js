import { api } from '../core/api.js';
import { escapeHtml as esc, localTime } from './shared.js?v=2026110201';
import { htmlPreviewURL, htmlDownloadURL, mountHTMLPreviews } from './html-preview.js?v=2026110805';

export async function showStudentHTML({button, container, planId, studentId}) {
  let stopped=false, dispose=null, versions=[], messages=[], beforeVersion=0, beforeMessage=0;
  const context={planId, studentId, classId:0, teacher:true};
  const load=async()=>{
    button.disabled=true;
    try {
      const query=new URLSearchParams();if(beforeVersion)query.set('beforeVersion',beforeVersion);if(beforeMessage)query.set('beforeMessageId',beforeMessage);
      const data=await api(`/design-plans/${planId}/records/${studentId}/html?${query}`);
      if(stopped || !container.isConnected)return;
      versions=[...new Map([...versions,...data.versions].map(v=>[v.id,v])).values()];
      messages=[...new Map([...messages,...data.messages].map(v=>[v.id,v])).values()];
      dispose?.();
      container.innerHTML=`<p class="meta">${data.finalVersionId?`最终提交：${esc(localTime(data.finalizedAt))}`:'尚未提交最终作品'}</p>
        ${data.sourcePlan?`<details class="design-source"><summary>网页生成使用的内容</summary>${data.sourcePlan.questions.map((q,i)=>`<h4>${i+1}. ${esc(q)}</h4><p class="design-readonly">${esc(data.sourcePlan.answers[i])}</p>`).join('')}</details>`:''}
        <div class="design-image-gallery">${[...versions].sort((a,b)=>a.version-b.version).map(v=>`<article class="design-image-thumb"><a target="_blank" rel="noopener" href="${htmlPreviewURL(planId,studentId,0,v.id,true)}"><div class="design-html-thumb" data-html-version="${esc(v.id)}">网页预览</div></a><strong>第${v.version}版${v.id===data.finalVersionId?' · 最终作品':v.id===data.latestVersion?.id?' · 最新版本':''}</strong><a class="design-file-button" target="_blank" rel="noopener" href="${htmlPreviewURL(planId,studentId,0,v.id,true)}">打开网页</a><a class="design-file-button" href="${htmlDownloadURL(planId,studentId,0,v.id,true)}">下载 HTML</a><small>${esc(localTime(v.createdAt))} · ${esc(v.model)}</small></article>`).join('') || '<p class="meta">尚无网页。</p>'}</div>
        ${[...messages].sort((a,b)=>a.id-b.id).map(m=>`<div class="design-chat-message"><strong>${m.role==='user'?'学生':'网页生成结果'} · ${esc(({queued:'排队中',running:'生成中',succeeded:'完成',failed:'失败',interrupted:'中断'})[m.status]||m.status)}</strong><time>${esc(localTime(m.createdAt))}</time><p>${esc(m.content)}</p></div>`).join('')}
        ${data.versionsHasMore||data.messagesHasMore?'<button type="button" class="secondary" data-html-more>加载更多网页和对话</button>':''}`;
      dispose=mountHTMLPreviews(container,context);
      beforeVersion=versions.length?Math.min(...versions.map(v=>v.version)):1;
      beforeMessage=messages.length?Math.min(...messages.map(v=>v.id)):1;
      container.querySelector('[data-html-more]')?.addEventListener('click',load);
      button.textContent='刷新网页与对话';
    } catch(error) { if(!stopped && container.isConnected) container.textContent=`读取失败：${error.message}`; }
    finally { if(button.isConnected)button.disabled=false; }
  };
  await load();
  return ()=>{stopped=true;dispose?.();};
}

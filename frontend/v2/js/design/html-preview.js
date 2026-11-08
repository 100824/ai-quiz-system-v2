import { api, apiBase } from '../core/api.js';

export function htmlVersionPath(planId, studentId, classId, versionId, teacher = false) {
  return `/design-plans/${planId}/${teacher ? `records/${studentId}/` : ''}html-versions/${encodeURIComponent(versionId)}?${teacher ? '' : `classId=${classId}&studentId=${studentId}`}`;
}
export function htmlDownloadURL(planId, studentId, classId, versionId, teacher = false) {
  return `${apiBase()}${htmlVersionPath(planId, studentId, classId, versionId, teacher)}&download=1`;
}
export function htmlPreviewURL(planId, studentId, classId, versionId, teacher = false) {
  return `./design-preview.html?${new URLSearchParams({planId, studentId, classId, versionId, ...(teacher ? {teacher:'1'} : {})})}`;
}

// Mount only visible thumbnails; each iframe has an opaque origin and no platform access.
export function mountHTMLPreviews(root, {planId, studentId, classId, teacher = false}) {
  let stopped = false;
  const pending = new Map();
  const resize = new ResizeObserver((entries) => {
    for (const {target, contentRect} of entries) {
      const frame = target.querySelector('iframe');
      if (frame) frame.style.transform = `scale(${contentRect.width / 1024})`;
    }
  });
  const visible = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      const slot = entry.target;
      if (!entry.isIntersecting) {
        pending.get(slot)?.abort(); pending.delete(slot); resize.unobserve(slot);
        slot.replaceChildren(); slot.textContent = '网页预览'; continue;
      }
      if (pending.has(slot) || slot.querySelector('iframe')) continue;
      const controller = new AbortController(); pending.set(slot, controller);
      slot.textContent = '正在读取预览…';
      api(htmlVersionPath(planId, studentId, classId, slot.dataset.htmlVersion, teacher), {signal:controller.signal}).then((data) => {
        if (stopped || controller.signal.aborted || !slot.isConnected) return;
        const frame = document.createElement('iframe');
        frame.setAttribute('sandbox', 'allow-scripts'); frame.setAttribute('referrerpolicy', 'no-referrer');
        frame.title = '网页版本缩略预览'; frame.tabIndex = -1;
        frame.srcdoc = data.html;
        frame.style.transform = `scale(${slot.clientWidth / 1024})`;
        slot.replaceChildren(frame); resize.observe(slot);
      }).catch((error) => { if (!stopped && !controller.signal.aborted) slot.textContent = `预览读取失败：${error.message}`; })
        .finally(() => { if (pending.get(slot) === controller) pending.delete(slot); });
    }
  });
  root.querySelectorAll('[data-html-version]').forEach((slot) => visible.observe(slot));
  return () => { stopped = true; visible.disconnect(); resize.disconnect(); pending.forEach((controller) => controller.abort()); root.querySelectorAll('[data-html-version]').forEach((slot) => slot.replaceChildren()); };
}

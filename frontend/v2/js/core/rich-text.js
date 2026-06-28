function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function resolveImageUrl(url) {
  if (!url) return '';
  if (/^https?:\/\//i.test(url)) return url;
  if (url.startsWith('/api/')) {
    return `${window.location.protocol}//${window.location.hostname}:8080${url}`;
  }
  return url;
}

function renderInlineRichText(value) {
  let html = escapeHtml(value);
  html = html.replace(/\{\{red:([^{}\n]+)\}\}/g, '<span class="question-rich-red">$1</span>');
  html = html.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
  return html;
}

export function renderRichText(value) {
  const pattern = /!\[([^\]]*)\]\(([^)\s]+)\)/g;
  let html = '';
  let lastIndex = 0;
  let match;
  const text = String(value ?? '');
  while ((match = pattern.exec(text)) !== null) {
    html += renderInlineRichText(text.slice(lastIndex, match.index)).replace(/\n/g, '<br>');
    html += `<img class="question-inline-image" src="${escapeHtml(resolveImageUrl(match[2]))}" alt="${escapeHtml(match[1] || '图片')}" loading="lazy">`;
    lastIndex = match.index + match[0].length;
  }
  html += renderInlineRichText(text.slice(lastIndex)).replace(/\n/g, '<br>');
  return html;
}

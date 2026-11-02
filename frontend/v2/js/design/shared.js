export const escapeHtml = (value) => String(value ?? '').replace(/[&<>"']/g, (char) => ({'&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'}[char]));
export const versionStatus = (count) => ['未提交', '仅第一版', '两版完成'][count] || '未提交';
export const comparable = (value) => String(value).replace(/\s/gu, '');
export const localTime = (value) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '未提交';

export function planText(version) {
  return `${version.title}\n第${version.version === 1 ? '一' : '二'}版方案\n\n${version.questions.map((question, i) => `${i + 1}. ${question}\n${version.answers[i]}`).join('\n\n')}`;
}

export async function copyText(text) {
  if (navigator.clipboard?.writeText && window.isSecureContext) {
    try { await navigator.clipboard.writeText(text); return; } catch { /* Try the HTTP-compatible fallback. */ }
  }
  const previous = document.activeElement;
  const input = document.createElement('textarea');
  input.value = text;
  input.style.cssText = 'position:fixed;left:-9999px;top:0';
  document.body.append(input);
  try {
    input.select();
    if (!document.execCommand('copy')) throw new Error('无法自动复制，请手动选择方案内容复制');
  } finally { input.remove(); previous?.focus(); }
}

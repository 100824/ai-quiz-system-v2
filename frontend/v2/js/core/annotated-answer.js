const HIGHLIGHT_CLASS_BY_COLOR = {
  green: 'part2-highlight--green',
  yellow: 'part2-highlight--yellow',
  red: 'part2-highlight--red'
};

function appendSanitizedNode(source, target, documentRef) {
  if (source.nodeType === Node.TEXT_NODE) {
    target.appendChild(documentRef.createTextNode(source.textContent || ''));
    return;
  }
  if (source.nodeType !== Node.ELEMENT_NODE) return;

  const tag = source.tagName.toLowerCase();
  if (tag === 'script' || tag === 'style') return;
  if (tag === 'br') {
    target.appendChild(documentRef.createElement('br'));
    return;
  }

  let childTarget = target;
  if (tag === 'span') {
    const color = Object.keys(HIGHLIGHT_CLASS_BY_COLOR).find((item) => (
      source.classList.contains(HIGHLIGHT_CLASS_BY_COLOR[item])
    ));
    if (color) {
      childTarget = documentRef.createElement('span');
      childTarget.className = `part2-highlight ${HIGHLIGHT_CLASS_BY_COLOR[color]}`;
      target.appendChild(childTarget);
    }
  }

  Array.from(source.childNodes).forEach((child) => appendSanitizedNode(child, childTarget, documentRef));
  if ((tag === 'div' || tag === 'p') && target.lastChild?.nodeName !== 'BR') {
    target.appendChild(documentRef.createElement('br'));
  }
}

export function sanitizeAnnotatedAnswer(value) {
  const source = document.createElement('template');
  source.innerHTML = String(value || '');
  const target = document.createElement('div');
  Array.from(source.content.childNodes).forEach((node) => appendSanitizedNode(node, target, document));
  while (target.lastChild?.nodeName === 'BR') target.lastChild.remove();
  return target.innerHTML;
}

export function renderAnnotatedAnswer(value, fallback = '未提交') {
  const html = sanitizeAnnotatedAnswer(value);
  const probe = document.createElement('div');
  probe.innerHTML = html;
  return probe.textContent?.trim() ? html : fallback;
}

export function countAnnotatedHighlights(root) {
  if (!root) return 0;
  return root.querySelectorAll(
    '.part2-highlight--green, .part2-highlight--yellow, .part2-highlight--red'
  ).length;
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderInlineMarkdown(value) {
  let html = escapeHtml(value);
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
  html = html.replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+|\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  html = html.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, '$1<em>$2</em>');
  html = html.replace(/~~([^~\n]+)~~/g, '<del>$1</del>');
  return html;
}

function renderCodeBlock(language, code) {
  const lang = String(language || '').trim();
  const className = lang ? ` class="language-${escapeHtml(lang)}"` : '';
  return `<pre class="markdown-code"><code${className}>${escapeHtml(code)}</code></pre>`;
}

export function renderMarkdown(value) {
  const text = String(value ?? '').replace(/\r\n/g, '\n');
  if (!text.trim()) return '';

  const codeBlocks = [];
  const tokenized = text.replace(/```([\w-]*)\n([\s\S]*?)```/g, (_match, language, code) => {
    const token = `\u0000CODE_BLOCK_${codeBlocks.length}\u0000`;
    codeBlocks.push({
      token,
      html: renderCodeBlock(language, code)
    });
    return token;
  });

  const lines = tokenized.split('\n');
  const output = [];
  let paragraph = [];
  let listType = '';
  let listItems = [];

  const flushParagraph = () => {
    if (!paragraph.length) return;
    output.push(`<p>${paragraph.join('<br>')}</p>`);
    paragraph = [];
  };

  const flushList = () => {
    if (!listType || !listItems.length) return;
    output.push(`<${listType}>${listItems.join('')}</${listType}>`);
    listType = '';
    listItems = [];
  };

  const flushAll = () => {
    flushParagraph();
    flushList();
  };

  for (const line of lines) {
    const trimmed = line.trim();
    const codeBlock = codeBlocks.find((item) => item.token === trimmed);
    if (codeBlock) {
      flushAll();
      output.push(codeBlock.html);
      continue;
    }

    if (!trimmed) {
      flushAll();
      continue;
    }

    const headingMatch = trimmed.match(/^(#{1,6})\s+(.+)$/);
    if (headingMatch) {
      flushAll();
      const level = headingMatch[1].length;
      output.push(`<h${level}>${renderInlineMarkdown(headingMatch[2])}</h${level}>`);
      continue;
    }

    const quoteMatch = trimmed.match(/^>\s+(.+)$/);
    if (quoteMatch) {
      flushAll();
      output.push(`<blockquote>${renderInlineMarkdown(quoteMatch[1])}</blockquote>`);
      continue;
    }

    const unorderedMatch = trimmed.match(/^[-*+]\s+(.+)$/);
    if (unorderedMatch) {
      flushParagraph();
      if (listType && listType !== 'ul') flushList();
      listType = 'ul';
      listItems.push(`<li>${renderInlineMarkdown(unorderedMatch[1])}</li>`);
      continue;
    }

    const orderedMatch = trimmed.match(/^\d+\.\s+(.+)$/);
    if (orderedMatch) {
      flushParagraph();
      if (listType && listType !== 'ol') flushList();
      listType = 'ol';
      listItems.push(`<li>${renderInlineMarkdown(orderedMatch[1])}</li>`);
      continue;
    }

    flushList();
    paragraph.push(renderInlineMarkdown(line));
  }

  flushAll();
  return output.join('');
}


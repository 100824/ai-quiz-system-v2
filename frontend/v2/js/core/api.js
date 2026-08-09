const API_BASE = `${window.location.protocol}//${window.location.hostname}:8080/api/v2`;

export async function api(path, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {})
    }
  });
  const data = await res.json().catch(() => ({ success: false, error: '响应解析失败' }));
  if (!res.ok || !data.success) {
    throw new Error(data.error || `请求失败: ${res.status}`);
  }
  return data.data || {};
}

async function responseError(res) {
  const data = await res.json().catch(() => ({ success: false, error: `请求失败: ${res.status}` }));
  return new Error(data.error || `请求失败: ${res.status}`);
}

export async function streamApi(path, options = {}, handlers = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
      ...(options.headers || {})
    }
  });
  if (!res.ok) throw await responseError(res);
  if (!res.body) throw new Error('浏览器不支持流式响应');

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let eventName = '';
  let dataLines = [];
  let doneData = null;

  const dispatch = async () => {
    if (!dataLines.length) {
      eventName = '';
      return;
    }
    const type = eventName || 'message';
    const raw = dataLines.join('\n');
    let payload;
    try {
      payload = JSON.parse(raw);
    } catch {
      throw new Error('流式响应解析失败');
    }
    if (type === 'start') {
      await handlers.onStart?.(payload);
    } else if (type === 'delta') {
      await handlers.onDelta?.(String(payload?.content || ''), payload);
    } else if (type === 'done') {
      doneData = payload;
      await handlers.onDone?.(payload);
    } else if (type === 'error') {
      throw new Error(payload?.error || 'AI生成失败');
    }
    eventName = '';
    dataLines = [];
  };

  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';
    for (const rawLine of lines) {
      const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine;
      if (!line) {
        await dispatch();
      } else if (line.startsWith('event:')) {
        eventName = line.slice(6).trim();
      } else if (line.startsWith('data:')) {
        dataLines.push(line.slice(5).trimStart());
      }
    }
    if (done) break;
  }
  if (buffer) {
    const line = buffer.endsWith('\r') ? buffer.slice(0, -1) : buffer;
    if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart());
  }
  await dispatch();
  if (!doneData) throw new Error('流式响应未完成');
  return doneData;
}

export function apiBase() {
  return API_BASE;
}

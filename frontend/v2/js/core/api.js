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

export function apiBase() {
  return API_BASE;
}

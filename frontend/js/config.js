(function initAppConfig() {
  const params = new URLSearchParams(window.location.search);
  const queryApiBase = params.get('apiBase');
  const defaultApiBase = `${window.location.protocol}//${window.location.hostname}:8080/api`;
  let storedApiBase = null;

  try {
    storedApiBase = window.localStorage.getItem('quiz-api-base');
  } catch (error) {
    console.warn('读取本地 API 配置失败，将使用默认后端地址。', error);
  }

  const apiBase = (queryApiBase || storedApiBase || defaultApiBase).replace(/\/$/, '');

  if (queryApiBase) {
    try {
      window.localStorage.setItem('quiz-api-base', apiBase);
    } catch (error) {
      console.warn('保存本地 API 配置失败。', error);
    }
  }

  window.APP_CONFIG = {
    apiBase
  };

  document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('[data-api-base]').forEach((node) => {
      node.textContent = apiBase;
      node.title = apiBase;
    });
  });
})();

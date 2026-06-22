const registry = new WeakMap();
const instances = new Set();

function optionText(option) {
  return option?.textContent?.trim() || '请选择';
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function selectedOption(select) {
  return select.options[select.selectedIndex] || select.options[0] || null;
}

function getMenuPosition(button, menu) {
  const rect = button.getBoundingClientRect();
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth || 0;
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight || 0;
  const width = Math.max(rect.width, 240);
  const menuWidth = Math.min(width, viewportWidth - 24);
  const left = Math.max(12, Math.min(rect.left, viewportWidth - menuWidth - 12));
  const spaceBelow = viewportHeight - rect.bottom - 12;
  const maxHeight = Math.max(180, Math.min(320, spaceBelow));
  const top = rect.bottom + 8;
  menu.style.position = 'fixed';
  menu.style.left = `${left}px`;
  menu.style.top = `${top}px`;
  menu.style.width = `${menuWidth}px`;
  menu.style.maxHeight = `${maxHeight}px`;
  menu.style.zIndex = '9999';
}

function closeAll(exceptWrapper = null) {
  instances.forEach(({ wrapper, button, menu, detachMenu }) => {
    if (wrapper === exceptWrapper || !wrapper.classList.contains('is-open')) return;
    wrapper.classList.remove('is-open');
    button.setAttribute('aria-expanded', 'false');
    menu.style.display = 'none';
    detachMenu();
  });
}

function syncCustomSelect(select) {
  const instance = registry.get(select);
  if (!instance) return;
  const { wrapper, button, valueNode, menu } = instance;
  const current = selectedOption(select);
  valueNode.textContent = optionText(current);
  button.disabled = select.disabled;
  wrapper.classList.toggle('is-disabled', select.disabled);
  menu.innerHTML = Array.from(select.options).map((option, index) => {
    const selected = option === current;
    return `
      <button
        type="button"
        class="custom-select__option${selected ? ' is-selected' : ''}"
        data-index="${index}"
        ${option.disabled ? 'disabled' : ''}
      >
        ${escapeHtml(optionText(option))}
      </button>
    `;
  }).join('');
}

function selectValue(select, value) {
  select.value = value;
  select.dispatchEvent(new Event('input', { bubbles: true }));
  select.dispatchEvent(new Event('change', { bubbles: true }));
  select.dispatchEvent(new CustomEvent('custom-select-change', {
    bubbles: true,
    detail: { value }
  }));
}

function enhanceSelect(select) {
  if (registry.has(select) || select.dataset.nativeSelect === 'true') return;

  const wrapper = document.createElement('div');
  wrapper.className = 'custom-select';
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'custom-select__button';
  button.setAttribute('aria-haspopup', 'listbox');
  button.setAttribute('aria-expanded', 'false');
  button.innerHTML = `
    <span class="custom-select__value"></span>
    <span class="custom-select__arrow" aria-hidden="true"></span>
  `;
  const menu = document.createElement('div');
  menu.className = 'custom-select__menu';
  menu.setAttribute('role', 'listbox');
  wrapper.append(button, menu);
  menu.dataset.portal = 'true';

  select.classList.add('custom-select__native');
  select.insertAdjacentElement('afterend', wrapper);

  const instance = {
    wrapper,
    button,
    valueNode: button.querySelector('.custom-select__value'),
    menu,
    observer: null,
    reposition: null,
    detachMenu: null
  };
  registry.set(select, instance);
  instances.add(instance);

  instance.observer = new MutationObserver(() => syncCustomSelect(select));
  instance.observer.observe(select, { childList: true, subtree: true, attributes: true });

  const attachMenu = () => {
    if (menu.parentElement !== document.body) {
      document.body.appendChild(menu);
    }
  };

  const detachMenu = () => {
    if (menu.parentElement === document.body) {
      wrapper.append(menu);
    }
  };
  instance.detachMenu = detachMenu;

  instance.reposition = () => getMenuPosition(button, menu);

  button.addEventListener('click', () => {
    if (select.disabled) return;
    const nextOpen = !wrapper.classList.contains('is-open');
    closeAll(wrapper);
    wrapper.classList.toggle('is-open', nextOpen);
    button.setAttribute('aria-expanded', String(nextOpen));
    if (nextOpen) {
      attachMenu();
      menu.style.display = 'grid';
      instance.reposition();
    } else {
      menu.style.display = 'none';
      detachMenu();
    }
  });

  menu.addEventListener('click', (event) => {
    const optionButton = event.target.closest('.custom-select__option');
    if (!optionButton || optionButton.disabled) return;
    const option = select.options[Number(optionButton.dataset.index)];
    if (option) {
      selectValue(select, option.value);
    }
    wrapper.classList.remove('is-open');
    button.setAttribute('aria-expanded', 'false');
    menu.style.display = 'none';
    detachMenu();
  });

  select.addEventListener('change', () => syncCustomSelect(select));
  window.addEventListener('scroll', () => {
    if (wrapper.classList.contains('is-open')) {
      instance.reposition?.();
    }
  }, true);
  window.addEventListener('resize', () => {
    if (wrapper.classList.contains('is-open')) {
      instance.reposition?.();
    }
  });
  syncCustomSelect(select);
}

export function enhanceCustomSelects(root = document) {
  root.querySelectorAll('select').forEach(enhanceSelect);
}

document.addEventListener('click', (event) => {
  if (!event.target.closest('.custom-select, .custom-select__menu')) {
    closeAll();
  }
});

document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape') {
    closeAll();
  }
});

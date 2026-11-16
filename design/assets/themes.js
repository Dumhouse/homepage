// Theme presets. In the Go app each one is a folder in the repo: themes/<id>/theme.json (+ optional theme.css, preview.png).
// Users can make their own in the editor, export it, and open a PR to add it to themes/.
window.HS_PRESETS = [
  { id: 'default', name: 'Default', author: 'Home Services', source: 'built-in', modes: { dark: {}, light: {} } },
  {
    id: 'nord', name: 'Nord', author: 'community', source: 'themes/nord',
    modes: {
      dark: { '--bg': '#2e3440', '--bg-2': '#2b303b', '--panel': '#3b4252', '--panel-2': '#434c5e', '--hover': '#4c566a', '--border': '#434c5e', '--border-2': '#4c566a', '--text': '#eceff4', '--text-2': '#d8dee9', '--muted': '#9aa5b8', '--accent': '#88c0d0', '--accent-soft': 'rgba(136,192,208,.16)', '--ok': '#a3be8c', '--warn': '#ebcb8b', '--bad': '#bf616a' },
      light: { '--bg': '#eceff4', '--bg-2': '#e5e9f0', '--panel': '#ffffff', '--panel-2': '#f3f5f9', '--hover': '#e5e9f0', '--border': '#d8dee9', '--border-2': '#c9d1de', '--text': '#2e3440', '--text-2': '#3b4252', '--muted': '#6b7489', '--accent': '#5e81ac', '--accent-soft': 'rgba(94,129,172,.14)' },
    },
  },
  {
    id: 'solarized', name: 'Solarized', author: 'community', source: 'themes/solarized',
    modes: {
      dark: { '--bg': '#002b36', '--bg-2': '#00262f', '--panel': '#073642', '--panel-2': '#0a3d4a', '--hover': '#0d4654', '--border': '#0f4553', '--border-2': '#1a5563', '--text': '#eee8d5', '--text-2': '#c5c8bd', '--muted': '#839496', '--accent': '#268bd2', '--accent-soft': 'rgba(38,139,210,.18)', '--ok': '#859900', '--warn': '#b58900', '--bad': '#dc322f' },
      light: { '--bg': '#fdf6e3', '--bg-2': '#f5efdc', '--panel': '#fffbf0', '--panel-2': '#f7f1de', '--hover': '#eee8d5', '--border': '#e6dfc8', '--border-2': '#d9d1b8', '--text': '#073642', '--text-2': '#364e55', '--muted': '#657b83', '--accent': '#268bd2', '--ok': '#859900', '--warn': '#b58900', '--bad': '#dc322f' },
    },
  },
  {
    id: 'terminal', name: 'Terminal', author: 'community', source: 'themes/terminal', font: 'mono', radius: 3, density: 'compact',
    modes: {
      dark: { '--bg': '#000000', '--bg-2': '#040804', '--panel': '#081008', '--panel-2': '#0d180d', '--hover': '#122012', '--border': '#1b2e1b', '--border-2': '#285028', '--text': '#b6f5b6', '--text-2': '#8fd18f', '--muted': '#5f9a5f', '--accent': '#39ff6a', '--accent-soft': 'rgba(57,255,106,.12)', '--ok': '#39ff6a', '--warn': '#ffd23f', '--bad': '#ff4d4d' },
      light: { '--bg': '#f2fff2', '--panel': '#ffffff', '--panel-2': '#f4fbf4', '--border': '#cfe8cf', '--border-2': '#b5d9b5', '--text': '#0b3d0b', '--text-2': '#1f5a1f', '--muted': '#4f7f4f', '--accent': '#118a2e' },
    },
    css: '.card, .tile { box-shadow: 0 0 0 1px rgba(57,255,106,.06), 0 0 18px rgba(57,255,106,.04); }\nh1::before { content: "> "; color: var(--accent); }',
  },
  {
    id: 'contrast', name: 'High contrast', author: 'Home Services', source: 'built-in', radius: 4,
    modes: {
      dark: { '--bg': '#000000', '--bg-2': '#000000', '--panel': '#0a0a0a', '--panel-2': '#141414', '--hover': '#222222', '--border': '#8a8a8a', '--border-2': '#bdbdbd', '--text': '#ffffff', '--text-2': '#ffffff', '--muted': '#d0d0d0', '--accent': '#ffd400', '--accent-soft': 'rgba(255,212,0,.18)', '--ok': '#00ff7f', '--warn': '#ffb000', '--bad': '#ff4040' },
      light: { '--bg': '#ffffff', '--bg-2': '#ffffff', '--panel': '#ffffff', '--panel-2': '#f2f2f2', '--border': '#000000', '--border-2': '#000000', '--text': '#000000', '--text-2': '#000000', '--muted': '#333333', '--accent': '#0033cc', '--ok': '#006b2e', '--warn': '#8a4b00', '--bad': '#b00000' },
    },
  },
  {
    id: 'sunset', name: 'Sunset', author: 'community', source: 'themes/sunset', font: 'rounded', radius: 16,
    modes: {
      dark: { '--bg': '#1a1220', '--bg-2': '#160f1b', '--panel': '#241a2c', '--panel-2': '#2c2036', '--hover': '#33263f', '--border': '#3a2c47', '--border-2': '#4a3a59', '--text': '#f5e9f0', '--text-2': '#d9c4d2', '--muted': '#a38ba0', '--accent': '#ff7a59', '--accent-soft': 'rgba(255,122,89,.16)', '--violet': '#c792ea' },
      light: { '--bg': '#fff6f1', '--bg-2': '#fdeee6', '--panel': '#ffffff', '--panel-2': '#fff4ee', '--hover': '#fde8dd', '--border': '#f3d9cc', '--border-2': '#ebc7b6', '--text': '#2a1a24', '--text-2': '#4d3442', '--muted': '#8a6b7b', '--accent': '#e2553a', '--accent-soft': 'rgba(226,85,58,.12)' },
    },
  },
];

(function () {
  const $ = (s, r = document) => r.querySelector(s);
  const current = () => JSON.parse(localStorage.getItem('hs-theme') || 'null');
  const setTheme = t => {
    if (!t || t.id === 'default') localStorage.removeItem('hs-theme');
    else localStorage.setItem('hs-theme', JSON.stringify(t));
    window.HS_THEME.apply();
  };
  window.HS_SET_THEME = setTheme;

  // preview: a tiny app drawn with the theme's own tokens (scoped to the card)
  const styleOf = t => {
    const mode = document.documentElement.dataset.theme;
    const v = (t.modes || {})[mode] || {};
    return Object.entries(v).map(([k, x]) => `${k}:${x}`).join(';') + (t.radius != null ? `;--radius:${t.radius}px` : '');
  };
  const card = t => {
    const on = (current()?.id || 'default') === t.id;
    return `<div class="th-card ${on ? 'on' : ''}" data-theme-id="${t.id}">
      <div class="th-prev" style="${styleOf(t)}">
        <div class="th-side"><i></i><i></i><i></i><i></i></div>
        <div class="th-main"><div class="th-bar"></div>
          <div class="th-tiles"><span><b></b><em class="ok"></em></span><span><b></b><em class="ok"></em></span><span><b></b><em class="warn"></em></span></div>
          <div class="th-btn"></div></div>
      </div>
      <div class="th-meta"><div><b>${t.name}</b><small>${t.source === 'yours' ? 'made by you' : t.source}</small></div>
        <div class="th-acts">${on ? '<span class="badge ok xs">in use</span>' : `<button class="btn sm" data-apply="${t.id}">Use</button>`}
        <a class="btn sm ghost icon" href="theme-editor.html?from=${t.id}" data-tip="Edit a copy"><i data-i="edit" data-s="14"></i></a></div></div>
    </div>`;
  };
  const render = () => {
    const g = $('#themeGallery');
    if (!g) return;
    const mine = JSON.parse(localStorage.getItem('hs-my-themes') || '[]');
    g.innerHTML = [...mine, ...window.HS_PRESETS].map(card).join('');
    window.HS_HYDRATE?.(g);
  };
  document.addEventListener('click', e => {
    const b = e.target.closest('[data-apply]');
    if (!b) return;
    const mine = JSON.parse(localStorage.getItem('hs-my-themes') || '[]');
    const t = [...mine, ...window.HS_PRESETS].find(x => x.id === b.dataset.apply);
    setTheme(t);
    render();
  });
  document.addEventListener('DOMContentLoaded', render);
  window.HS_RENDER_GALLERY = render;
})();

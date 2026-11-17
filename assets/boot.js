// Runs in <head> before the page paints: light/dark, sidebar state, and the user's theme.
// A theme = { id, name, author, version, modes: { dark: {--var: value}, light: {...} },
//             font, density, radius, css, layout: { home, sidebar, tiles } }
(function () {
  const d = document.documentElement, ls = localStorage;
  d.dataset.theme = ls.getItem('theme') || 'dark';
  d.dataset.sb = ls.getItem('sb') || (innerWidth < 1200 ? 'collapsed' : '');

  // Custom CSS is sandboxed: no @import, no external url() — themes can't load or leak anything.
  const sanitize = css => String(css || '')
    .replace(/@import[^;]*;?/gi, '/* @import removed */')
    .replace(/url\(\s*(['"]?)(?!data:|assets\/|\.\/)[^)]*\)/gi, 'none /* external url removed */')
    .replace(/expression\s*\(/gi, '/* removed */(');

  const FONTS = {
    inter: '"Inter", ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif',
    system: 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
    rounded: 'ui-rounded, "SF Pro Rounded", "Nunito", system-ui, sans-serif',
    mono: '"JetBrains Mono", ui-monospace, "SF Mono", Menlo, monospace',
    serif: 'ui-serif, Georgia, "Iowan Old Style", serif',
  };

  const apply = (t = JSON.parse(ls.getItem('hs-theme') || 'null')) => {
    // clear previous inline tokens
    [...d.style].filter(p => p.startsWith('--')).forEach(p => d.style.removeProperty(p));
    document.getElementById('user-css')?.remove();
    delete d.dataset.density;
    d.classList.remove('no-bars');
    if (!t) return;
    if (t.layout && t.layout.bars === false) d.classList.add('no-bars');
    const vars = (t.modes || {})[d.dataset.theme] || {};
    for (const k in vars) d.style.setProperty(k, vars[k]);
    if (t.font && FONTS[t.font]) d.style.setProperty('--font', FONTS[t.font]);
    if (t.radius != null) { d.style.setProperty('--radius', t.radius + 'px'); d.style.setProperty('--radius-sm', Math.max(0, t.radius - 3) + 'px'); }
    if (t.density) d.dataset.density = t.density;
    if (t.css) {
      const s = document.createElement('style');
      s.id = 'user-css';
      s.textContent = sanitize(t.css);
      document.head.appendChild(s);
    }
  };
  window.HS_THEME = { apply, sanitize, FONTS };
  try { apply(); } catch (e) { console.warn('theme ignored', e); }
})();

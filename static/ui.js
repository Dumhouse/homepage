// Home Services real pages: sidebar/theme/copy/toast. The prototype app.js is
// intentionally not loaded — it renders mock data for the design screens.
(() => {
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const root = document.documentElement;

  const toast = msg => {
    let t = $('#toast');
    if (!t) {
      t = document.createElement('div');
      t.id = 'toast';
      t.className = 'toast';
      document.body.appendChild(t);
    }
    t.textContent = msg;
    t.classList.add('show');
    clearTimeout(t._hide);
    t._hide = setTimeout(() => t.classList.remove('show'), 2200);
  };

  const savePref = (key, value) => {
    const body = new URLSearchParams({ key, value });
    fetch('/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body, keepalive: true,
    }).catch(() => { /* localStorage still remembers */ });
  };

  // ---- sidebar ----
  const syncSidebar = () => {
    const collapsed = root.dataset.sb === 'collapsed';
    $$('#sidebar .nav a').forEach(a => {
      if (collapsed) a.setAttribute('data-tip', a.dataset.label || '');
      else a.removeAttribute('data-tip');
    });
  };
  const toggleSidebar = () => {
    const next = root.dataset.sb === 'collapsed' ? 'open' : 'collapsed';
    root.dataset.sb = next;
    localStorage.setItem('sb', next);
    savePref('sidebar', next);
    syncSidebar();
  };
  $('#sbToggle')?.addEventListener('click', toggleSidebar);
  syncSidebar();

  // ---- theme ----
  const ICONS = {
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
  };
  const svg = name => `<svg class="i" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name]}</svg>`;
  const themeBtn = $('#themeBtn');
  const paintTheme = () => {
    if (themeBtn) themeBtn.innerHTML = svg(root.dataset.theme === 'light' ? 'moon' : 'sun');
  };
  themeBtn?.addEventListener('click', () => {
    const next = root.dataset.theme === 'light' ? 'dark' : 'light';
    root.dataset.theme = next;
    localStorage.setItem('theme', next);
    window.HS_THEME?.apply?.();
    paintTheme();
  });
  paintTheme();

  // ---- copy buttons ----
  document.addEventListener('click', async e => {
    const b = e.target.closest('[data-copy]');
    if (!b) return;
    e.preventDefault();
    try {
      await navigator.clipboard.writeText(b.dataset.copy);
      toast('Copied');
    } catch {
      toast('Could not copy');
    }
  });

  // ---- ⌘K palette ----
  const PATHS = {
    server: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
    folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
    activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
    link: '<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>',
    globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  };
  const KIND_ICON = { host: 'server', service: 'activity', project: 'folder', app_link: 'link', location: 'globe', action: 'plus' };
  const iconSvg = name => `<svg class="i" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${PATHS[name] || PATHS.search}</svg>`;
  const esc = s => String(s == null ? '' : s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

  const ACTIONS = [
    { kind: 'action', title: 'New host', subtitle: 'Create a VM, LXC, bare metal or VPS', url: '/hosts/new' },
    { kind: 'action', title: 'New project', subtitle: 'Create an app', url: '/projects/new' },
    { kind: 'action', title: 'MCP & tokens', subtitle: 'Connect an agent', url: '/settings/mcp' },
  ];

  const backdrop = $('#paletteBackdrop'), input = $('#paletteInput'), list = $('#paletteList');
  let items = [], sel = 0, searchTimer;

  const render = () => {
    if (!list) return;
    list.innerHTML = items.map((it, i) => `
      <a class="p-item ${i === sel ? 'sel' : ''}" href="${esc(it.url)}" data-i="${i}">
        ${iconSvg(KIND_ICON[it.kind] || 'search')}
        <div><b>${esc(it.title)}</b>${it.subtitle ? `<small>${esc(it.subtitle)}</small>` : ''}</div>
        <span class="p-kind">${esc((it.kind || '').replace('_', ' '))}</span>
      </a>`).join('') || '<div class="p-empty">No matches. Try a host, project, service or app link.</div>';
  };

  const search = async q => {
    if (!q) {
      items = ACTIONS.slice();
      sel = 0;
      render();
      return;
    }
    let results = [];
    try {
      const res = await fetch('/api/v1/search?limit=8&q=' + encodeURIComponent(q), { headers: { Accept: 'application/json' } });
      if (res.ok) results = await res.json() || [];
    } catch { /* fall back to actions */ }
    const lower = q.toLowerCase();
    items = results.concat(ACTIONS.filter(a => a.title.toLowerCase().includes(lower)));
    sel = 0;
    render();
  };

  const openPalette = () => {
    if (!backdrop) return;
    backdrop.hidden = false;
    if (input) { input.value = ''; input.focus(); }
    search('');
  };
  const closePalette = () => { if (backdrop) backdrop.hidden = true; };

  $('#paletteBtn')?.addEventListener('click', openPalette);
  backdrop?.addEventListener('click', e => { if (e.target === backdrop) closePalette(); });
  list?.addEventListener('click', () => closePalette());
  input?.addEventListener('input', () => {
    clearTimeout(searchTimer);
    const q = input.value.trim();
    searchTimer = setTimeout(() => search(q), 110);
  });

  document.addEventListener('keydown', e => {
    const typing = e.target.closest?.('input, textarea, select, [contenteditable]');
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      backdrop?.hidden ? openPalette() : closePalette();
      return;
    }
    if (e.key === '[' && !e.metaKey && !e.ctrlKey && !typing) {
      e.preventDefault();
      toggleSidebar();
    }
    if (e.key === '/' && !e.metaKey && !e.ctrlKey && !typing) {
      e.preventDefault();
      openPalette();
    }
    if (!backdrop || backdrop.hidden) return;
    if (e.key === 'Escape') { e.preventDefault(); closePalette(); }
    if (e.key === 'ArrowDown') { e.preventDefault(); sel = Math.min(items.length - 1, sel + 1); render(); }
    if (e.key === 'ArrowUp') { e.preventDefault(); sel = Math.max(0, sel - 1); render(); }
    if (e.key === 'Enter') {
      const pick = items[sel];
      if (pick) { e.preventDefault(); location.href = pick.url; }
    }
  });

  // ---- tiles: whole tile navigates; inner action links win ----
  // Sites (absolute URLs) open in a new tab; pages of this app open here.
  const openTile = tile => {
    const u = tile.dataset.url;
    if (/^https?:\/\//i.test(u)) window.open(u, '_blank', 'noopener');
    else location.href = u;
  };
  document.addEventListener('click', e => {
    const tile = e.target.closest('.tile[data-url], .la[data-url]');
    if (!tile || e.target.closest('a, button')) return;
    openTile(tile);
  });
  document.addEventListener('keydown', e => {
    if (e.key !== 'Enter' || e.target.closest?.('a, button')) return;
    const tile = e.target.closest?.('.tile[data-url], .la[data-url]');
    if (tile) openTile(tile);
  });

  // ---- launcher filter ----
  const launcherSearch = $('#launcherSearch');
  launcherSearch?.addEventListener('input', () => {
    const q = launcherSearch.value.toLowerCase();
    let shown = 0;
    $$('#launcherGrid .la-it').forEach(a => {
      const hit = (a.dataset.name || '').toLowerCase().includes(q);
      a.hidden = !hit;
      if (hit) shown++;
    });
    const empty = $('#launcherEmpty');
    if (empty) empty.hidden = shown > 0;
  });

  // ---- tooltips: any [data-tip] (text only) ----
  const tip = document.createElement('div');
  tip.className = 'tip';
  document.body.appendChild(tip);
  document.addEventListener('mouseover', e => {
    const t = e.target.closest?.('[data-tip]');
    if (!t || !t.dataset.tip) { tip.classList.remove('show'); return; }
    tip.textContent = t.dataset.tip;
    const r = t.getBoundingClientRect();
    tip.classList.add('show');
    const tw = tip.offsetWidth, th = tip.offsetHeight;
    const x = Math.min(Math.max(8, r.left + r.width / 2 - tw / 2), innerWidth - tw - 8);
    let y = r.top - th - 8; if (y < 8) y = r.bottom + 8;
    tip.style.left = x + 'px'; tip.style.top = y + 'px';
  });
  document.addEventListener('scroll', () => tip.classList.remove('show'), true);

  // ---- menus (.menu-wrap > .btn toggles), bell popover, [data-open] ----
  document.addEventListener('click', e => {
    const m = e.target.closest('.menu-wrap > .btn');
    $$('.menu-wrap.open').forEach(w => { if (!m || w !== m.parentElement) w.classList.remove('open'); });
    if (m) { e.preventDefault(); m.parentElement.classList.toggle('open'); }
    const o = e.target.closest('[data-open="palette"]');
    if (o) { e.preventDefault(); openPalette(); }
  });

  // ---- copy chips: <span class="copy" data-copy="…"> ----
  $$('.copy').forEach(el => { if (!el.dataset.copy) el.dataset.copy = el.textContent.trim(); });
  document.addEventListener('click', e => {
    const c = e.target.closest('.copy');
    if (c) setTimeout(() => { c.classList.add('copied'); setTimeout(() => c.classList.remove('copied'), 900); });
  }, true);

  // ---- segmented view switch for lists/tiles (remembered) ----
  $$('.seg[data-target]').forEach(g => {
    const target = $(g.dataset.target);
    const key = 'seg:' + g.dataset.target;
    const apply = v => {
      $$('button', g).forEach(b => b.classList.toggle('on', b.dataset.v === v));
      target?.classList.toggle('compact', v === 'compact');
    };
    apply(localStorage.getItem(key) || 'tiles');
    g.addEventListener('click', e => {
      const b = e.target.closest('button[data-v]'); if (!b) return;
      localStorage.setItem(key, b.dataset.v); apply(b.dataset.v);
    });
  });

  // ---- client-side filters: input[data-filter="#list"] narrows [data-name] rows ----
  $$('input[data-filter]').forEach(inp => inp.addEventListener('input', () => {
    const q = inp.value.toLowerCase();
    $$(inp.dataset.filter + ' [data-name]').forEach(el => { el.hidden = !(el.dataset.name || '').toLowerCase().includes(q); });
  }));

  // N opens the Add menu
  document.addEventListener('keydown', e => {
    if (e.key.toLowerCase() === 'n' && !e.metaKey && !e.ctrlKey && !e.altKey && !e.target.closest?.('input, textarea, select, [contenteditable]')) {
      $('#addBtn')?.click();
    }
  });

  // server-rendered flash toast disappears on its own
  if ($('#toast')) setTimeout(() => $('#toast')?.classList.remove('show'), 4500);
})();

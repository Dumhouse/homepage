// Home Services — mockup behaviour (sidebar, topbar, theme, ⌘K palette, copy, combobox, charts)
(() => {
  const P = {
    grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
    server: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
    network: '<rect x="9" y="2" width="6" height="6" rx="1"/><rect x="2" y="16" width="6" height="6" rx="1"/><rect x="16" y="16" width="6" height="6" rx="1"/><path d="M12 8v4M5 16v-2h14v2"/>',
    radar: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><path d="m12 12 6-6"/>',
    bell: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>',
    sliders: '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
    ext: '<path d="M14 3h7v7M21 3l-9 9M19 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5"/>',
    key: '<circle cx="7.5" cy="15.5" r="4.5"/><path d="m10.7 12.3 9.3-9.3M17 6l3 3M15 8l2 2"/>',
    lock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
    unlock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.5-2"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    terminal: '<path d="m4 17 6-6-6-6M12 19h8"/>',
    refresh: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
    edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
    alert: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    x: '<path d="M18 6 6 18M6 6l12 12"/>',
    globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>',
    git: '<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="8" r="2.5"/><path d="M6 8.5v7M18 10.5c0 4-6 3-11 5.5"/>',
    book: '<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V3H6.5A2.5 2.5 0 0 0 4 5.5z"/><path d="M4 19.5V21h16"/>',
    activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
    shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
    box: '<path d="M21 8 12 3 3 8v8l9 5 9-5z"/><path d="m3 8 9 5 9-5M12 13v8"/>',
    dots: '<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>',
    zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
    upload: '<path d="M12 16V4M7 9l5-5 5 5M4 20h16"/>',
    download: '<path d="M12 4v12M7 11l5 5 5-5M4 20h16"/>',
    link: '<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>',
    bot: '<rect x="4" y="8" width="16" height="12" rx="3"/><path d="M12 4v4M9 13h.01M15 13h.01"/>',
    trash: '<path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14"/>',
    chevron: '<path d="m6 9 6 6 6-6"/>',
    right: '<path d="m9 6 6 6-6 6"/>',
    arrow: '<path d="M5 12h14M13 6l6 6-6 6"/>',
    db: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
    cpu: '<rect x="6" y="6" width="12" height="12" rx="2"/><path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    eye: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
    eyeoff: '<path d="M3 3l18 18M10.6 5.1A10 10 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3 3.9M6.6 6.6C3.9 8.3 2 12 2 12s3.5 7 10 7c1.9 0 3.5-.6 4.9-1.4"/>',
    merge: '<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="18" r="2.5"/><path d="M6 8.5V12a6 6 0 0 0 6 6h3.5"/>',
    star: '<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1 6.2L12 17.3 6.5 20.2l1-6.2L3 9.6l6.2-.9z"/>',
    layout: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/>',
    list: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
    cloud: '<path d="M17.5 19H8a6 6 0 1 1 1.3-11.9A7 7 0 0 1 22 12a4.5 4.5 0 0 1-4.5 7z"/>',
    coin: '<circle cx="12" cy="12" r="9"/><path d="M14.8 9.2A3 3 0 0 0 12.2 8h-.6a2.2 2.2 0 0 0 0 4.4h.8a2.2 2.2 0 0 1 0 4.4h-.7a3 3 0 0 1-2.6-1.2M12 6v2m0 8.8V18"/>',
    sidebar: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>',
    home: '<path d="M3 11 12 4l9 7"/><path d="M5 10v10h5v-6h4v6h5V10"/>',
    power: '<path d="M12 2v10M18.4 6.6a9 9 0 1 1-12.8 0"/>',
  };
  const GH = '<path fill="currentColor" stroke="none" d="M12 .5C5.7.5.5 5.7.5 12c0 5.1 3.3 9.4 7.9 10.9.6.1.8-.3.8-.6v-2c-3.2.7-3.9-1.5-3.9-1.5-.5-1.3-1.3-1.7-1.3-1.7-1-.7.1-.7.1-.7 1.2.1 1.8 1.2 1.8 1.2 1 1.8 2.8 1.3 3.5 1 .1-.8.4-1.3.7-1.6-2.6-.3-5.3-1.3-5.3-5.7 0-1.3.5-2.3 1.2-3.1-.1-.3-.5-1.5.1-3.1 0 0 1-.3 3.3 1.2a11.5 11.5 0 0 1 6 0C17.3 4.7 18.3 5 18.3 5c.7 1.6.2 2.8.1 3.1.8.8 1.2 1.8 1.2 3.1 0 4.4-2.7 5.4-5.3 5.7.4.4.8 1.1.8 2.2v3.3c0 .3.2.7.8.6A11.5 11.5 0 0 0 23.5 12C23.5 5.7 18.3.5 12 .5z"/>';

  const icon = (name, size = 16) =>
    `<svg class="i" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${name === 'github' ? GH : (P[name] || '')}</svg>`;
  window.icon = icon;

  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const page = document.body.dataset.page;

  // ---------- home view (Tiles / Launcher), remembered ----------
  const HOME = { tiles: 'dashboard.html', launcher: 'dashboard-launcher.html' };
  const homeHref = HOME[localStorage.getItem('home')] || HOME.tiles;
  document.addEventListener('click', e => {
    const b = e.target.closest('[data-home]');
    if (!b) return;
    localStorage.setItem('home', b.dataset.home);
    location.href = HOME[b.dataset.home];
  });

  // ---------- navigation ----------
  const nav = [
    ['dashboard', 'Dashboard', 'grid', homeHref, '', '', 'd'],
    ['projects', 'Projects', 'folder', 'projects.html', '9', '', 'p'],
    ['hosts', 'Hosts', 'server', 'hosts.html', '17', '', 'h'],
    ['services', 'Services', 'activity', 'services.html', '58', '', 's'],
    ['domains', 'Domains', 'globe', 'domains.html', '1', 'warn', 'o'],
    ['costs', 'Costs', 'coin', 'costs.html', '', '', 'c'],
    ['network', 'Network', 'network', 'network.html', '', '', 'n'],
    ['discovery', 'Discovery', 'radar', 'discovery.html', '4', 'warn', 'i'],
    ['alerts', 'Alerts', 'bell', 'alerts.html', '3', 'bad', 'a'],
  ];
  const navOps = [
    ['ai', 'AI changes', 'bot', 'ai.html', '1', 'warn', 'r'],
    ['backups', 'Backups', 'db', '#'],
    ['audit', 'Audit log', 'clock', '#'],
    ['settings', 'Settings', 'sliders', 'settings.html', '', '', 't'],
  ];
  const link = ([id, label, ic, href, count, cls]) =>
    `<a href="${href}" class="${id === page ? 'active' : ''}" data-label="${label}" ${href === '#' ? 'data-todo' : ''}>${icon(ic)}<span>${label}</span>${count ? `<span class="count ${cls || ''}">${count}</span>` : ''}</a>`;
  const sb = $('#sidebar');
  if (sb) {
    sb.innerHTML = `
      <a class="brand" href="${homeHref}"><span class="logo">${icon('server', 15)}</span><span>Home Services<small>192.168.10.101</small></span></a>
      <nav class="nav">${nav.map(link).join('')}</nav>
      <div class="nav-section">Operations</div>
      <nav class="nav">${navOps.map(link).join('')}</nav>
      <div class="sidebar-foot">
        <a class="mcp-status" href="settings.html#mcp">${icon('bot', 14)} MCP · 2 tokens · <span class="ok-t">online</span></a>
        <div class="user">
          <span class="avatar">D</span><span class="small">alex</span>
          <button class="btn ghost sm icon" id="kbdBtn" data-tip="Keyboard shortcuts (?)" style="margin-left:auto">${icon('grid', 15)}</button>
          <button class="btn ghost sm icon" id="themeBtn" data-tip="Switch light / dark"></button>
        </div>
      </div>`;
  }

  // ---------- collapsible sidebar (remembered in localStorage) ----------
  const root = document.documentElement;
  const syncSb = () => {
    const c = root.dataset.sb === 'collapsed';
    $$('#sidebar .nav a').forEach(a => c ? a.setAttribute('data-tip', a.dataset.label) : a.removeAttribute('data-tip'));
    const t = $('#sbToggle');
    if (t) t.dataset.tip = (c ? 'Show sidebar' : 'Hide sidebar') + ' ( [ )';
  };
  const toggleSb = () => {
    root.dataset.sb = root.dataset.sb === 'collapsed' ? 'open' : 'collapsed';
    localStorage.setItem('sb', root.dataset.sb);
    syncSb();
  };
  $('#sbToggle')?.addEventListener('click', toggleSb);
  document.addEventListener('keydown', e => {
    if (e.key === '[' && !e.metaKey && !e.ctrlKey && !e.target.closest?.('input, textarea, select, [contenteditable]')) toggleSb();
  });
  syncSb();

  // mobile bottom nav: the 4 most used + search
  const mn = document.createElement('nav');
  mn.className = 'mobile-nav';
  const mItems = [nav[0], nav[2], nav[1], nav[7]];
  mn.innerHTML = mItems.map(([id, label, ic, href, count, cls]) =>
    `<a href="${href}" class="${id === page ? 'active' : ''}">${icon(ic, 20)}<span>${label}</span>${cls ? `<span class="count">${count}</span>` : ''}</a>`).join('') +
    `<a href="#" data-open="palette">${icon('search', 20)}<span>Search</span></a>`;
  document.body.appendChild(mn);

  // ---------- topbar ----------
  const tb = $('#topbar');
  if (tb) {
    const crumbs = (document.body.dataset.crumbs || '').split('/').map(s => s.trim()).filter(Boolean);
    const cHtml = crumbs.map((c, i) => {
      const [label, href] = c.split('|');
      return i === crumbs.length - 1 ? `<b>${label}</b>` : `<a href="${href}">${label}</a>${icon('right', 14)}`;
    }).join('');
    tb.innerHTML = `
      <button class="btn ghost icon sb-toggle" id="sbToggle" aria-label="Toggle sidebar">${icon('sidebar', 18)}</button>
      <div class="crumbs">${cHtml}</div>
      <button class="search-trigger" data-open="palette">${icon('search')} <span>Search or jump to…</span><span class="kbd">⌘K</span></button>
      <div class="menu-wrap">
        <button class="btn ghost icon bell" aria-label="Notifications" data-tip="Notifications">${icon('bell', 18)}<span class="bell-n">3</span></button>
        <div class="notif-pop" role="dialog" aria-label="Notifications">
          <div class="notif-h"><b>Needs attention</b><span class="muted small">3 alerts</span><a class="muted small" href="settings.html#notifications" style="margin-left:auto">Settings</a></div>
          <div class="n-item"><span class="n-ic bad">${icon('db', 16)}</span><div class="n-txt"><b>proxthin storage 89% full</b><small>Proxmox · 2 h ago</small></div><a class="btn sm" href="network.html">View</a></div>
          <div class="n-item"><span class="n-ic warn">${icon('unlock', 16)}</span><div class="n-txt"><b>3 hosts accept SSH passwords</b><small>LAN 88 · today 09:12</small></div><button class="btn sm" data-drawer="ssh-fix">Fix</button></div>
          <div class="n-item"><span class="n-ic warn">${icon('activity', 16)}</span><div class="n-txt"><b>mailpit down on acme-staging</b><small>14 min</small></div><a class="btn sm" href="alerts.html">Open</a></div>
          <div class="notif-sec">Suggestions</div>
          <div class="n-item"><span class="n-ic info">${icon('bot', 16)}</span><div class="n-txt"><b>AI proposed 6 changes</b><small>claude-code · acme-staging · 4 min ago</small></div><a class="btn sm" href="ai.html">Review</a></div>
          <div class="n-item"><span class="n-ic info">${icon('zap', 16)}</span><div class="n-txt"><b>4 plugins suggested</b><small>MikroTik, Docker, Home Assistant, Forgejo</small></div><a class="btn sm" href="discovery.html">See</a></div>
          <div class="n-item"><span class="n-ic info">${icon('globe', 16)}</span><div class="n-txt"><b>Stale DNS record bao.home.example</b><small>Cloudflare · cleanup</small></div><a class="btn sm" href="decommission.html">Clean up</a></div>
          <div class="notif-f"><a href="alerts.html">See all alerts →</a><button class="linklike" data-say="All marked as read">Mark all read</button></div>
        </div>
      </div>
      <button class="btn primary" data-open="quickadd" data-tip="Quick add — paste an IP, ssh command or URL (N)">${icon('plus')} Add</button>`;
  }

  // ---------- theme ----------
  const themeBtn = $('#themeBtn');
  const paintTheme = () => { if (themeBtn) themeBtn.innerHTML = icon(document.documentElement.dataset.theme === 'light' ? 'moon' : 'sun', 15); };
  paintTheme();
  themeBtn?.addEventListener('click', () => {
    const t = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
    document.documentElement.dataset.theme = t;
    localStorage.setItem('theme', t);
    window.HS_THEME?.apply();   // re-apply the custom theme's tokens for this mode
    window.HS_RENDER_GALLERY?.();
    paintTheme();
  });

  // ---------- toast (with optional Undo) ----------
  const toast = document.createElement('div');
  toast.className = 'toast';
  document.body.appendChild(toast);
  let tt;
  const say = (msg, undo) => {
    toast.innerHTML = `<span>${msg}</span>${undo ? '<button type="button">Undo</button>' : ''}`;
    toast.classList.toggle('actionable', !!undo);
    toast.classList.add('show');
    if (undo) toast.querySelector('button').onclick = () => { toast.classList.remove('show'); setTimeout(() => say('Restored'), 150); };
    clearTimeout(tt); tt = setTimeout(() => toast.classList.remove('show'), undo ? 5000 : 1800);
  };
  window.HS_SAY = say;
  window.say = say;

  // ---------- hydrate: icons, copy chips, charts (runs on page + injected content) ----------
  // Favicons: the real app fetches each site's icon server-side and caches it (works with
  // self-signed certs and logins). The mockup loads them live; if one fails, the letter stays.
  const FAV = {
    '#e57000': 'https://192.168.10.250:8006/pve2/images/logo-128.png',
    '#ff6600': 'http://192.168.10.247:3000/assets/img/favicon.svg',
    '#317f6f': 'https://ntfy.home.example/static/images/favicon.ico',
    '#175ddc': 'https://vault.home.example/favicon.ico',
    '#d97706': 'https://192.168.10.234/favicon.svg',
    '#5b9cff': 'assets/favicon.svg',
    '#18bcf2': 'http://192.168.10.200:8123/static/icons/favicon.ico',
    '#0f766e': 'http://192.168.10.248:3000/homepage.ico',
    '#0288d1': 'http://192.168.10.245/favicon.ico',
    '#16a34a': 'http://192.168.10.246:8080/favicon.ico',
    '#7c3aed': 'https://acme.192.168.10.240.sslip.io/favicon.ico',
  };
  const favs = root => $$('.ico', root).forEach(el => {
    if (el.dataset.fh) return;
    el.dataset.fh = 1;
    if (!el.dataset.fav) {
      const m = (el.getAttribute('style') || '').match(/--c:\s*(#[0-9a-f]{6})/i);
      if (m && FAV[m[1].toLowerCase()]) el.dataset.fav = FAV[m[1].toLowerCase()];
    }
    if (!el.dataset.fav) return;
    const img = new Image();
    img.alt = '';
    img.onload = () => { el.classList.add('has-img'); el.textContent = ''; el.appendChild(img); };
    img.src = el.dataset.fav;
  });

  const hydrate = (root = document) => {
    window.HS_HYDRATE = hydrate;
    favs(root);
    $$('i[data-i]', root).forEach(el => { el.outerHTML = icon(el.dataset.i, +el.dataset.s || 16); });
    $$('.copy:not([data-h])', root).forEach(el => {
      el.dataset.h = 1;
      if (!el.dataset.copy) el.dataset.copy = el.textContent.trim();
      el.insertAdjacentHTML('beforeend', icon('copy', 13));
      el.addEventListener('click', e => {
        e.preventDefault(); e.stopPropagation();
        navigator.clipboard?.writeText(el.dataset.copy);
        el.classList.add('copied');
        say(`Copied <code>${el.dataset.copy}</code>`);
        setTimeout(() => el.classList.remove('copied'), 900);
      });
    });
    $$('.codebox:not([data-h])', root).forEach(el => {
      el.dataset.h = 1;
      const b = document.createElement('button');
      b.type = 'button'; b.className = 'btn sm'; b.innerHTML = icon('copy', 13) + 'Copy';
      b.onclick = () => { navigator.clipboard?.writeText(el.querySelector('code').textContent); say('Command copied'); };
      el.appendChild(b);
    });
    $$('[data-uptime]', root).forEach(el => {
      const s = el.dataset.uptime.length < 10 ? 'o'.repeat(40) : el.dataset.uptime;
      el.innerHTML = [...s].map(c => `<i class="${c === 'o' ? '' : c}"></i>`).join('');
      el.removeAttribute('data-uptime');
    });
    $$('svg[data-spark]', root).forEach(svg => {
      const v = svg.dataset.spark.split(',').map(Number);
      const w = +svg.getAttribute('width'), h = +svg.getAttribute('height');
      const max = Math.max(...v), min = Math.min(...v), rng = max - min || 1;
      const pts = v.map((y, i) => [(i / (v.length - 1)) * w, h - 2 - ((y - min) / rng) * (h - 4)]);
      const line = pts.map(p => p.map(n => n.toFixed(1)).join(',')).join(' ');
      svg.classList.add('spark');
      svg.innerHTML = `<path d="M0,${h} L${line.replace(/ /g, ' L')} L${w},${h} Z"/><polyline points="${line}"/>`;
      svg.removeAttribute('data-spark');
    });
  };
  window.hydrate = hydrate;
  hydrate();

  // ---------- tooltips: any [data-tip] ----------
  const tip = document.createElement('div');
  tip.className = 'tip';
  document.body.appendChild(tip);
  document.addEventListener('mouseover', e => {
    const t = e.target.closest('[data-tip]');
    if (!t) { tip.classList.remove('show'); return; }
    tip.innerHTML = t.dataset.tip;
    const r = t.getBoundingClientRect();
    tip.classList.add('show');
    const tw = tip.offsetWidth, th = tip.offsetHeight;
    let x = Math.min(Math.max(8, r.left + r.width / 2 - tw / 2), innerWidth - tw - 8);
    let y = r.top - th - 8; if (y < 8) y = r.bottom + 8;
    tip.style.left = x + 'px'; tip.style.top = y + 'px';
  });

  // ---------- open sites: [data-url] tiles open in a new tab ----------
  const openUrl = url => window.open(url, '_blank', 'noopener');
  // Home click behaviour (Settings > Opening links): 'site' opens the service, 'details' opens its page here
  const openTile = t => {
    const details = t.dataset.details || t.querySelector('.t-acts a:last-child')?.getAttribute('href');
    if (localStorage.getItem('tileClick') === 'details' && details) location.href = details;
    else openUrl(t.dataset.url);
  };
  document.addEventListener('click', e => {
    const t = e.target.closest('[data-url]');
    if (!t || e.target.closest('a, button')) return;
    openTile(t);
  });
  document.addEventListener('keydown', e => {
    const t = e.target.closest?.('[data-url]');
    if (t && e.key === 'Enter' && !e.target.closest('a, button')) openTile(t);
  });
  const tc = document.getElementById('tileClick');
  if (tc) { tc.value = localStorage.getItem('tileClick') || 'site'; tc.addEventListener('change', () => { localStorage.setItem('tileClick', tc.value); say('Saved'); }); }

  // ---------- simple click handlers ----------
  document.addEventListener('click', e => {
    const todo = e.target.closest('[data-todo]');
    if (todo) { e.preventDefault(); say('Not designed yet'); return; }
    const s = e.target.closest('[data-say]');
    if (s) { e.preventDefault(); say(s.dataset.say, s.hasAttribute('data-undo')); }
    const m = e.target.closest('.menu-wrap > .btn');
    $$('.menu-wrap.open').forEach(w => { if (!m || w !== m.parentElement) w.classList.remove('open'); });
    if (m) { e.preventDefault(); m.parentElement.classList.toggle('open'); }
  });

  // ---------- chips / segmented / icon picker / checklist ----------
  $$('.chips[data-single]').forEach(g => g.addEventListener('click', e => {
    const c = e.target.closest('.chip'); if (!c) return;
    $$('.chip', g).forEach(x => x.classList.toggle('on', x === c));
  }));
  $$('.seg').forEach(g => g.addEventListener('click', e => {
    const b = e.target.closest('button'); if (!b) return;
    $$('button', g).forEach(x => x.classList.toggle('on', x === b));
    if (g.dataset.target) $(g.dataset.target).classList.toggle('compact', b.dataset.v === 'compact');
  }));
  $$('.iconpick').forEach(g => g.addEventListener('click', e => {
    const b = e.target.closest('button'); if (!b) return;
    $$('button', g).forEach(x => x.classList.toggle('on', x === b));
  }));
  $$('.choices').forEach(g => g.addEventListener('click', e => {
    const c = e.target.closest('.choice'); if (!c) return;
    if (g.dataset.multi !== undefined) c.classList.toggle('on');
    else $$('.choice', g).forEach(x => x.classList.toggle('on', x === c));
  }));
  const updateChecklist = () => {
    $$('.checklist').forEach(cl => {
      const boxes = $$('input[type=checkbox]', cl), done = boxes.filter(b => b.checked).length;
      boxes.forEach(b => b.closest('li').classList.toggle('done', b.checked));
      const out = $(cl.dataset.progress || '#none');
      if (out) {
        $('.bar > i', out).style.width = (done / boxes.length * 100) + '%';
        $('.n', out).textContent = `${done} of ${boxes.length} done`;
      }
    });
  };
  document.addEventListener('change', e => { if (e.target.closest('.checklist')) updateChecklist(); });
  updateChecklist();

  // ---------- combobox ----------
  const initCombos = (root = document) => $$('.combo:not([data-h])', root).forEach(cb => {
    cb.dataset.h = 1;
    const input = $('.input', cb), list = $('.combo-list', cb);
    const create = $('.combo-opt.create', cb);
    const filter = () => {
      const q = input.value.trim().toLowerCase();
      let exact = false;
      $$('.combo-opt:not(.create)', cb).forEach(o => {
        const t = o.dataset.v.toLowerCase();
        o.classList.toggle('hidden', q && !t.includes(q) && !o.textContent.toLowerCase().includes(q));
        if (t === q) exact = true;
      });
      if (create) { create.classList.toggle('hidden', !q || exact); create.querySelector('b').textContent = input.value.trim(); }
    };
    input.addEventListener('focus', () => { cb.classList.add('open'); filter(); });
    input.addEventListener('input', () => { cb.classList.add('open'); filter(); });
    input.addEventListener('blur', () => setTimeout(() => cb.classList.remove('open'), 150));
    list.addEventListener('mousedown', e => {
      const o = e.target.closest('.combo-opt'); if (!o) return;
      e.preventDefault();
      if (o.classList.contains('create')) say(`Created “${input.value.trim()}”`);
      else input.value = o.dataset.v;
      cb.classList.remove('open'); input.blur();
    });
  });
  initCombos();

  // ---------- tag input ----------
  $$('.taginput').forEach(ti => {
    const inp = $('input', ti);
    ti.addEventListener('click', e => { if (e.target.matches('.tag button')) e.target.parentElement.remove(); else inp.focus(); });
    inp.addEventListener('keydown', e => {
      if ((e.key === 'Enter' || e.key === ',') && inp.value.trim()) {
        e.preventDefault();
        inp.insertAdjacentHTML('beforebegin', `<span class="tag">${inp.value.trim()}<button type="button">×</button></span>`);
        inp.value = '';
      } else if (e.key === 'Backspace' && !inp.value) inp.previousElementSibling?.remove();
    });
  });

  // ---------- shared drawer templates ----------
  const sharedTemplates = {
    'ssh-fix': `
      <div class="drawer-h">
        <span class="ico" style="--c:#b7791f">${icon('key', 16)}</span>
        <div><h2>Switch to key-only SSH</h2><div class="sub">3 hosts still accept passwords. Passwords can be guessed; keys can’t.</div></div>
        <button class="btn ghost sm icon close" data-close>${icon('x')}</button>
      </div>
      <div class="drawer-b">
        <div>
          <div class="small muted" style="margin-bottom:6px">Host</div>
          <div class="chips" data-single>
            <button class="chip on">192.168.2.251</button><button class="chip">192.168.2.254</button><button class="chip">MikroTik · 88.1</button>
          </div>
        </div>
        <div>
          <div class="small muted" style="margin-bottom:6px">System <span class="help" data-tip="Detected from the SSH banner. Change it if it’s wrong.">?</span></div>
          <div class="chips os-tabs"><button class="chip on" data-os="deb">Debian / Ubuntu</button><button class="chip" data-os="alpine">Alpine</button><button class="chip" data-os="ros">MikroTik RouterOS</button></div>
        </div>
        <div class="alert warn" style="align-items:flex-start">${icon('alert', 18)}<div class="body"><b>Keep your current SSH session open</b><div class="muted">until step 4 passes — then you can’t lock yourself out.</div></div></div>

        <ol class="steps" data-os="deb">
          <li><b>Put your public key on the server</b><p>Run on your Mac. It asks for the password one last time.</p><div class="codebox"><code>ssh-copy-id root@192.168.2.251</code></div></li>
          <li><b>Check the key works</b><p>Should print “key works” without asking for a password.</p><div class="codebox"><code>ssh -o PasswordAuthentication=no root@192.168.2.251 'echo key works'</code></div></li>
          <li><b>Turn off password login</b><p>Run on the server.</p><div class="codebox"><code>printf 'PasswordAuthentication no\nKbdInteractiveAuthentication no\n' | sudo tee /etc/ssh/sshd_config.d/10-keys-only.conf
sudo systemctl reload ssh</code></div></li>
          <li><b>Confirm here</b><p>Home Services asks the server which logins it allows — no password is sent.</p><button class="btn primary" data-say="✓ 192.168.2.251 now allows keys only">${icon('refresh', 15)}Re-check now</button></li>
        </ol>
        <ol class="steps" data-os="alpine" hidden>
          <li><b>Put your public key on the server</b><p>Run on your Mac.</p><div class="codebox"><code>ssh-copy-id root@192.168.2.251</code></div></li>
          <li><b>Check the key works</b><div class="codebox"><code>ssh -o PasswordAuthentication=no root@192.168.2.251 'echo key works'</code></div></li>
          <li><b>Turn off password login</b><p>Run on the server.</p><div class="codebox"><code>sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
rc-service sshd reload</code></div></li>
          <li><b>Confirm here</b><button class="btn primary" data-say="✓ re-checked">${icon('refresh', 15)}Re-check now</button></li>
        </ol>
        <ol class="steps" data-os="ros" hidden>
          <li><b>Upload your public key to the router</b><p>Run on your Mac.</p><div class="codebox"><code>scp ~/.ssh/id_ed25519.pub admin@192.168.2.1:</code></div></li>
          <li><b>Import it for the admin user</b><p>Run in the RouterOS terminal (Winbox or SSH).</p><div class="codebox"><code>/user ssh-keys import public-key-file=id_ed25519.pub user=admin</code></div></li>
          <li><b>Don’t allow passwords for users with keys</b><div class="codebox"><code>/ip ssh set always-allow-password-login=no</code></div></li>
          <li><b>Confirm here</b><button class="btn primary" data-say="✓ re-checked">${icon('refresh', 15)}Re-check now</button></li>
        </ol>
      </div>
      <div class="drawer-f"><span class="muted small" style="margin-right:auto">Home Services never changes your servers.</span><button class="btn ghost" data-close>Close</button><button class="btn" data-say="Snoozed this alert for 7 days" data-close>Remind me later</button></div>`,
    'cf-connect': `
      <div class="drawer-h">
        <span class="ico" style="--c:#f38020">${icon('cloud', 16)}</span>
        <div><h2>Connect Cloudflare</h2><div class="sub">One read-only token imports every domain in the account — DNS records, tunnel routes and renewal dates. Nothing to type by hand.</div></div>
        <button class="btn ghost sm icon close" data-close>${icon('x')}</button>
      </div>
      <div class="drawer-b">
        <ol class="steps">
          <li><b>Create a token in Cloudflare</b><p>My Profile → API Tokens → Create Token → <i>Create Custom Token</i>. Name it <code>home-services</code>.</p>
            <a class="btn sm" href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener">${icon('ext', 14)}Open API Tokens</a></li>
          <li><b>Give it read-only permissions</b><p>Add these rows — all <b>Read</b>, nothing can be changed:</p>
            <ul class="perm-list">
              <li><span class="badge xs">Zone</span>Zone · Read <span class="muted xs">— list your domains</span></li>
              <li><span class="badge xs">Zone</span>DNS · Read <span class="muted xs">— records</span></li>
              <li><span class="badge xs">Account</span>Cloudflare Tunnel · Read <span class="muted xs">— hostname → local service</span></li>
              <li><span class="badge xs">Account</span>Registrar: Domains · Read <span class="muted xs">— expiry, auto-renew, price</span></li>
            </ul>
            <p>Zone resources: <b>All zones from an account</b> — new domains show up on their own.</p></li>
          <li><b>Paste the token</b><p>Home Services saves it in <b>Clavis</b> (project <code>homeservices</code>) — never in its own database — and only uses it to read. Keep your own copy in Vaultwarden if you like.</p>
            <input class="input mono" type="password" placeholder="Paste token…" autocomplete="off"></li>
          <li><b>Check and import</b><p>Shows what was found before anything is saved.</p>
            <button class="btn primary" data-say="Found 1 zone (home.example), 6 records, tunnel “edge” with 5 routes — 4 matched to hosts">${icon('refresh', 15)}Test token</button></li>
        </ol>
        <div class="alert info" style="align-items:flex-start">${icon('info', 18)}<div class="body"><b>Several Cloudflare accounts?</b><div class="muted">Add one token per account. Domains from all of them show in one list, labelled by account.</div></div></div>
      </div>
      <div class="drawer-f"><span class="muted small" style="margin-right:auto">Read-only. Home Services never edits DNS.</span><button class="btn ghost" data-close>Cancel</button></div>`,
    'mcp-connect': `
      <div class="drawer-h">
        <span class="ico" style="--c:#d97706">${icon('bot', 16)}</span>
        <div><h2>Connect an AI agent</h2><div class="sub">Standard MCP over HTTP with a bearer token — works with any MCP-capable agent. Each agent gets its own revocable token.</div></div>
        <button class="btn ghost sm icon close" data-close>${icon('x')}</button>
      </div>
      <div class="drawer-b">
        <div class="chips agent-tabs"><button class="chip on" data-agent="claude">Claude Code</button><button class="chip" data-agent="codex">Codex CLI</button><button class="chip" data-agent="gemini">Gemini CLI</button><button class="chip" data-agent="cursor">Cursor</button><button class="chip" data-agent="vscode">VS Code</button><button class="chip" data-agent="other">Other / stdio-only</button></div>
        <ol class="steps"><li><b>Create a token</b><p>Settings › MCP › New token (read, or read + write). Or copy the one saved in Vaultwarden.</p><a class="vw-link" href="https://vault.home.example/#/vault?itemId=f3c4" target="_blank" rel="noopener">${icon('lock', 13)}Open in Vaultwarden</a></li></ol>
        <div class="ag" data-agent="claude"><b>Claude Code</b><p class="small text-2">Run in a terminal (token hidden while typing):</p><div class="codebox"><code>read -s NEXO_MCP_TOKEN && export NEXO_MCP_TOKEN
claude mcp add --transport http acme-staging <span data-mcp-url>URL</span> --header "Authorization: Bearer $NEXO_MCP_TOKEN"
claude mcp list</code></div></div>
        <div class="ag" data-agent="codex" hidden><b>Codex CLI</b><p class="small text-2">Add to <span class="mono">~/.codex/config.toml</span>, then export the token:</p><div class="codebox"><code>[mcp_servers.homeservices]
url = "<span data-mcp-url>URL</span>"
bearer_token_env_var = "HS_TOKEN"</code></div></div>
        <div class="ag" data-agent="gemini" hidden><b>Gemini CLI</b><p class="small text-2">Add to <span class="mono">~/.gemini/settings.json</span>:</p><div class="codebox"><code>{ "mcpServers": { "homeservices": {
  "httpUrl": "<span data-mcp-url>URL</span>",
  "headers": { "Authorization": "Bearer $HS_TOKEN" } } } }</code></div></div>
        <div class="ag" data-agent="cursor" hidden><b>Cursor</b><p class="small text-2">Add to <span class="mono">~/.cursor/mcp.json</span> (or Settings › MCP):</p><div class="codebox"><code>{ "mcpServers": { "homeservices": {
  "url": "<span data-mcp-url>URL</span>",
  "headers": { "Authorization": "Bearer \${env:HS_TOKEN}" } } } }</code></div></div>
        <div class="ag" data-agent="vscode" hidden><b>VS Code (Copilot agent mode)</b><p class="small text-2">Add to <span class="mono">.vscode/mcp.json</span>:</p><div class="codebox"><code>{ "servers": { "homeservices": {
  "type": "http", "url": "<span data-mcp-url>URL</span>",
  "headers": { "Authorization": "Bearer \${input:hs-token}" } } },
  "inputs": [{ "id": "hs-token", "type": "promptString", "password": true, "description": "Home Services token" }] }</code></div></div>
        <div class="ag" data-agent="other" hidden><b>Any other agent</b><p class="small text-2">Needs: URL <span class="mono" data-mcp-url>URL</span>, transport <i>Streamable HTTP</i>, header <span class="mono">Authorization: Bearer &lt;token&gt;</span>. If your agent only supports local (stdio) servers, bridge it:</p><div class="codebox"><code>npx mcp-remote <span data-mcp-url>URL</span> --header "Authorization: Bearer $HS_TOKEN"</code></div></div>
        <p class="muted xs">Config formats change between versions — check your agent’s MCP docs if a field is rejected. No MCP support? Give it <span class="mono">/inventory.md</span> (read-only Markdown) instead.</p>
      </div>
      <div class="drawer-f"><span class="muted small" style="margin-right:auto">Last used 2 h ago by “claude-code”.</span><button class="btn ghost" data-close>Close</button></div>`,
  };
  Object.entries(sharedTemplates).forEach(([id, html]) => {
    if (document.getElementById('drawer-' + id)) return;
    const t = document.createElement('template'); t.id = 'drawer-' + id; t.innerHTML = html;
    document.body.appendChild(t);
  });

  // ---------- drawer: <button data-drawer="ssh-fix"> + <template id="drawer-ssh-fix"> ----------
  const dbd = document.createElement('div'); dbd.className = 'drawer-backdrop';
  const drawer = document.createElement('aside'); drawer.className = 'drawer'; drawer.setAttribute('role', 'dialog');
  document.body.append(dbd, drawer);
  const openDrawer = id => {
    const tpl = document.getElementById('drawer-' + id);
    if (!tpl) { say('Not designed yet'); return; }
    drawer.innerHTML = tpl.innerHTML;
    hydrate(drawer);
    initCombos(drawer);
    $$('.chips[data-single]', drawer).forEach(g => g.addEventListener('click', e => {
      const c = e.target.closest('.chip'); if (!c) return;
      $$('.chip', g).forEach(x => x.classList.toggle('on', x === c));
    }));
    $$('.os-tabs', drawer).forEach(g => g.addEventListener('click', e => {
      const c = e.target.closest('.chip'); if (!c) return;
      $$('.chip', g).forEach(x => x.classList.toggle('on', x === c));
      $$('[data-os]', drawer).forEach(p => { p.hidden = p.dataset.os !== c.dataset.os; });
    }));
    $$('.agent-tabs', drawer).forEach(g => g.addEventListener('click', e => {
      const c = e.target.closest('.chip'); if (!c) return;
      $$('.chip', g).forEach(x => x.classList.toggle('on', x === c));
      $$('.ag', drawer).forEach(p => { p.hidden = p.dataset.agent !== c.dataset.agent; });
    }));
    dbd.classList.add('open'); drawer.classList.add('open');
  };
  const closeDrawer = () => { dbd.classList.remove('open'); drawer.classList.remove('open'); };
  dbd.addEventListener('click', closeDrawer);
  document.addEventListener('click', e => {
    const d = e.target.closest('[data-drawer]');
    if (d) { e.preventDefault(); openDrawer(d.dataset.drawer); }
    if (e.target.closest('[data-close]')) closeDrawer();
  });

  // ---------- modals ----------
  const modal = (id, html) => {
    const m = document.createElement('div');
    m.className = 'modal-backdrop'; m.id = id; m.innerHTML = html;
    document.body.appendChild(m);
    m.addEventListener('click', e => { if (e.target === m || e.target.closest('[data-close]')) m.classList.remove('open'); });
    hydrate(m);
    return m;
  };

  // Quick add — paste anything
  const known = { '192.168.10.240': 'acme-staging', '192.168.10.250': 'proxmox', '192.168.10.254': 'edge', '192.168.10.247': 'forgejo', '192.168.2.1': 'mikrotik', '192.168.10.101': 'homeservices' };
  const ports = { 22: 'SSH', 80: 'HTTP', 443: 'HTTPS', 2222: 'SSH', 3000: 'Web app (Rails / Node)', 3306: 'MySQL', 5000: 'Docker registry', 5432: 'PostgreSQL', 6379: 'Redis', 8006: 'Proxmox UI', 8080: 'HTTP (alt)', 8123: 'Home Assistant', 8200: 'Vault / OpenBao', 9000: 'S3 / MinIO', 9090: 'Cockpit / Prometheus', 27017: 'MongoDB' };
  // fingerprints shipped with plugins (mockup: by port, hostname, or typed product name)
  const FP = {
    8006: ['Proxmox VE', 'proxmox', 'import all VMs and containers'], 3001: ['Uptime Kuma', 'uptime-kuma', 'import its monitors instead of re-creating them'],
    8123: ['Home Assistant', 'home-assistant', 'version and pending updates'], 8728: ['MikroTik', 'mikrotik', 'DHCP leases and device names'],
    9443: ['Portainer', 'portainer', 'containers on all its environments'], 2019: ['Caddy admin API', 'caddy', 'domain → host:port routes'],
  };
  const FPhost = { 'ntfy.home.example': ['ntfy', 'ntfy', 'send alerts here'], 'vault.home.example': ['Vaultwarden', 'bitwarden', '“Open in Vaultwarden” links'] };
  const FPname = [['uptime kuma', 3001], ['proxmox', 8006], ['home assistant', 8123], ['mikrotik', 8728], ['portainer', 9443]];
  const plug = f => row('Plugin', `Looks like <b>${f[0]}</b> — connect to ${f[2]}?`, `<span class="badge info">${icon('zap', 12)}suggest</span>`);

  const qa = modal('quickadd', `
    <div class="modal">
      <div class="modal-h"><b>Quick add</b><span class="muted small">paste anything — I’ll figure out what it is</span><button class="btn ghost sm icon" data-close style="margin-left:auto">${icon('x')}</button></div>
      <div class="modal-b">
        <input class="paste" placeholder="ssh deploy@192.168.10.123 -p 2222" autocomplete="off" spellcheck="false">
        <div class="examples">Try: <button type="button">ssh root@192.168.2.251</button><button type="button">192.168.10.240:5432</button><button type="button">https://ntfy.home.example</button><button type="button">ct 118</button><button type="button">192.168.2.251:3001</button><button type="button">uptime kuma</button></div>
        <div class="preview"><div class="preview-empty">The preview of what will be created appears here.</div></div>
        <div class="small muted">Or create manually:</div>
        <div class="create-grid">
          <a href="host-edit.html">${icon('server', 20)}Host</a>
          <a href="#" data-todo>${icon('folder', 20)}Project</a>
          <a href="#" data-todo>${icon('activity', 20)}Service</a>
          <a href="#" data-todo>${icon('globe', 20)}Domain</a>
        </div>
      </div>
      <div class="modal-f"><span class="muted small" style="margin-right:auto">Nothing is saved until you confirm.</span><button class="btn ghost" data-close>Cancel</button><a class="btn primary" id="qaGo" href="host-edit.html">${icon('arrow')}Continue</a></div>
    </div>`);
  const qin = $('.paste', qa), prev = $('.preview', qa);
  const row = (k, v, b = '') => `<div class="preview-row"><span class="k">${k}</span><span>${v}</span><span class="spacer"></span>${b}</div>`;
  const parse = () => {
    const v = qin.value.trim();
    let m, out = '';
    if (!v) { prev.innerHTML = '<div class="preview-empty">The preview of what will be created appears here.</div>'; return; }
    if ((m = v.match(/^(?:ssh\s+)?(?:-p\s*(\d+)\s+)?([a-z_][\w.-]*)@([\w.-]+)(?::(\d+))?(?:\s+-p\s*(\d+))?$/i))) {
      const host = m[3], port = m[1] || m[4] || m[5] || '22';
      out += known[host] ? row('Host', `<b>${known[host]}</b> <span class="muted mono small">${host}</span>`, '<span class="badge info">exists</span>')
                         : row('Host', `<span class="mono">${host}</span>`, '<span class="badge ok">new</span>');
      out += row('SSH login', `<span class="mono">${m[2]}</span> on port <span class="mono">${port}</span>`, '<span class="badge ok">add</span>');
      out += row('Then', 'Check that this login works with keys only', `<span class="badge">${icon('key', 13)}auto</span>`);
    } else if ((m = v.match(/^(https?):\/\/([^/:\s]+)(?::(\d+))?/i))) {
      const port = m[3] || (m[1] === 'https' ? '443' : '80');
      out += row('Domain', `<span class="mono">${m[2]}</span>`, '<span class="badge ok">new</span>');
      out += row('Service', `${m[1].toUpperCase()} on :${port}`, '<span class="badge ok">add</span>');
      out += row('Then', 'Resolve DNS, find the host, check TLS expiry', `<span class="badge">${icon('lock', 13)}auto</span>`);
      if (FPhost[m[2]]) out += plug(FPhost[m[2]]);
    } else if ((m = v.match(/^(\d{1,3}(?:\.\d{1,3}){3})(?::(\d+))?$/))) {
      const host = m[1];
      out += known[host] ? row('Host', `<b>${known[host]}</b> <span class="muted mono small">${host}</span>`, '<span class="badge info">exists</span>')
                         : row('Host', `<span class="mono">${host}</span>`, '<span class="badge ok">new</span>');
      if (m[2]) out += row('Service', `:${m[2]} · looks like <b>${ports[m[2]] || (FP[m[2]] || [])[0] || 'unknown'}</b>`, '<span class="badge ok">add</span>');
      if (m[2] && FP[m[2]]) out += plug(FP[m[2]]);
      else out += row('Then', 'Scan common ports and suggest services', `<span class="badge">${icon('radar', 13)}auto</span>`);
    } else if ((m = v.match(/^(ct|vm|lxc)?\s*(\d{3,4})$/i))) {
      out += row('Import', `Proxmox guest <b>${m[2]}</b> from node proxmox`, '<span class="badge info">import</span>');
      out += row('Gets', 'Name, IP, CPU/RAM/disk, status — you confirm before saving');
    } else if ((m = FPname.find(([n]) => v.toLowerCase().includes(n)))) {
      const f = FP[m[1]];
      out += row('Product', `<b>${f[0]}</b> — where does it run?`, '<span class="badge info">known</span>');
      out += row('Next', `Paste its address, or <b>scan for it</b>: we look for :${m[1]} on your subnets`, `<span class="badge">${icon('radar', 13)}find</span>`);
      out += plug(f);
    } else {
      out = '<div class="preview-empty">Not sure what that is. Try an IP, <code>user@host</code>, a URL or a Proxmox ID.</div>';
    }
    prev.innerHTML = out;
  };
  qin.addEventListener('input', parse);
  qin.addEventListener('keydown', e => { if (e.key === 'Enter') location.href = 'host-edit.html'; });
  $('.examples', qa).addEventListener('click', e => { const b = e.target.closest('button'); if (b) { qin.value = b.textContent; parse(); qin.focus(); } });
  const openQA = () => { qa.classList.add('open'); qin.value = ''; parse(); setTimeout(() => qin.focus(), 0); };

  // Keyboard shortcuts help
  const sc = (label, ...keys) => `<div class="sc"><span>${label}</span><span>${keys.map(k => `<span class="kbd">${k}</span>`).join('')}</span></div>`;
  const kb = modal('shortcuts', `
    <div class="modal">
      <div class="modal-h"><b>Keyboard shortcuts</b><button class="btn ghost sm icon" data-close style="margin-left:auto">${icon('x')}</button></div>
      <div class="modal-b"><div class="shortcuts">
        <h4>Anywhere</h4>
        ${sc('Search / jump', '⌘', 'K')}${sc('Search', '/')}${sc('Quick add', 'N')}${sc('This help', '?')}
        <h4>Go to</h4>
        ${nav.concat(navOps.filter(n => n[6])).map(n => sc(n[1], 'G', n[6].toUpperCase())).join('')}${sc('Hide / show sidebar', '[')}
        <h4>In lists</h4>
        ${sc('Move', '↑', '↓')}${sc('Open', '↵')}${sc('Copy IP / command', 'C')}${sc('Close', 'Esc')}
      </div></div>
    </div>`);
  $('#kbdBtn')?.addEventListener('click', () => kb.classList.add('open'));

  // ---------- ⌘K palette ----------
  const items = [
    ['Recent', 'acme-staging', 'CT · 192.168.10.240', 'host.html', 'N', '#7c3aed'],
    ['Recent', 'Acme', 'staging site', 'project.html', 'N', '#7c3aed', 'https://acme.192.168.10.240.sslip.io'],
    ['Recent', 'Discovery inbox', '4 new', 'discovery.html', '◎', '#334155'],
    ['Projects', 'Telegram collector', '192.168.10.162', 'project.html', 'T', '#229ed9', 'http://192.168.10.162'],
    ['Projects', 'Home Services', 'open source', 'project.html', 'H', '#5b9cff'],
    ['Projects', 'Homeboard', '192.168.2.250', 'project.html', 'H', '#0ea5a4', 'http://192.168.2.250'],
    ['Hosts', 'Proxmox', '192.168.10.250:8006', 'host.html', 'P', '#e57000', 'https://192.168.10.250:8006'],
    ['Hosts', 'edge', 'CT · 192.168.10.254', 'host.html', 'E', '#22b8cf'],
    ['Hosts', 'Vaultwarden', 'vault.home.example', 'host.html', 'V', '#175ddc', 'https://vault.home.example'],
    ['Hosts', 'Home Assistant', '192.168.10.200:8123', 'host.html', 'H', '#18bcf2', 'http://192.168.10.200:8123'],
    ['Hosts', 'Gatus status', 'home.home.example', 'host.html', 'G', '#16a34a', 'https://home.home.example'],
    ['Hosts', 'Forgejo', '192.168.10.247:3000', 'host.html', 'F', '#ff6600', 'http://192.168.10.247:3000'],
    ['Hosts', 'ntfy', 'ntfy.home.example', 'host.html', 'N', '#317f6f', 'https://ntfy.home.example'],
    ['Hosts', 'Clavis', '192.168.10.234', 'host.html', 'C', '#d97706', 'https://192.168.10.234'],
    ['Hosts', 'MikroTik', '192.168.2.1', 'host.html', 'M', '#b91c1c', 'http://192.168.2.1'],
    ['Services', 'postgres · acme-staging', ':5432', 'services.html', 'P', '#336791'],
    ['Services', 'redis · acme-staging', ':6379', 'services.html', 'R', '#dc382d'],
    ['Services', 'caddy · edge', ':80', 'services.html', 'C', '#22b8cf'],
    ['Domains', 'ntfy.home.example', '→ 192.168.10.252:80', 'domains.html', 'D', '#317f6f', 'https://ntfy.home.example'],
    ['Domains', 'vault.home.example', '→ 192.168.10.253:80', 'domains.html', 'D', '#175ddc', 'https://vault.home.example'],
    ['Domains', 'home.example', 'Cloudflare · renews Apr 2, 2027', 'domain.html', 'S', '#f38020'],
    ['Projects', 'Acme · Sidekiq', '/sidekiq', 'project.html#app-links', 'N', '#b1003e', 'https://acme.192.168.10.240.sslip.io/sidekiq'],
    ['Projects', 'Acme · MCP server', '/mcp', 'project.html#app-links', 'N', '#d97706'],
    ['Actions', 'Review AI changes', '6 pending', 'ai.html', '🤖', '#334155'],
    ['Actions', 'Costs', '€17.10 / month', 'costs.html', '€', '#334155'],
    ['Actions', 'Connect Cloudflare', 'import domains & tunnel routes', '#cf-connect', '☁', '#334155'],
    ['Actions', 'Quick add…', 'N', '#quickadd', '+', '#334155'],
    ['Actions', 'Run discovery scan', '2 subnets', 'discovery.html', '⟳', '#334155'],
    ['Actions', 'Fix SSH password logins', '3 hosts', '#ssh-fix', '🔑', '#334155'],
    ['Actions', 'Export ~/.ssh/config', 'download', '#', '↓', '#334155'],
    ['Actions', 'Network topology', 'map', 'topology.html', '⌘', '#334155'],
    ['Actions', 'Settings · MCP tokens', '', 'settings.html#mcp', '⚙', '#334155'],
  ];
  const pal = modal('palette', `
    <div class="palette" role="dialog" aria-label="Search">
      <div class="p-input">${icon('search', 18)}<input placeholder="Search hosts, IPs, ports, projects, actions…" /><span class="kbd">esc</span></div>
      <div class="results"></div>
      <div class="foot"><span><span class="kbd">↑</span><span class="kbd">↓</span> move</span><span><span class="kbd">↵</span> open site</span><span><span class="kbd">⇧</span><span class="kbd">↵</span> details</span><span style="margin-left:auto">Tip: type <code>:5432</code>, <code>88.</code> or <code>ssh</code></span></div>
    </div>`);
  pal.className = 'palette-backdrop';
  const pin = $('input', pal), res = $('.results', pal);
  let hl = 0, shown = [];
  const go = (it, details) => {
    if (it[6] && !details) { pal.classList.remove('open'); openUrl(it[6]); return; }
    if (it[3] === '#quickadd') { pal.classList.remove('open'); openQA(); }
    else if (it[3] === '#ssh-fix' || it[3] === '#cf-connect') { pal.classList.remove('open'); openDrawer(it[3].slice(1)); }
    else if (it[3] === '#') say('Downloaded ssh_config');
    else location.href = it[3];
  };
  const render = () => {
    const q = pin.value.trim().toLowerCase().replace(/^:/, ':');
    shown = q ? items.filter(it => (it[1] + ' ' + it[2]).toLowerCase().includes(q))
              : items.filter(it => it[0] === 'Recent' || it[0] === 'Actions');
    hl = Math.min(hl, Math.max(shown.length - 1, 0));
    let g = '';
    res.innerHTML = shown.length ? shown.map((it, i) => {
      const head = it[0] !== g ? `<div class="p-group">${(g = it[0])}</div>` : '';
      const open = it[6] ? `<span class="p-open">${icon('ext', 13)} open</span>` : '';
      return `${head}<a class="p-item ${i === hl ? 'hl' : ''}" href="${it[3]}" data-i="${i}"><span class="ico sm" style="--c:${it[5]}">${it[4]}</span>${it[1]}<span class="r">${it[2]}</span>${open}</a>`;
    }).join('') : `<div class="p-empty">Nothing found for “${pin.value}”.<br><a class="btn sm mt-8" href="#" id="pAdd">${icon('plus', 14)}Quick add “${pin.value}”</a></div>`;
    favs(res);
    $('#pAdd', res)?.addEventListener('click', e => { e.preventDefault(); const v = pin.value; pal.classList.remove('open'); openQA(); qin.value = v; parse(); });
  };
  const openPal = () => { pal.classList.add('open'); pin.value = ''; hl = 0; render(); setTimeout(() => pin.focus(), 0); };
  res.addEventListener('click', e => { const it = e.target.closest('.p-item'); if (it) { e.preventDefault(); go(shown[+it.dataset.i], e.shiftKey); } });
  res.addEventListener('mousemove', e => { const it = e.target.closest('.p-item'); if (it && +it.dataset.i !== hl) { hl = +it.dataset.i; render(); } });
  pin.addEventListener('input', () => { hl = 0; render(); });
  pin.addEventListener('keydown', e => {
    if (e.key === 'ArrowDown') { hl = Math.min(hl + 1, shown.length - 1); render(); e.preventDefault(); }
    if (e.key === 'ArrowUp') { hl = Math.max(hl - 1, 0); render(); e.preventDefault(); }
    if (e.key === 'Enter' && shown[hl]) { e.preventDefault(); go(shown[hl], e.shiftKey); }
  });

  document.addEventListener('click', e => {
    const o = e.target.closest('[data-open]'); if (!o) return;
    e.preventDefault();
    ({ palette: openPal, quickadd: openQA, shortcuts: () => kb.classList.add('open') })[o.dataset.open]?.();
  });

  // ---------- global keys ----------
  let gPending = 0;
  document.addEventListener('keydown', e => {
    const typing = e.target.closest?.('input, textarea, select, [contenteditable]');
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); pal.classList.contains('open') ? pal.classList.remove('open') : openPal(); return; }
    if (e.key === 'Escape') { $$('.modal-backdrop.open, .palette-backdrop.open').forEach(m => m.classList.remove('open')); closeDrawer(); return; }
    if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
    if (gPending && Date.now() - gPending < 1200) {
      const t = nav.concat(navOps).find(n => n[6] === e.key.toLowerCase());
      gPending = 0; if (t) location.href = t[3]; return;
    }
    if (e.key === 'g') gPending = Date.now();
    else if (e.key === '/') { e.preventDefault(); openPal(); }
    else if (e.key === 'n') { e.preventDefault(); openQA(); }
    else if (e.key === '?') { kb.classList.add('open'); }
  });

  // deep link: page.html#drawer-ssh-fix opens that drawer
  if (location.hash.startsWith('#drawer-')) openDrawer(location.hash.slice(8));
})();

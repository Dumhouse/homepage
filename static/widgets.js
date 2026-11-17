// Home Services widgets: drawer, styled selects, date pickers that follow the
// display format from Settings, add/remove form rows, scan progress.
(() => {
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const root = document.documentElement;
  const el = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; };
  const chevron = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>';

  // ---------------- drawer: <button data-drawer="x"> + <template id="drawer-x"> ----------------
  const backdrop = el('div', 'drawer-backdrop');
  const drawer = el('aside', 'drawer');
  drawer.setAttribute('role', 'dialog');
  drawer.setAttribute('aria-modal', 'true');
  document.body.append(backdrop, drawer);
  let lastFocus = null;
  const closeDrawer = () => {
    drawer.classList.remove('open'); backdrop.classList.remove('open');
    setTimeout(() => { if (!drawer.classList.contains('open')) drawer.replaceChildren(); }, 250);
    lastFocus?.focus?.();
  };
  const openDrawer = (id, opener) => {
    const tpl = document.getElementById('drawer-' + id);
    if (!tpl) return;
    lastFocus = opener;
    drawer.replaceChildren(tpl.content.cloneNode(true));
    // carry data-* values from the opener into fields named by data-set-*
    if (opener) for (const [k, v] of Object.entries(opener.dataset)) {
      if (!k.startsWith('set')) continue;
      const name = k.slice(3).replace(/^./, c => c.toLowerCase()).replace(/[A-Z]/g, c => '_' + c.toLowerCase());
      $$(`[name="${name}"]`, drawer).forEach(f => { f.value = v; });
      $$(`[data-fill="${name}"]`, drawer).forEach(f => { f.textContent = v; });
    }
    $$('[data-src]', drawer).forEach(async pre => {
      try {
        const txt = await (await fetch(pre.dataset.src)).text();
        pre.textContent = txt;
        $$('[data-copy-src]', drawer).forEach(b => { b.dataset.copy = txt; });
      } catch { pre.textContent = 'Could not load.'; }
    });
    if (opener?.dataset.action) $('form', drawer)?.setAttribute('action', opener.dataset.action);
    enhance(drawer);
    drawer.classList.add('open'); backdrop.classList.add('open');
    setTimeout(() => $('input:not([type=hidden]):not([readonly]), textarea, .sel-btn', drawer)?.focus(), 60);
  };
  document.addEventListener('click', e => {
    const o = e.target.closest('[data-drawer]');
    if (o) { e.preventDefault(); openDrawer(o.dataset.drawer, o); return; }
    if (e.target.closest('[data-close]') && drawer.contains(e.target)) { e.preventDefault(); closeDrawer(); }
  });
  backdrop.addEventListener('click', closeDrawer);
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && drawer.classList.contains('open') && !$('.sel-pop.open, .dp-pop.open')) closeDrawer();
  });

  // ---------------- styled select ----------------
  let openSel = null;
  const closeSel = () => { if (!openSel) return; openSel.pop.classList.remove('open'); openSel.btn.setAttribute('aria-expanded', 'false'); openSel = null; };
  const enhanceSelect = sel => {
    if (sel.dataset.enhanced || sel.multiple || sel.size > 1) return;
    sel.dataset.enhanced = '1';
    const wrap = el('div', 'sel');
    if (sel.style.cssText) wrap.style.cssText = sel.style.cssText;
    sel.parentNode.insertBefore(wrap, sel);
    wrap.appendChild(sel);
    sel.classList.add('sel-native');
    sel.tabIndex = -1;
    const btn = el('button', 'input sel-btn');
    btn.type = 'button';
    btn.setAttribute('aria-haspopup', 'listbox');
    btn.setAttribute('aria-expanded', 'false');
    if (sel.id) { btn.id = sel.id + '-btn'; $$(`label[for="${sel.id}"]`).forEach(l => l.htmlFor = btn.id); }
    const lbl = el('span', 'sel-val');
    btn.append(lbl);
    btn.insertAdjacentHTML('beforeend', chevron);
    const pop = el('div', 'sel-pop');
    pop.setAttribute('role', 'listbox');
    wrap.append(btn);
    document.body.append(pop);
    let active = -1;
    const opts = () => [...sel.options].filter(o => !o.hidden);
    const sync = () => {
      const o = sel.options[sel.selectedIndex];
      lbl.textContent = o ? o.textContent : '';
      lbl.classList.toggle('muted', !o || o.value === '');
      btn.disabled = sel.disabled;
    };
    const render = () => {
      pop.replaceChildren();
      opts().forEach((o, i) => {
        const it = el('div', 'sel-opt' + (o.selected ? ' on' : '') + (o.disabled ? ' dis' : ''), o.textContent);
        it.setAttribute('role', 'option');
        it.dataset.i = i;
        it.addEventListener('mousedown', e => { e.preventDefault(); pick(i); });
        pop.append(it);
      });
    };
    const mark = i => {
      const items = $$('.sel-opt', pop);
      items.forEach(x => x.classList.remove('act'));
      active = Math.max(0, Math.min(items.length - 1, i));
      items[active]?.classList.add('act');
      items[active]?.scrollIntoView({ block: 'nearest' });
    };
    const place = () => {
      const r = btn.getBoundingClientRect();
      pop.style.minWidth = r.width + 'px';
      pop.style.left = Math.min(r.left, innerWidth - pop.offsetWidth - 8) + 'px';
      const below = innerHeight - r.bottom;
      if (below < 240 && r.top > below) { pop.style.top = ''; pop.style.bottom = (innerHeight - r.top + 4) + 'px'; }
      else { pop.style.bottom = ''; pop.style.top = (r.bottom + 4) + 'px'; }
    };
    const open = () => {
      if (sel.disabled) return;
      closeSel(); closeDp();
      render(); pop.classList.add('open'); place();
      btn.setAttribute('aria-expanded', 'true');
      openSel = { pop, btn };
      mark(opts().findIndex(o => o.selected));
    };
    const pick = i => {
      const o = opts()[i];
      if (!o || o.disabled) return;
      sel.value = o.value;
      sel.dispatchEvent(new Event('input', { bubbles: true }));
      sel.dispatchEvent(new Event('change', { bubbles: true }));
      sync(); closeSel(); btn.focus();
    };
    btn.addEventListener('click', () => (openSel?.pop === pop ? closeSel() : open()));
    let typed = '', typedAt = 0;
    btn.addEventListener('keydown', e => {
      const isOpen = openSel?.pop === pop;
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        if (!isOpen) open(); else mark(active + (e.key === 'ArrowDown' ? 1 : -1));
      } else if ((e.key === 'Enter' || e.key === ' ') && isOpen) { e.preventDefault(); pick(active); }
      else if (e.key === 'Escape' && isOpen) { e.preventDefault(); closeSel(); }
      else if (e.key === 'Tab') closeSel();
      else if (e.key.length === 1 && /\S/.test(e.key)) {
        const now = Date.now(); typed = (now - typedAt > 700 ? '' : typed) + e.key.toLowerCase(); typedAt = now;
        const i = opts().findIndex(o => o.textContent.trim().toLowerCase().startsWith(typed));
        if (i >= 0) { if (isOpen) mark(i); else pick(i); }
      }
    });
    sel.addEventListener('change', sync);
    sel.form?.addEventListener('reset', () => setTimeout(sync));
    sync();
  };
  document.addEventListener('mousedown', e => {
    if (openSel && !e.target.closest('.sel-pop') && !openSel.btn.contains(e.target)) closeSel();
    if (openDp && !e.target.closest('.dp-pop') && !openDp.input.contains(e.target)) closeDp();
  });
  addEventListener('resize', () => { closeSel(); closeDp(); });
  document.addEventListener('scroll', e => { if (!e.target.closest?.('.sel-pop, .dp-pop')) { closeSel(); closeDp(); } }, true);

  // ---------------- date / date-time picker ----------------
  const fmt = () => root.dataset.datefmt || 'long';
  const h12 = () => root.dataset.clock === '12';
  const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  const pad = n => String(n).padStart(2, '0');
  const iso = d => `${d.y}-${pad(d.m)}-${pad(d.d)}`;
  const showDate = d => {
    switch (fmt()) {
      case 'eu': return `${pad(d.d)}.${pad(d.m)}.${d.y}`;
      case 'iso': return iso(d);
      case 'us': return `${pad(d.m)}/${pad(d.d)}/${d.y}`;
      default: return `${MONTHS[d.m - 1]} ${d.d}, ${d.y}`;
    }
  };
  const showTime = (h, mi) => h12() ? `${(h % 12) || 12}:${pad(mi)} ${h < 12 ? 'AM' : 'PM'}` : `${pad(h)}:${pad(mi)}`;
  const placeholder = withTime => ({ eu: 'dd.mm.yyyy', iso: 'yyyy-mm-dd', us: 'mm/dd/yyyy', long: 'Jan 2, 2026' }[fmt()] || '') + (withTime ? (h12() ? ' h:mm AM' : ' hh:mm') : '');
  const valid = (y, m, d) => { const t = new Date(y, m - 1, d); return t.getFullYear() === y && t.getMonth() === m - 1 && t.getDate() === d; };
  // parse what the user typed in the current style (also accepts ISO)
  const parseTyped = (s, withTime) => {
    s = s.trim();
    if (!s) return null;
    let date = null, rest = s, m;
    if ((m = s.match(/^(\d{4})-(\d{1,2})-(\d{1,2})(.*)$/))) { date = { y: +m[1], m: +m[2], d: +m[3] }; rest = m[4]; }
    else if (fmt() === 'eu' && (m = s.match(/^(\d{1,2})[./-](\d{1,2})[./-](\d{4})(.*)$/))) { date = { y: +m[3], m: +m[2], d: +m[1] }; rest = m[4]; }
    else if ((m = s.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})(.*)$/))) { date = { y: +m[3], m: +m[1], d: +m[2] }; rest = m[4]; }
    else if ((m = s.match(/^(\d{1,2})\.(\d{1,2})\.(\d{4})(.*)$/))) { date = { y: +m[3], m: +m[2], d: +m[1] }; rest = m[4]; }
    else if ((m = s.match(/^([A-Za-z]{3})[a-z]*\.?\s+(\d{1,2}),?\s+(\d{4})(.*)$/))) {
      const mi = MONTHS.findIndex(x => x.toLowerCase() === m[1].toLowerCase());
      if (mi >= 0) { date = { y: +m[3], m: mi + 1, d: +m[2] }; rest = m[4]; }
    }
    if (!date || !valid(date.y, date.m, date.d)) return null;
    if (withTime) {
      const t = rest.replace(/^[,\sT]+/, '').match(/^(\d{1,2}):(\d{2})(?::\d{2})?\s*([ap]\.?m\.?)?$/i);
      let h = 0, mi = 0;
      if (t) {
        h = +t[1]; mi = +t[2];
        if (t[3]) { const pm = /p/i.test(t[3]); if (h === 12) h = 0; if (pm) h += 12; }
        if (h > 23 || mi > 59) return null;
      } else if (rest.trim()) return null;
      date.h = h; date.mi = mi;
    }
    return date;
  };
  // data-utc inputs: the server stores UTC, the picker shows local time
  const fromValue = (v, withTime, utc) => {
    const m = (v || '').match(/^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2}))?/);
    if (!m) return null;
    let d = { y: +m[1], m: +m[2], d: +m[3] };
    if (withTime) {
      d.h = +(m[4] || 0); d.mi = +(m[5] || 0);
      if (utc) { const t = new Date(Date.UTC(d.y, d.m - 1, d.d, d.h, d.mi)); d = { y: t.getFullYear(), m: t.getMonth() + 1, d: t.getDate(), h: t.getHours(), mi: t.getMinutes() }; }
    }
    return d;
  };
  const toValue = (d, withTime, utc) => {
    if (!d) return '';
    if (withTime && utc) {
      const t = new Date(d.y, d.m - 1, d.d, d.h || 0, d.mi || 0);
      return `${t.getUTCFullYear()}-${pad(t.getUTCMonth() + 1)}-${pad(t.getUTCDate())}T${pad(t.getUTCHours())}:${pad(t.getUTCMinutes())}`;
    }
    return iso(d) + (withTime ? `T${pad(d.h || 0)}:${pad(d.mi || 0)}` : '');
  };
  const show = (d, withTime) => d ? showDate(d) + (withTime ? ', ' + showTime(d.h || 0, d.mi || 0) : '') : '';

  let openDp = null;
  const closeDp = () => { if (!openDp) return; openDp.pop.classList.remove('open'); openDp = null; };
  const enhanceDate = input => {
    if (input.dataset.enhanced) return;
    const withTime = input.type === 'datetime-local';
    if (input.type !== 'date' && !withTime) return;
    const utc = input.hasAttribute('data-utc');
    input.dataset.enhanced = '1';
    // the original input keeps the name and ISO value; a text input shows the chosen style
    const vis = el('input', input.className + ' dp-input');
    vis.type = 'text';
    vis.autocomplete = 'off';
    vis.placeholder = placeholder(withTime);
    vis.required = input.required;
    if (input.id) { vis.id = input.id; input.id = input.id + '-iso'; }
    input.type = 'hidden';
    input.required = false;
    input.parentNode.insertBefore(vis, input);
    const wrap = el('div', 'dp');
    vis.parentNode.insertBefore(wrap, vis);
    wrap.append(vis, input);
    const cal = el('button', 'dp-ic');
    cal.type = 'button'; cal.tabIndex = -1; cal.setAttribute('aria-label', 'Open calendar');
    cal.innerHTML = '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/></svg>';
    wrap.append(cal);
    let cur = fromValue(input.value, withTime, utc);
    vis.value = show(cur, withTime);
    const pop = el('div', 'dp-pop');
    document.body.append(pop);
    const today = new Date();
    let view = cur ? { y: cur.y, m: cur.m } : { y: today.getFullYear(), m: today.getMonth() + 1 };
    const commit = d => {
      cur = d;
      input.value = toValue(d, withTime, utc);
      vis.value = show(d, withTime);
      vis.classList.remove('bad');
      vis.setCustomValidity('');
      input.dispatchEvent(new Event('change', { bubbles: true }));
    };
    const render = () => {
      pop.replaceChildren();
      const head = el('div', 'dp-head');
      const prev = el('button', 'btn ghost sm icon', '‹'); prev.type = 'button';
      const next = el('button', 'btn ghost sm icon', '›'); next.type = 'button';
      const title = el('b', '', `${['January','February','March','April','May','June','July','August','September','October','November','December'][view.m - 1]} ${view.y}`);
      prev.onclick = () => { view.m--; if (view.m < 1) { view.m = 12; view.y--; } render(); };
      next.onclick = () => { view.m++; if (view.m > 12) { view.m = 1; view.y++; } render(); };
      head.append(prev, title, next);
      const grid = el('div', 'dp-grid');
      ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'].forEach(w => grid.append(el('span', 'dp-w', w)));
      const first = (new Date(view.y, view.m - 1, 1).getDay() + 6) % 7;
      const days = new Date(view.y, view.m, 0).getDate();
      for (let i = 0; i < first; i++) grid.append(el('span'));
      for (let d = 1; d <= days; d++) {
        const b = el('button', 'dp-d', d); b.type = 'button';
        if (cur && cur.y === view.y && cur.m === view.m && cur.d === d) b.classList.add('on');
        if (today.getFullYear() === view.y && today.getMonth() + 1 === view.m && today.getDate() === d) b.classList.add('today');
        b.onmousedown = e => {
          e.preventDefault();
          commit({ y: view.y, m: view.m, d, h: cur?.h ?? 9, mi: cur?.mi ?? 0 });
          if (!withTime) closeDp(); else render();
        };
        grid.append(b);
      }
      pop.append(head, grid);
      const foot = el('div', 'dp-foot');
      if (withTime) {
        const t = el('input', 'input dp-time'); t.type = 'text';
        t.value = cur ? showTime(cur.h || 0, cur.mi || 0) : '';
        t.placeholder = h12() ? '9:00 AM' : '09:00';
        t.onchange = () => {
          const p = parseTyped('2000-01-01 ' + t.value, true);
          if (p && cur) commit({ ...cur, h: p.h, mi: p.mi }); else t.classList.add('bad');
        };
        foot.append(el('span', 'muted xs', 'Time'), t);
      }
      const todayBtn = el('button', 'btn ghost sm', 'Today'); todayBtn.type = 'button';
      todayBtn.onmousedown = e => { e.preventDefault(); const n = new Date(); commit({ y: n.getFullYear(), m: n.getMonth() + 1, d: n.getDate(), h: n.getHours(), mi: n.getMinutes() }); view = { y: n.getFullYear(), m: n.getMonth() + 1 }; if (!withTime) closeDp(); else render(); };
      const clear = el('button', 'btn ghost sm', 'Clear'); clear.type = 'button';
      clear.onmousedown = e => { e.preventDefault(); commit(null); closeDp(); };
      foot.append(el('span', 'spacer'), clear, todayBtn);
      pop.append(foot);
    };
    const open = () => {
      closeSel(); closeDp();
      view = cur ? { y: cur.y, m: cur.m } : { y: today.getFullYear(), m: today.getMonth() + 1 };
      render(); pop.classList.add('open');
      const r = wrap.getBoundingClientRect();
      pop.style.left = Math.max(8, Math.min(r.left, innerWidth - pop.offsetWidth - 8)) + 'px';
      if (innerHeight - r.bottom < pop.offsetHeight + 8 && r.top > pop.offsetHeight) { pop.style.top = ''; pop.style.bottom = (innerHeight - r.top + 4) + 'px'; }
      else { pop.style.bottom = ''; pop.style.top = (r.bottom + 4) + 'px'; }
      openDp = { pop, input: wrap };
    };
    vis.addEventListener('focus', open);
    vis.addEventListener('click', () => { if (openDp?.pop !== pop) open(); });
    cal.addEventListener('click', () => { vis.focus(); open(); });
    vis.addEventListener('keydown', e => { if (e.key === 'Escape') closeDp(); if (e.key === 'Enter') { vis.blur(); } });
    // keep the hidden ISO value in step while typing, and check once more on submit
    vis.addEventListener('input', () => {
      const p = parseTyped(vis.value, withTime);
      if (p) { cur = p; input.value = toValue(p, withTime, utc); vis.classList.remove('bad'); vis.setCustomValidity(''); }
      else if (!vis.value.trim()) input.value = '';
    });
    vis.form?.addEventListener('submit', e => {
      if (!vis.value.trim()) { input.value = ''; return; }
      const p = parseTyped(vis.value, withTime);
      if (p) { input.value = toValue(p, withTime, utc); return; }
      e.preventDefault();
      vis.classList.add('bad');
      vis.setCustomValidity('Use ' + placeholder(withTime));
      vis.reportValidity();
    }, true);
    vis.addEventListener('blur', () => {
      if (!vis.value.trim()) { commit(null); return; }
      const p = parseTyped(vis.value, withTime);
      if (p) { commit(p); return; }
      vis.classList.add('bad');
      vis.setCustomValidity('Use ' + placeholder(withTime));
    });
  };

  // ---------------- repeatable rows: [data-rows] with <template data-row> and [data-add-row] ----------------
  document.addEventListener('click', e => {
    const add = e.target.closest('[data-add-row]');
    if (add) {
      const box = document.getElementById(add.dataset.addRow);
      const tpl = box && $('template', box);
      if (!tpl) return;
      const n = (+box.dataset.next || $$('[data-row-item]', box).length + 1);
      box.dataset.next = n + 1;
      const html = tpl.innerHTML.replaceAll('__N__', n);
      const holder = document.createElement('div');
      holder.innerHTML = html;
      const row = holder.firstElementChild;
      tpl.before(row);
      enhance(row);
      $('input:not([type=hidden])', row)?.focus();
      return;
    }
    const rm = e.target.closest('[data-remove-row]');
    if (rm) {
      const row = rm.closest('[data-row-item]');
      const box = row?.parentElement;
      row?.remove();
      if (box && !$$('[data-row-item]', box).length) box.querySelector('[data-empty]')?.removeAttribute('hidden');
    }
  });

  // ---------------- discovery: live scan progress ----------------
  const prog = $('#scanProgress');
  if (prog) {
    const bar = $('.bar i', prog), txt = $('[data-scan-text]', prog);
    let wasRunning = prog.dataset.running === '1';
    const tick = async () => {
      try {
        const st = await (await fetch('/discovery/status', { headers: { Accept: 'application/json' } })).json();
        if (st.running) {
          prog.hidden = false;
          bar.style.width = (st.percent || 0) + '%';
          const where = st.count > 1 ? ` (${st.index} of ${st.count})` : '';
          txt.textContent = st.phase === 'identify'
            ? `${st.percent || 0}% · identifying what answered on ${st.subnet}${where} (names, page titles)`
            : st.total ? `${st.percent || 0}% · checking ports on ${st.subnet}${where} (${st.done} of ${st.total} probes)` : `Starting ${st.subnet || ''}${where}…`;
          wasRunning = true;
          setTimeout(tick, 1000);
        } else if (wasRunning) {
          location.replace('/discovery?ok=scan-done');
        }
      } catch { setTimeout(tick, 3000); }
    };
    if (wasRunning) tick();
  }

  // ---------------- pop-ups (<details class="add-pop">) close on outside click / Esc ----------------
  document.addEventListener('click', e => {
    $$('details.add-pop[open]').forEach(d => { if (!d.contains(e.target) && !e.target.closest('.sel-pop, .dp-pop')) d.open = false; });
  });
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && !$('.sel-pop.open, .dp-pop.open')) $$('details.add-pop[open]').forEach(d => { d.open = false; });
  });

  // ---------------- tabs: <div class="tabs" data-tabs="#panel"> + [data-tab] cards ----------------
  // Overview shows every card; any other tab shows only its cards in one column.
  $$('.tabs[data-tabs]').forEach(bar => {
    const panel = $(bar.dataset.tabs);
    if (!panel) return;
    const links = $$('[data-tab-link]', bar);
    const first = links[0]?.dataset.tabLink;
    const show = name => {
      if (!links.some(a => a.dataset.tabLink === name)) name = first;
      $$('[data-tab-link]', bar).forEach(a => a.classList.toggle('active', a.dataset.tabLink === name));
      $$('[data-tab]', panel).forEach(c => {
        c.hidden = name !== 'overview' && !c.dataset.tab.split(' ').includes(name);
      });
      panel.classList.toggle('one-col', name !== 'overview');
    };
    bar.addEventListener('click', e => {
      const a = e.target.closest('[data-tab-link]');
      if (!a) return;
      e.preventDefault();
      show(a.dataset.tabLink);
      history.replaceState(null, '', a.dataset.tabLink === first ? location.pathname + location.search : '#' + a.dataset.tabLink);
    });
    show(location.hash.slice(1));
  });

  // ---------------- async forms: run a slow POST without freezing the page ----------------
  // form[data-async]: the button spins while the request runs; the page then
  // loads the redirect target (which carries the result toast).
  document.addEventListener('submit', async e => {
    const f = e.target.closest('form[data-async]');
    if (!f || e.defaultPrevented) return;
    e.preventDefault();
    if (f.dataset.busy) return;
    f.dataset.busy = '1';
    const btn = $('button', f);
    btn?.classList.add('busy');
    if (btn) btn.disabled = true;
    try {
      const res = await fetch(f.action, { method: 'POST', body: new FormData(f), credentials: 'same-origin' });
      const dest = new URL(res.url, location.href);
      if (dest.pathname === location.pathname && location.hash) dest.hash = location.hash;
      location.assign(dest.href);
    } catch {
      delete f.dataset.busy;
      btn?.classList.remove('busy');
      if (btn) btn.disabled = false;
      alert('The request failed — is Home Services still running?');
    }
  });

  // ---------------- enhance ----------------
  function enhance(scope) {
    $$('select.input', scope).forEach(enhanceSelect);
    $$('input[type=date], input[type=datetime-local]', scope).forEach(enhanceDate);
  }
  enhance(document);
  window.hsEnhance = enhance;
})();

// ---------------- sortable tables: <table class="table sortable"> ----------------
// Click a column header to sort by it; click again to reverse. The choice is
// remembered per page. Cells may carry data-sort="…" when the visible text is
// not what should be compared (dates, formatted amounts).
(() => {
  const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
  const key = t => 'sort:' + location.pathname + ':' + ([...document.querySelectorAll('table.sortable')].indexOf(t));
  const val = (tr, i) => {
    const c = tr.children[i];
    return c ? (c.dataset.sort ?? c.textContent).trim() : '';
  };
  const apply = (table, col, dir) => {
    const body = table.tBodies[0];
    if (!body) return;
    const rows = [...body.rows].filter(r => r.children.length > col);
    const num = rows.every(r => { const v = val(r, col).replace(/[^\d.\-]/g, ''); return v !== '' && !isNaN(+v); });
    rows.sort((a, b) => {
      const x = val(a, col), y = val(b, col);
      if (num) return (parseFloat(x.replace(/[^\d.\-]/g, '')) - parseFloat(y.replace(/[^\d.\-]/g, ''))) * dir;
      if (x === '' || y === '') return x === y ? 0 : (x === '' ? 1 : -1); // empty cells last
      return collator.compare(x, y) * dir;
    });
    rows.forEach(r => body.appendChild(r));
    [...table.tHead.rows[0].cells].forEach((th, i) => th.setAttribute('aria-sort', i === col ? (dir > 0 ? 'ascending' : 'descending') : 'none'));
  };
  document.querySelectorAll('table.sortable').forEach(table => {
    if (!table.tHead) return;
    [...table.tHead.rows[0].cells].forEach((th, i) => {
      if (!th.textContent.trim().replace('?', '')) return; // action columns
      th.classList.add('sort-h');
      th.tabIndex = 0;
      th.setAttribute('role', 'columnheader');
      const go = () => {
        const dir = th.getAttribute('aria-sort') === 'ascending' ? -1 : 1;
        apply(table, i, dir);
        try { localStorage.setItem(key(table), i + ':' + dir); } catch {}
      };
      th.addEventListener('click', e => { if (!e.target.closest('.help, a, button')) go(); });
      th.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); go(); } });
    });
    try {
      const saved = (localStorage.getItem(key(table)) || '').split(':');
      if (saved.length === 2) apply(table, +saved[0], +saved[1]);
    } catch {}
  });
})();

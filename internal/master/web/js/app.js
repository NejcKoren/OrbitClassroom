'use strict';
// Orbit Master – user interface
// Talks to the local master process (/api/...). State arrives through
// Server-Sent Events (/api/events) and is rendered incrementally.

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const clamp = (v, a, b) => Math.min(b, Math.max(a, v));
const debounce = (fn, ms) => { let h; return (...a) => { clearTimeout(h); h = setTimeout(() => fn(...a), ms); }; };

async function api(path, opts = {}) {
  const o = { method: opts.method || (opts.body !== undefined ? 'POST' : 'GET'), headers: {} };
  if (opts.body !== undefined) { o.body = JSON.stringify(opts.body); o.headers['Content-Type'] = 'application/json'; }
  const r = await fetch('/api/' + path, o);
  const ct = r.headers.get('Content-Type') || '';
  const data = ct.includes('json') ? await r.json() : await r.text();
  if (!r.ok) throw new Error((data && data.error) || ('HTTP ' + r.status));
  return data;
}

const store = {
  get(k, d) { try { const v = localStorage.getItem('orbit.' + k); return v === null ? d : JSON.parse(v); } catch { return d; } },
  set(k, v) { try { localStorage.setItem('orbit.' + k, JSON.stringify(v)); } catch {} },
};

const S = {
  meta: {}, settings: {}, langs: [],
  state: { computers: [], rooms: [], inbox: [], poll: null, demo: null },
  view: 'computers', room: store.get('room', null), selected: new Set(), lastClicked: null,
  filter: 'all', query: '', spot: false,
  tiles: new Map(), visible: new Set(),
  discover: { results: null, busy: false, scan: false, picked: new Set() },
  shots: null, docLang: null, connected: true, handsSeen: new Set(),
};

const ACCENTS = {
  indigo: ['#6b7cff', '107,124,255', '#9b6bff'], violet: ['#9b6bff', '155,107,255', '#d06bff'],
  sky: ['#38bdf8', '56,189,248', '#6b7cff'], teal: ['#14b8a6', '20,184,166', '#38bdf8'],
  green: ['#22c55e', '34,197,94', '#14b8a6'], orange: ['#f97316', '249,115,22', '#f43f5e'],
  pink: ['#ec4899', '236,72,153', '#9b6bff'], red: ['#ef4444', '239,68,68', '#f97316'],
};

// ===================================================================== boot

async function init() {
  try {
    const meta = await api('settings');
    applyMeta(meta);
  } catch (e) {
    document.body.innerHTML = `<div class="empty"><h2>Orbit Master</h2><p>${esc(e.message)}</p><p>Please restart OrbitMaster.exe.</p></div>`;
    return;
  }
  await loadLang(S.settings.language || pickLang());
  try { S.langs = await api('langs'); } catch { S.langs = [{ code: 'en', name: 'English' }]; }
  applyTheme();
  renderShell();
  connectSSE();
  bindKeys();
  if (!S.settings.onboarded) setTimeout(onboarding, 300);
}

function pickLang() {
  const cands = [S.meta.systemLanguage, (navigator.language || 'en').slice(0, 2)];
  return cands.find(l => l === 'sl' || l === 'en') || 'en';
}

function applyMeta(meta) {
  S.meta = meta;
  S.settings = meta.settings;
}

function applyTheme() {
  let th = S.settings.theme || 'dark';
  if (th === 'auto') th = matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  document.documentElement.dataset.theme = th;
  const a = ACCENTS[S.settings.accent] || ACCENTS.indigo;
  const r = document.documentElement.style;
  r.setProperty('--accent', a[0]); r.setProperty('--accent-rgb', a[1]); r.setProperty('--accent-2', a[2]);
  r.setProperty('--tile', (S.settings.tileSize || 300) + 'px');
}
matchMedia('(prefers-color-scheme: light)').addEventListener('change', () => applyTheme());

const saveSettings = (() => {
  let pending = {};
  const flush = debounce(async () => {
    const patch = pending; pending = {};
    try { applyMeta(await api('settings', { method: 'PUT', body: { ...S.settings, ...patch } })); }
    catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); }
  }, 350);
  return patch => { Object.assign(S.settings, patch); Object.assign(pending, patch); flush(); };
})();

// ===================================================================== live state

function connectSSE() {
  let fails = 0;
  const es = new EventSource('/api/events');
  es.addEventListener('state', e => { fails = 0; setConnected(true); onState(JSON.parse(e.data)); });
  es.addEventListener('toast', e => onServerToast(JSON.parse(e.data)));
  es.addEventListener('quit', () => { es.close(); showClosed(); });
  es.onerror = () => { if (++fails > 3) setConnected(false); };
}

function setConnected(ok) {
  if (S.connected === ok) return;
  S.connected = ok;
  if (!ok) toast(t('toast.disconnected'), 'bad', { text: t('toast.disconnectedText'), timeout: 6000 });
}

function showClosed() {
  document.body.innerHTML = `<div class="empty" style="height:100vh;align-content:center"><div class="big">${icon('logo')}</div><h2>${esc(t('closed.title'))}</h2><p>${esc(t('closed.text'))}</p></div>`;
}

function onState(st) {
  const prevPoll = S.state.poll;
  S.state = st;
  if (!st.rooms.find(r => r.id === S.room)) S.room = st.rooms[0] ? st.rooms[0].id : null;
  const ids = new Set(st.computers.map(c => c.id));
  for (const id of [...S.selected]) if (!ids.has(id)) S.selected.delete(id);
  for (const [id, el] of S.tiles) if (!ids.has(id)) { el.remove(); S.tiles.delete(id); }
  renderSidebar();
  renderTopPills();
  renderTitle();
  if (S.view === 'computers') { renderGrid(); updateToolbar(); renderSelbar(); }
  if (S.view === 'inbox') renderInbox();
  if (S.view === 'polls' || (S.view === 'polls' && prevPoll !== st.poll)) renderPolls();
  if (S.view === 'discover' && S.discover.results) renderDiscover();
}

function onServerToast(ev) {
  if (ev.kind === 'hand') {
    sound('hand');
    toast(t('toast.hand', { name: ev.name }), 'hand', {
      icon: 'hand', text: ev.user, timeout: 9000,
      actions: [{ label: t('act.view'), onClick: () => openViewerById(ev.id, false) }, { label: t('act.lowerHand'), onClick: () => act('hand_lower', {}, [comp(ev.id)].filter(Boolean)) }],
    });
  } else if (ev.kind === 'inbox') {
    sound('msg');
    toast(t('toast.inbox', { name: ev.name }), 'info', { icon: 'inbox', text: ev.text, timeout: 9000, actions: [{ label: t('inbox.open'), onClick: () => setView('inbox') }] });
  }
}

let audioCtx;
function sound(kind) {
  if (!S.settings.sounds) return;
  try {
    audioCtx = audioCtx || new AudioContext();
    const notes = kind === 'hand' ? [660, 880] : [740];
    notes.forEach((f, i) => {
      const o = audioCtx.createOscillator(), g = audioCtx.createGain();
      o.frequency.value = f; o.type = 'sine';
      const t0 = audioCtx.currentTime + i * 0.14;
      g.gain.setValueAtTime(0, t0); g.gain.linearRampToValueAtTime(0.12, t0 + 0.02); g.gain.exponentialRampToValueAtTime(0.001, t0 + 0.35);
      o.connect(g).connect(audioCtx.destination); o.start(t0); o.stop(t0 + 0.4);
    });
  } catch {}
}

// ===================================================================== helpers

const comp = id => S.state.computers.find(c => c.id === id);
const roomComputers = () => S.state.computers.filter(c => c.room === S.room);
const online = c => c && c.state === 'online';
const roomName = id => (S.state.rooms.find(r => r.id === id) || {}).name || '';

function shownComputers() {
  let list = roomComputers();
  const q = S.query.trim().toLowerCase();
  if (q) list = list.filter(c => [c.name, c.host, c.st && c.st.user, c.st && c.st.app].some(v => (v || '').toLowerCase().includes(q)));
  switch (S.filter) {
    case 'online': list = list.filter(online); break;
    case 'offline': list = list.filter(c => !online(c)); break;
    case 'hands': list = list.filter(c => c.st && c.st.hand); break;
    case 'locked': list = list.filter(c => c.st && c.st.locked); break;
  }
  if (S.spot && S.selected.size) list = list.filter(c => S.selected.has(c.id));
  return list;
}

// targets: the selected computers, or every shown computer when nothing is selected
function targets(includeOffline = false) {
  let list = S.selected.size ? roomComputers().filter(c => S.selected.has(c.id)) : shownComputers();
  if (!includeOffline) list = list.filter(online);
  return list;
}

function report(results, okText) {
  const entries = Object.entries(results || {});
  const bad = entries.filter(([, e]) => e);
  if (!bad.length) { toast(okText, 'good'); return; }
  const detail = bad.slice(0, 6).map(([id, e]) => `${(comp(id) || {}).name || id}: ${errTr(e)}`).join('\n');
  toast(t('toast.partial', { ok: entries.length - bad.length, total: entries.length }), 'bad', { text: detail, timeout: 9000 });
}

async function act(action, params = {}, list = targets(), okKey) {
  if (!list.length) { toast(t('toast.noTargets'), 'bad', { text: t('toast.noTargetsText') }); return null; }
  try {
    const res = await api('action', { body: { targets: list.map(c => c.id), action, params } });
    report(res.results, t(okKey || ('done.' + action), { n: list.length }));
    return res;
  } catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); return null; }
}

// errTr translates well-known error messages coming from agents / the network.
const ERRS = [
  [/key not trusted/i, 'err.notTrusted'], [/connection refused/i, 'err.refused'], [/timeout|deadline/i, 'err.timeout'],
  [/no route to host|network is unreachable/i, 'err.noRoute'], [/no such host/i, 'err.noHost'], [/not connected/i, 'err.notConnected'],
  [/unknown action/i, 'err.oldAgent'], [/only http/i, 'err.onlyHttp'], [/remote control is disabled/i, 'viewer.ctlDisabled'],
  [/screen unavailable/i, 'err.screen'], [/certificate changed/i, 'err.cert'],
];
function errTr(e) {
  if (!e) return '';
  for (const [re, k] of ERRS) if (re.test(e)) return t(k);
  return e;
}

function initials(s) {
  const p = String(s || '?').replace(/[^\p{L}\p{N} ]/gu, ' ').trim().split(/\s+/);
  return ((p[0] || '?')[0] + (p[1] ? p[1][0] : (p[0] || '')[1] || '')).toUpperCase();
}

// ===================================================================== shell

const NAV = [
  { id: 'computers', icon: 'grid', label: 'nav.computers' },
  { id: 'discover', icon: 'radar', label: 'nav.discover' },
  { id: 'inbox', icon: 'inbox', label: 'nav.inbox' },
  { id: 'polls', icon: 'poll', label: 'nav.polls' },
  { id: 'shots', icon: 'image', label: 'nav.shots' },
];

function renderShell() {
  $('#paletteBtn').innerHTML = `${icon('search')}<span>${esc(t('top.search'))}</span><kbd>Ctrl K</kbd>`;
  $('#paletteBtn').onclick = openPalette;
  buildToolbar();
  renderSidebar();
  setView(S.view);
}

function renderSidebar() {
  const sb = $('#sidebar');
  const unread = S.state.inbox.filter(m => !m.read).length;
  const hands = S.state.computers.filter(c => c.st && c.st.hand).length;
  const pollActive = S.state.poll && S.state.poll.active;
  const counts = { inbox: unread, polls: pollActive ? '•' : 0 };
  const rooms = S.state.rooms.map(r => {
    const all = S.state.computers.filter(c => c.room === r.id);
    const on = all.filter(online).length;
    return `<div class="room-item" data-room="${esc(r.id)}">
      <button class="nav-item ${S.view === 'computers' && S.room === r.id ? 'active' : ''}" data-go-room="${esc(r.id)}">${icon('layers')}<span>${esc(r.name)}</span><span class="room-n">${on}/${all.length}</span></button>
      <button class="room-edit" data-room-menu="${esc(r.id)}" title="${esc(t('room.options'))}">${icon('dots')}</button></div>`;
  }).join('');
  const html = `
    <div class="brand"><div class="brand-logo">${icon('logo')}</div><div><b>Orbit</b><small>Classroom · Master</small></div></div>
    ${NAV.map(n => `<button class="nav-item ${S.view === n.id ? 'active' : ''}" data-nav="${n.id}">${icon(n.icon)}<span>${esc(t(n.label))}</span>${counts[n.id] ? `<span class="count">${counts[n.id]}</span>` : ''}${n.id === 'computers' && hands ? `<span class="count warn" title="${esc(t('filter.hands'))}">${icon('hand')} ${hands}</span>` : ''}</button>`).join('')}
    <div class="side-title"><span>${esc(t('nav.rooms'))}</span><button id="addRoomBtn" title="${esc(t('room.add'))}">${icon('plus')}</button></div>
    ${rooms}
    <div class="sidebar-foot">
      <button class="nav-item ${S.view === 'help' ? 'active' : ''}" data-nav="help">${icon('help')}<span>${esc(t('nav.help'))}</span></button>
      <button class="nav-item ${S.view === 'settings' ? 'active' : ''}" data-nav="settings">${icon('settings')}<span>${esc(t('nav.settings'))}</span></button>
      <div class="foot-row">
        <button class="icon-btn" id="themeBtn" title="${esc(t('settings.theme'))}">${icon(document.documentElement.dataset.theme === 'dark' ? 'sun' : 'moon')}</button>
        <button class="icon-btn" id="langBtn" title="${esc(t('settings.language'))}">${icon('language')}<small style="margin-left:4px;font-weight:700">${esc(I18N.lang.toUpperCase())}</small></button>
      </div>
      <div class="version">Orbit Classroom ${esc(S.meta.version || '')}</div>
    </div>`;
  if (sb._html === html) return;
  sb._html = html;
  sb.innerHTML = html;
  $$('[data-nav]', sb).forEach(b => b.onclick = () => setView(b.dataset.nav));
  $$('[data-go-room]', sb).forEach(b => b.onclick = () => { selectRoom(b.dataset.goRoom); setView('computers'); });
  $$('[data-room-menu]', sb).forEach(b => b.onclick = e => { e.stopPropagation(); roomMenu(b, b.dataset.roomMenu); });
  $$('.room-item', sb).forEach(el => {
    el.addEventListener('dragover', e => { if (e.dataTransfer.types.includes('text/orbit-ids')) { e.preventDefault(); el.classList.add('drop'); } });
    el.addEventListener('dragleave', () => el.classList.remove('drop'));
    el.addEventListener('drop', async e => {
      e.preventDefault(); el.classList.remove('drop');
      const ids = JSON.parse(e.dataTransfer.getData('text/orbit-ids') || '[]');
      if (ids.length) { await api('computers/move', { body: { ids, room: el.dataset.room } }); toast(tn('toast.moved', ids.length, { room: roomName(el.dataset.room) }), 'good'); }
    });
  });
  $('#addRoomBtn').onclick = addRoomDialog;
  $('#themeBtn').onclick = () => { const cur = document.documentElement.dataset.theme; saveSettings({ theme: cur === 'dark' ? 'light' : 'dark' }); applyTheme(); sb._html = ''; renderSidebar(); };
  $('#langBtn').onclick = e => showMenu(e.currentTarget, S.langs.map(l => ({ label: l.name, hint: l.code.toUpperCase(), icon: l.code === I18N.lang ? 'check' : 'language', onClick: () => switchLang(l.code) })));
}

async function switchLang(code) {
  saveSettings({ language: code });
  await loadLang(code);
  $('#sidebar')._html = '';
  S.tiles.forEach(el => el._sig = '');
  renderShell();
  if (S.view === 'help') renderHelp();
}

function selectRoom(id) {
  if (S.room !== id) { S.selected.clear(); S.room = id; store.set('room', id); }
  renderSidebar();
  renderGrid(); renderSelbar(); updateToolbar(); renderTitle();
}

function setView(v) {
  S.view = v;
  $$('.view').forEach(el => el.classList.toggle('active', el.id === 'view-' + v));
  renderSidebar();
  renderTitle();
  if (v === 'computers') { renderGrid(); renderSelbar(); updateToolbar(); }
  if (v === 'discover') renderDiscover();
  if (v === 'inbox') { renderInbox(); if (S.state.inbox.some(m => !m.read)) setTimeout(() => api('inbox/read', { body: {} }), 1500); }
  if (v === 'polls') renderPolls();
  if (v === 'shots') loadShots();
  if (v === 'help') renderHelp();
  if (v === 'settings') renderSettings();
  sendView();
}

function renderTitle() {
  let title = '', sub = '';
  if (S.view === 'computers') {
    const all = roomComputers();
    title = roomName(S.room) || t('nav.computers');
    sub = all.length ? t('top.onlineOf', { on: all.filter(online).length, n: all.length }) : '';
  } else {
    title = t('nav.' + S.view);
  }
  $('#viewTitle').textContent = title;
  $('#viewSub').textContent = sub;
  document.title = title + ' – Orbit Master';
}

function renderTopPills() {
  const el = $('#topPills');
  const d = S.state.demo;
  const hands = S.state.computers.filter(c => c.room === S.room && c.st && c.st.hand).length;
  let html = '';
  if (hands) html += `<button class="top-pill hand" id="pillHands">${icon('hand')} ${esc(tn('top.hands', hands))}</button>`;
  if (d) html += `<div class="top-pill demo"><span class="live-dot"></span>${esc(d.source === 'teacher' ? t('demo.pillTeacher') : t('demo.pillStudent', { name: d.sourceName }))} · ${d.connected}/${d.targets}<button id="pillStop">${esc(t('demo.stop'))}</button></div>`;
  if (el._html === html) return;
  el._html = html;
  el.innerHTML = html;
  const ph = $('#pillHands');
  if (ph) ph.onclick = () => { setView('computers'); S.filter = 'hands'; renderSelbar(); renderGrid(); };
  const ps = $('#pillStop');
  if (ps) ps.onclick = stopDemo;
}

// ===================================================================== toolbar

const TOOLS = [
  { id: 'demo', icon: 'cast', label: 'act.demo', caret: true },
  { id: 'lock', icon: 'lock', label: 'act.lock', caret: true },
  { sep: true },
  { id: 'view', icon: 'eye', label: 'act.view' },
  { id: 'control', icon: 'pointer', label: 'act.control' },
  { sep: true },
  { id: 'message', icon: 'message', label: 'act.message' },
  { id: 'poll', icon: 'poll', label: 'act.poll' },
  { id: 'website', icon: 'globe', label: 'act.website' },
  { id: 'run', icon: 'run', label: 'act.run' },
  { id: 'file', icon: 'fileup', label: 'act.file' },
  { id: 'screenshot', icon: 'camera', label: 'act.screenshot' },
  { id: 'focus', icon: 'target', label: 'act.focus' },
  { sep: true },
  { id: 'power', icon: 'power', label: 'act.power', caret: true },
  { id: 'more', icon: 'dots', label: 'act.more', caret: true },
];

function buildToolbar() {
  const tb = $('#toolbar');
  tb.innerHTML = TOOLS.map(x => x.sep ? '<div class="tool-sep"></div>' :
    `<button class="tool" data-tool="${x.id}" title="${esc(t(x.label + '.tip'))}">${icon(x.icon)}<span>${esc(t(x.label))}</span>${x.caret ? icon('chevron', 'caret') : ''}</button>`).join('');
  $$('[data-tool]', tb).forEach(b => b.onclick = () => runTool(b.dataset.tool, b));
}

function updateToolbar() {
  const list = targets();
  const tb = $('#toolbar');
  const set = (id, on, disabled, label, ic) => {
    const b = $(`[data-tool="${id}"]`, tb);
    if (!b) return;
    b.classList.toggle('on', !!on);
    b.disabled = !!disabled;
    if (label) b.querySelector('span').textContent = label;
    if (ic) { const svg = b.querySelector('svg.ic:not(.caret)'); if (svg && svg.dataset.n !== ic) { svg.outerHTML = icon(ic); const s2 = b.querySelector('svg.ic:not(.caret)'); s2.dataset.n = ic; } }
  };
  const none = !list.length;
  const allLocked = list.length && list.every(c => c.st && c.st.locked);
  set('demo', !!S.state.demo, none && !S.state.demo, S.state.demo ? t('act.demoStop') : t('act.demo'), S.state.demo ? 'stop' : 'cast');
  set('lock', allLocked, none, allLocked ? t('act.unlock') : t('act.lock'), allLocked ? 'unlock' : 'lock');
  const one = list.length === 1 || (S.selected.size === 1);
  set('view', false, none);
  set('control', false, none || !one);
  ['message', 'poll', 'website', 'run', 'file', 'screenshot'].forEach(id => set(id, false, none));
  set('focus', list.length && list.every(c => c.st && c.st.focus), none);
  set('power', false, !targets(true).length);
}

function runTool(id, btn) {
  const list = targets();
  switch (id) {
    case 'demo':
      if (S.state.demo) return stopDemo();
      return showMenu(btn, [
        { icon: 'maximize', label: t('demo.fullscreen'), hint: t('demo.fullscreenHint'), onClick: () => startDemo('teacher', 'fullscreen') },
        { icon: 'monitor', label: t('demo.window'), hint: t('demo.windowHint'), onClick: () => startDemo('teacher', 'window') },
        { sep: true },
        { icon: 'users', label: t('demo.student'), hint: t('demo.studentHint'), onClick: shareStudentDialog },
      ]);
    case 'lock': {
      const allLocked = list.length && list.every(c => c.st && c.st.locked);
      return showMenu(btn, [
        allLocked ? { icon: 'unlock', label: t('lock.unlock'), onClick: () => act('unlock', {}, list) } : { icon: 'lock', label: t('lock.screen'), hint: 'L', onClick: () => act('lock', { mode: 'screen' }, list) },
        { icon: 'keyboardOff', label: t('lock.input'), onClick: () => act('lock', { mode: 'input' }, list) },
        { icon: 'edit', label: t('lock.custom'), onClick: () => lockDialog(list) },
        { sep: true },
        { icon: 'unlock', label: t('lock.unlockAll'), onClick: () => act('unlock', {}, roomComputers().filter(online)) },
      ]);
    }
    case 'view': return pickOneThen(list, c => openViewer(c, false));
    case 'control': return pickOneThen(list, c => openViewer(c, true));
    case 'message': return messageDialog(list);
    case 'poll': return pollDialog(list);
    case 'website': return websiteDialog(list);
    case 'run': return runDialog(list);
    case 'file': return fileDialog(list);
    case 'screenshot': return takeScreenshots(list);
    case 'focus': return focusDialog(list);
    case 'power': return showMenu(btn, powerItems(targets(true)));
    case 'more': return showMenu(btn, moreItems());
  }
}

function pickOneThen(list, fn) {
  const sel = S.selected.size === 1 ? comp([...S.selected][0]) : (list.length === 1 ? list[0] : null);
  if (sel && online(sel)) return fn(sel);
  if (list.length > 1) {
    return showMenu($('[data-tool="view"]'), [{ title: t('pick.one') }, ...list.slice(0, 40).map(c => ({ icon: 'monitor', label: c.name, hint: c.st && c.st.user, onClick: () => fn(c) }))]);
  }
  toast(t('toast.noTargets'), 'bad');
}

function powerItems(list) {
  const off = list.filter(c => !online(c));
  const on = list.filter(online);
  return [
    { icon: 'zap', label: t('power.on'), hint: off.length ? String(off.length) : '', onClick: () => wake(off.length ? off : list) },
    { sep: true },
    { icon: 'restart', label: t('power.reboot'), onClick: () => powerDialog('reboot', on) },
    { icon: 'power', label: t('power.shutdown'), onClick: () => powerDialog('shutdown', on) },
    { icon: 'logout', label: t('power.logoff'), onClick: () => powerDialog('logoff', on) },
    { sep: true },
    { icon: 'x', label: t('power.abort'), onClick: () => act('power', { mode: 'abort' }, on, 'done.abort') },
  ];
}

function moreItems() {
  const items = [
    { icon: 'hand', label: t('more.lowerHands'), onClick: () => act('hand_lower', {}, roomComputers().filter(c => c.st && c.st.hand), 'done.hand_lower') },
    { icon: 'star', label: S.spot ? t('more.spotOff') : t('more.spotOn'), hint: t('more.spotHint'), onClick: toggleSpot },
    { icon: 'check', label: t('more.selectAll'), hint: 'Ctrl+A', onClick: selectAll },
    { icon: 'plus', label: t('computer.add'), onClick: () => computerDialog() },
    { icon: 'radar', label: t('nav.discover'), onClick: () => setView('discover') },
  ];
  const ca = S.settings.customActions || [];
  if (ca.length) {
    items.push({ sep: true }, { title: t('more.custom') });
    ca.forEach(a => items.push({ icon: a.type === 'url' ? 'globe' : a.type === 'message' ? 'message' : 'run', label: a.name, onClick: () => runCustom(a) }));
  }
  items.push({ sep: true }, { icon: 'settings', label: t('more.manageCustom'), onClick: () => { setView('settings'); setTimeout(() => { const el = $('#set-custom'); el && el.scrollIntoView({ behavior: 'smooth' }); }, 100); } });
  return items;
}

function runCustom(a) {
  const list = targets();
  if (a.type === 'url') return act('open_url', { url: a.value }, list, 'done.open_url');
  if (a.type === 'message') return act('message', { text: a.value }, list, 'done.message');
  return act('run', { program: a.value, args: a.args || '' }, list, 'done.run');
}

function toggleSpot() {
  if (!S.spot && !S.selected.size) { toast(t('more.spotNeedSel'), 'bad'); return; }
  S.spot = !S.spot;
  document.body.classList.toggle('spot', S.spot);
  renderGrid(); renderSelbar(); sendView();
}

function selectAll() { shownComputers().forEach(c => S.selected.add(c.id)); renderGrid(); renderSelbar(); updateToolbar(); }

// ===================================================================== selection bar & grid

function renderSelbar() {
  const bar = $('#selbar');
  const list = targets();
  const shown = shownComputers();
  const applies = S.selected.size ? tn('sel.selected', S.selected.size) : tn('sel.all', list.length);
  const chips = ['all', 'online', 'offline', 'hands', 'locked'].map(f => `<button class="chip ${S.filter === f ? 'on' : ''}" data-filter="${f}">${esc(t('filter.' + f))}</button>`).join('');
  bar.innerHTML = `
    <span class="applies">${icon('target', '')} ${t('sel.appliesTo')} <b>${esc(applies)}</b></span>
    ${S.selected.size ? `<button class="chip" id="clearSel">${icon('x')}${esc(t('sel.clear'))}</button>` : ''}
    ${S.spot ? `<button class="chip on" id="spotOff">${icon('star')}${esc(t('more.spotOff'))}</button>` : ''}
    <div class="spacer"></div>
    <div class="chips">${chips}</div>
    <input class="input" id="gridSearch" style="width:180px;padding:6px 10px" placeholder="${esc(t('sel.filter'))}" value="${esc(S.query)}">
    <div class="zoom" title="${esc(t('sel.zoom'))}">${icon('minus')}<input type="range" id="zoom" min="180" max="560" step="10" value="${S.settings.tileSize || 300}">${icon('plus')}</div>`;
  const sv = bar.querySelector('.applies svg'); if (sv) sv.style.cssText = 'width:15px;height:15px;vertical-align:-3px;margin-right:4px';
  $$('[data-filter]', bar).forEach(b => b.onclick = () => { S.filter = b.dataset.filter; renderSelbar(); renderGrid(); updateToolbar(); });
  const cs = $('#clearSel', bar); if (cs) cs.onclick = clearSelection;
  const so = $('#spotOff', bar); if (so) so.onclick = toggleSpot;
  const gs = $('#gridSearch', bar);
  gs.oninput = debounce(() => { S.query = gs.value; renderGrid(); updateToolbar(); const a = $('.applies b', bar); if (a) a.textContent = S.selected.size ? tn('sel.selected', S.selected.size) : tn('sel.all', targets().length); }, 150);
  const z = $('#zoom', bar);
  z.oninput = () => { document.documentElement.style.setProperty('--tile', z.value + 'px'); };
  z.onchange = () => { saveSettings({ tileSize: +z.value }); sendView(); };
  void shown;
}

function clearSelection() { S.selected.clear(); if (S.spot) { S.spot = false; document.body.classList.remove('spot'); } renderGrid(); renderSelbar(); updateToolbar(); sendView(); }

const io = new IntersectionObserver(entries => {
  for (const e of entries) { const id = e.target.dataset.id; if (e.isIntersecting) S.visible.add(id); else S.visible.delete(id); }
  sendView();
}, { root: null, rootMargin: '200px' });

let lastView = '';
const sendView = debounce(() => {
  const ids = S.view === 'computers' ? [...S.visible].filter(id => { const c = comp(id); return c && c.room === S.room; }) : [];
  const dpr = window.devicePixelRatio || 1;
  const tw = S.spot ? 960 : clamp(Math.round((S.settings.tileSize || 300) * dpr * 1.1), 200, 720);
  const body = { ids, fast: S.spot ? [...S.selected] : [], width: tw };
  const js = JSON.stringify(body);
  if (js === lastView) return;
  lastView = js;
  api('view', { body }).catch(() => {});
}, 120);

function renderGrid() {
  if (S.view !== 'computers') return;
  const grid = $('#grid');
  const list = shownComputers();
  const empty = $('#gridEmpty');
  if (!S.state.rooms.length) {
    empty.innerHTML = emptyState('layers', t('empty.noRoomsTitle'), t('empty.noRoomsText'), [`<button class="btn primary" id="eAddRoom">${icon('plus')}${esc(t('room.add'))}</button>`]);
    $('#eAddRoom').onclick = addRoomDialog;
  } else if (!roomComputers().length) {
    empty.innerHTML = emptyState('radar', t('empty.noPcTitle'), t('empty.noPcText'), [
      `<button class="btn primary" id="eDisc">${icon('radar')}${esc(t('discover.search'))}</button>`,
      `<button class="btn" id="eAdd">${icon('plus')}${esc(t('computer.add'))}</button>`,
      `<button class="btn" id="eSetup">${icon('package')}${esc(t('setup.package'))}</button>`]);
    $('#eDisc').onclick = () => { setView('discover'); runDiscover(); };
    $('#eAdd').onclick = () => computerDialog();
    $('#eSetup').onclick = () => { location.href = '/api/agent-package'; };
  } else if (!list.length) {
    empty.innerHTML = emptyState('search', t('empty.noMatchTitle'), t('empty.noMatchText'), [`<button class="btn" id="eReset">${esc(t('empty.reset'))}</button>`]);
    $('#eReset').onclick = () => { S.filter = 'all'; S.query = ''; renderSelbar(); renderGrid(); };
  } else empty.innerHTML = '';
  const want = list.map(c => tile(c));
  const cur = [...grid.children];
  if (cur.length !== want.length || cur.some((el, i) => el !== want[i])) {
    for (const el of cur) if (!want.includes(el)) { el.remove(); io.unobserve(el); S.visible.delete(el.dataset.id); }
    want.forEach(el => grid.appendChild(el));
  }
}

function emptyState(ic, title, text, buttons = []) {
  return `<div class="empty"><div><div class="big">${icon(ic)}</div><h2>${esc(title)}</h2><p>${esc(text)}</p><div class="row">${buttons.join('')}</div></div></div>`;
}

function tile(c) {
  let el = S.tiles.get(c.id);
  if (!el) {
    el = document.createElement('div');
    el.className = 'tile';
    el.dataset.id = c.id;
    el.draggable = true;
    el.innerHTML = `<div class="shot"><img alt=""><div class="shimmer"></div><div class="ph"></div><div class="badges"></div><div class="hflag"></div>
      <div class="hover-acts"><button data-a="view">${icon('eye')}</button><button data-a="control">${icon('pointer')}</button><button data-a="lock">${icon('lock')}</button><button data-a="message">${icon('message')}</button></div><div class="pa"></div></div>
      <div class="meta"><div class="row1"><span class="dot"></span><span class="name"></span><span class="user"></span></div><div class="row2"><span class="app"></span><span class="idle"></span></div></div>`;
    el.addEventListener('click', e => onTileClick(e, el.dataset.id));
    el.addEventListener('dblclick', () => { const x = comp(el.dataset.id); if (online(x)) openViewer(x, false); });
    el.addEventListener('contextmenu', e => { e.preventDefault(); tileMenu(e, el.dataset.id); });
    $$('.hover-acts button', el).forEach(b => b.addEventListener('click', e => {
      e.stopPropagation();
      const x = comp(el.dataset.id);
      if (!x) return;
      if (b.dataset.a === 'view') openViewer(x, false);
      if (b.dataset.a === 'control') openViewer(x, true);
      if (b.dataset.a === 'lock') act(x.st && x.st.locked ? 'unlock' : 'lock', { mode: 'screen' }, [x]);
      if (b.dataset.a === 'message') messageDialog([x]);
    }));
    el.addEventListener('dragstart', e => {
      const id = el.dataset.id;
      const ids = S.selected.has(id) ? [...S.selected] : [id];
      e.dataTransfer.setData('text/orbit-ids', JSON.stringify(ids));
      e.dataTransfer.effectAllowed = 'move';
      el.classList.add('dragging');
    });
    el.addEventListener('dragend', () => { el.classList.remove('dragging'); $$('.drop-before').forEach(x => x.classList.remove('drop-before')); });
    el.addEventListener('dragover', e => { if (e.dataTransfer.types.includes('text/orbit-ids')) { e.preventDefault(); el.classList.add('drop-before'); } });
    el.addEventListener('dragleave', () => el.classList.remove('drop-before'));
    el.addEventListener('drop', e => {
      e.preventDefault(); el.classList.remove('drop-before');
      const ids = JSON.parse(e.dataTransfer.getData('text/orbit-ids') || '[]');
      reorder(ids, el.dataset.id);
    });
    S.tiles.set(c.id, el);
    io.observe(el);
    const hb = $$('.hover-acts button', el);
    ['act.view', 'act.control', 'act.lock', 'act.message'].forEach((k, i) => hb[i].title = t(k));
  }
  updateTile(el, c);
  return el;
}

function updateTile(el, c) {
  const st = c.st || {};
  const sig = JSON.stringify([c.name, c.state, c.err, st.user, st.app, st.title, Math.floor((st.idle || 0) / 60), st.locked, st.lockMode, st.demo, st.hand, st.focus, st.viewers, st.controlled, S.selected.has(c.id), I18N.lang, pollAnswer(c)]);
  if (el._sig !== sig) {
    el._sig = sig;
    el.className = `tile ${c.state} ${online(c) ? 'online' : 'off'} ${S.selected.has(c.id) ? 'sel' : ''} ${el._img ? 'has-img' : ''}`;
    $('.name', el).textContent = c.name;
    $('.user', el).textContent = online(c) ? (st.user || '') : '';
    $('.app', el).textContent = online(c) && st.app ? appLabel(st) : (online(c) ? '' : c.host);
    $('.app', el).title = st.title || '';
    const idle = $('.idle', el);
    if (online(c) && st.idle >= 60) { idle.textContent = `${t('tile.idle')} ${fmtDuration(st.idle)}`; idle.classList.toggle('long', st.idle >= 300); }
    else idle.textContent = '';
    const ph = $('.ph', el);
    const phMap = { offline: ['offline', t('state.offline')], connecting: ['refresh', t('state.connecting')], denied: ['key', t('state.denied')], incompatible: ['alert', t('state.incompatible')] };
    const p = phMap[c.state] || ['monitor', ''];
    ph.innerHTML = `${icon(p[0])}<div>${esc(p[1])}</div>${c.err && c.state !== 'connecting' ? `<small style="opacity:.7">${esc(errTr(c.err))}</small>` : ''}${!online(c) && c.macs && c.macs.length ? `<button class="btn sm" data-wake>${icon('zap')}${esc(t('power.on'))}</button>` : ''}`;
    const wb = $('[data-wake]', ph); if (wb) wb.onclick = e => { e.stopPropagation(); wake([c]); };
    let badges = '';
    if (online(c)) {
      if (st.viewers > 0 || st.controlled) badges += `<span class="badge live">${icon('eye')}${esc(st.controlled ? t('badge.control') : t('badge.live'))}</span>`;
      if (st.locked) badges += `<span class="badge lock">${icon(st.lockMode === 'input' ? 'keyboardOff' : 'lock')}</span>`;
      if (st.demo) badges += `<span class="badge demo">${icon('cast')}</span>`;
      if (st.focus) badges += `<span class="badge focus">${icon('target')}</span>`;
    }
    $('.badges', el).innerHTML = badges;
    $('.hflag', el).innerHTML = online(c) && st.hand ? `<div class="handflag" title="${esc(t('filter.hands'))}">${icon('hand')}</div>` : '';
    const pa = pollAnswer(c);
    $('.pa', el).innerHTML = pa !== null ? `<span class="poll-ans">${esc(pa)}</span>` : '';
    const lb = $$('.hover-acts button', el)[2];
    if (lb) { lb.innerHTML = icon(st.locked ? 'unlock' : 'lock'); lb.title = t(st.locked ? 'act.unlock' : 'act.lock'); }
  }
  if (online(c) && c.thumbVer && c.thumbVer !== el._ver) {
    el._ver = c.thumbVer;
    const url = `/api/thumb/${encodeURIComponent(c.id)}?v=${c.thumbVer}`;
    const img = $('img', el);
    const pre = new Image();
    pre.onload = () => { img.src = url; img.classList.add('ok'); el._img = true; el.classList.add('has-img'); };
    pre.src = url;
  }
}

function appLabel(st) {
  const a = (st.app || '').replace(/\.exe$/i, '');
  const known = { msedge: 'Edge', chrome: 'Chrome', firefox: 'Firefox', winword: 'Word', excel: 'Excel', powerpnt: 'PowerPoint', explorer: 'Explorer', notepad: 'Notepad', code: 'VS Code', teams: 'Teams', 'ms-teams': 'Teams', orbitagent: 'Orbit', minecraft: 'Minecraft', javaw: 'Java', discord: 'Discord', spotify: 'Spotify' };
  const name = known[a.toLowerCase()] || a;
  return st.title && st.title !== name ? `${name} · ${st.title}` : name;
}

function pollAnswer(c) {
  const p = S.state.poll;
  if (!p || !p.answers || p.answers[c.id] === undefined) return null;
  const idx = p.answers[c.id];
  const label = p.options[idx] || '?';
  return label.length > 14 ? label.slice(0, 13) + '…' : label;
}

function onTileClick(e, id) {
  if (e.target.closest('button')) return;
  const list = shownComputers().map(c => c.id);
  if (e.shiftKey && S.lastClicked && list.includes(S.lastClicked)) {
    const a = list.indexOf(S.lastClicked), b = list.indexOf(id);
    list.slice(Math.min(a, b), Math.max(a, b) + 1).forEach(x => S.selected.add(x));
  } else if (e.ctrlKey || e.metaKey) {
    S.selected.has(id) ? S.selected.delete(id) : S.selected.add(id);
  } else {
    const only = S.selected.size === 1 && S.selected.has(id);
    S.selected.clear();
    if (!only) S.selected.add(id);
  }
  S.lastClicked = id;
  renderGrid(); renderSelbar(); updateToolbar();
  if (S.spot) sendView();
}

async function reorder(ids, beforeId) {
  if (ids.includes(beforeId)) return;
  const order = roomComputers().map(c => c.id).filter(id => !ids.includes(id));
  const idx = order.indexOf(beforeId);
  order.splice(idx < 0 ? order.length : idx, 0, ...ids);
  await api('computers/move', { body: { ids: order, room: S.room, order: true } });
}

// rubber-band selection
(function rubberBand() {
  let start = null, box = null, base = null;
  document.addEventListener('mousedown', e => {
    const wrap = $('#gridWrap');
    if (e.button !== 0 || !wrap || !wrap.contains(e.target) || e.target.closest('.tile, button, input, .empty')) return;
    start = { x: e.clientX, y: e.clientY };
    base = e.ctrlKey || e.metaKey ? new Set(S.selected) : new Set();
    if (!e.ctrlKey && !e.metaKey && S.selected.size) clearSelection();
  });
  document.addEventListener('mousemove', e => {
    if (!start) return;
    const r = { l: Math.min(start.x, e.clientX), t: Math.min(start.y, e.clientY), r: Math.max(start.x, e.clientX), b: Math.max(start.y, e.clientY) };
    if (!box && (r.r - r.l > 6 || r.b - r.t > 6)) { box = document.createElement('div'); box.className = 'rubber'; document.body.appendChild(box); }
    if (!box) return;
    Object.assign(box.style, { left: r.l + 'px', top: r.t + 'px', width: (r.r - r.l) + 'px', height: (r.b - r.t) + 'px' });
    S.selected = new Set(base);
    for (const el of $$('#grid .tile')) {
      const b = el.getBoundingClientRect();
      if (b.left < r.r && b.right > r.l && b.top < r.b && b.bottom > r.t) S.selected.add(el.dataset.id);
    }
    renderGrid();
  });
  document.addEventListener('mouseup', () => {
    if (box) { box.remove(); box = null; renderSelbar(); updateToolbar(); }
    start = null;
  });
})();

function tileMenu(e, id) {
  const c = comp(id);
  if (!c) return;
  if (!S.selected.has(id)) { S.selected.clear(); S.selected.add(id); renderGrid(); renderSelbar(); updateToolbar(); }
  const st = c.st || {};
  const items = [{ title: c.name + (st.user ? ' · ' + st.user : '') }];
  if (online(c)) {
    items.push(
      { icon: 'eye', label: t('act.view'), hint: t('hint.dblclick'), onClick: () => openViewer(c, false) },
      { icon: 'pointer', label: t('act.control'), onClick: () => openViewer(c, true) },
      { icon: st.locked ? 'unlock' : 'lock', label: t(st.locked ? 'act.unlock' : 'act.lock'), onClick: () => act(st.locked ? 'unlock' : 'lock', { mode: 'screen' }, [c]) },
      { icon: 'message', label: t('act.message'), onClick: () => messageDialog([c]) },
      { icon: 'camera', label: t('act.screenshot'), onClick: () => takeScreenshots([c]) },
      { icon: 'cast', label: t('demo.shareThis'), onClick: () => startDemo(c.id, 'fullscreen') },
    );
    if (st.hand) items.push({ icon: 'hand', label: t('act.lowerHand'), onClick: () => act('hand_lower', {}, [c], 'done.hand_lower') });
  } else {
    items.push({ icon: 'zap', label: t('power.on'), onClick: () => wake([c]) });
  }
  items.push({ sep: true },
    { icon: 'edit', label: t('computer.edit'), onClick: () => computerDialog(c) },
    { icon: 'layers', label: t('computer.move'), onClick: () => showMenu({ x: e.clientX + 8, y: e.clientY + 8 }, S.state.rooms.filter(r => r.id !== c.room).map(r => ({ icon: 'layers', label: r.name, onClick: () => api('computers/move', { body: { ids: [...S.selected], room: r.id } }) }))) },
    { icon: 'trash', label: t('computer.remove'), danger: true, onClick: () => removeComputers([...S.selected]) });
  showMenu({ x: e.clientX, y: e.clientY }, items);
}

// ===================================================================== menus

let openMenuEl = null;
function closeMenu() { if (openMenuEl) { openMenuEl.remove(); openMenuEl = null; } }
function showMenu(anchor, items) {
  closeMenu();
  const m = document.createElement('div');
  m.className = 'menu';
  m.innerHTML = items.map((it, i) => it.sep ? '<hr>' : it.title ? `<div class="menu-title">${esc(it.title)}</div>` :
    `<button data-i="${i}" class="${it.danger ? 'danger' : ''}">${icon(it.icon || 'chevronRight')}<span>${esc(it.label)}</span>${it.hint ? `<small>${esc(it.hint)}</small>` : ''}</button>`).join('');
  document.body.appendChild(m);
  let x, y;
  if (anchor instanceof Element) { const r = anchor.getBoundingClientRect(); x = r.left; y = r.bottom + 6; }
  else { x = anchor.x; y = anchor.y; }
  const mw = m.offsetWidth, mh = m.offsetHeight;
  x = clamp(x, 8, innerWidth - mw - 8); y = clamp(y, 8, innerHeight - mh - 8);
  m.style.left = x + 'px'; m.style.top = y + 'px';
  $$('button[data-i]', m).forEach(b => b.onclick = () => { closeMenu(); const it = items[+b.dataset.i]; it.onClick && it.onClick(); });
  openMenuEl = m;
  setTimeout(() => document.addEventListener('mousedown', function h(ev) { if (!m.contains(ev.target)) { closeMenu(); document.removeEventListener('mousedown', h); } }), 0);
}

function roomMenu(anchor, id) {
  const r = S.state.rooms.find(x => x.id === id);
  if (!r) return;
  showMenu(anchor, [
    { title: r.name },
    { icon: 'edit', label: t('room.rename'), onClick: () => roomDialog(r) },
    { icon: 'plus', label: t('computer.add'), onClick: () => { selectRoom(id); computerDialog(); } },
    { icon: 'radar', label: t('nav.discover'), onClick: () => { selectRoom(id); setView('discover'); } },
    { sep: true },
    { icon: 'trash', label: t('room.delete'), danger: true, onClick: () => deleteRoom(r) },
  ]);
}

// ===================================================================== modal & toasts

function modal({ title, sub, icon: ic = 'info', body = '', buttons = [], wide = false, onOpen, dismiss = true }) {
  const bd = document.createElement('div');
  bd.className = 'backdrop';
  bd.innerHTML = `<div class="modal ${wide ? 'wide' : ''}" role="dialog"><header><div class="mi">${icon(ic)}</div><h3>${esc(title)}${sub ? `<span class="sub">${esc(sub)}</span>` : ''}</h3><button class="icon-btn" data-close>${icon('x')}</button></header>
    <div class="mbody">${body}</div><footer>${buttons.map((b, i) => `<button class="btn ${b.primary ? 'primary' : ''} ${b.danger ? 'danger' : ''} ${b.left ? 'left' : ''} ${b.ghost ? 'ghost' : ''}" data-b="${i}">${b.icon ? icon(b.icon) : ''}${esc(b.label)}</button>`).join('')}</footer></div>`;
  document.body.appendChild(bd);
  const close = () => { bd.remove(); document.removeEventListener('keydown', onKey, true); };
  const onKey = e => {
    if (e.key === 'Escape' && dismiss) { e.stopPropagation(); close(); }
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { const p = buttons.findIndex(b => b.primary); if (p >= 0) { e.preventDefault(); $(`[data-b="${p}"]`, bd).click(); } }
  };
  document.addEventListener('keydown', onKey, true);
  $('[data-close]', bd).onclick = close;
  if (dismiss) bd.addEventListener('mousedown', e => { if (e.target === bd) close(); });
  $$('[data-b]', bd).forEach(b => b.onclick = async () => {
    const def = buttons[+b.dataset.b];
    if (!def.onClick) return close();
    b.disabled = true;
    try { const keep = await def.onClick(bd, close); if (keep !== true) close(); }
    catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); }
    finally { b.disabled = false; }
  });
  const first = $('input:not([type=checkbox]):not([type=radio]), textarea', bd);
  setTimeout(() => first && first.focus(), 50);
  onOpen && onOpen(bd, close);
  return { el: bd, close };
}

function confirmDialog(title, text, okLabel, danger = true) {
  return new Promise(res => {
    modal({ title, icon: danger ? 'alert' : 'info', body: `<p class="hint" style="font-size:14px;margin:0">${esc(text)}</p>`,
      buttons: [{ label: t('btn.cancel'), ghost: true, onClick: () => res(false) }, { label: okLabel, primary: !danger, danger, onClick: () => res(true) }] });
  });
}

function toast(title, kind = 'info', opts = {}) {
  const box = $('#toasts');
  const el = document.createElement('div');
  el.className = 'toast ' + kind;
  const ic = opts.icon || { good: 'check', bad: 'alert', hand: 'hand', info: 'info' }[kind] || 'info';
  el.innerHTML = `<div class="ti">${icon(ic)}</div><div class="tt"><b>${esc(title)}</b>${opts.text ? `<span>${esc(opts.text).replace(/\n/g, '<br>')}</span>` : ''}${opts.actions ? `<div class="tact">${opts.actions.map((a, i) => `<button class="btn sm" data-ta="${i}">${esc(a.label)}</button>`).join('')}</div>` : ''}</div><button class="icon-btn" style="padding:4px">${icon('x')}</button>`;
  box.appendChild(el);
  const kill = () => { el.classList.add('out'); setTimeout(() => el.remove(), 260); };
  el.querySelector('.icon-btn').onclick = kill;
  $$('[data-ta]', el).forEach(b => b.onclick = () => { opts.actions[+b.dataset.ta].onClick(); kill(); });
  setTimeout(kill, opts.timeout || (kind === 'bad' ? 7000 : 3500));
  while (box.children.length > 5) box.firstChild.remove();
}

// ===================================================================== action dialogs

const MSG_TEMPLATES = ['tpl.lookBoard', 'tpl.fiveMin', 'tpl.save', 'tpl.break', 'tpl.welldone', 'tpl.quiet'];

function messageDialog(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  modal({
    title: t('message.title'), sub: tn('sel.to', list.length), icon: 'message',
    body: `<div class="form">
      <div class="presets">${MSG_TEMPLATES.map(k => `<button class="chip" data-tpl="${k}">${esc(t(k))}</button>`).join('')}</div>
      <label class="field">${esc(t('message.text'))}<textarea class="input" id="mText" rows="4" placeholder="${esc(t('message.ph'))}"></textarea></label>
      <label class="field">${esc(t('message.titleOpt'))}<input class="input" id="mTitle" placeholder="${esc(t('message.titlePh'))}"></label></div>`,
    onOpen: bd => $$('[data-tpl]', bd).forEach(b => b.onclick = () => { $('#mText', bd).value = t(b.dataset.tpl); $('#mText', bd).focus(); }),
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('btn.send'), icon: 'send', primary: true, onClick: async bd => {
      const text = $('#mText', bd).value.trim();
      if (!text) { $('#mText', bd).focus(); return true; }
      await act('message', { text, title: $('#mTitle', bd).value.trim() }, list);
    } }],
  });
}

function lockDialog(list) {
  modal({
    title: t('lock.custom'), sub: tn('sel.to', list.length), icon: 'lock',
    body: `<div class="form"><label class="field">${esc(t('lock.titleLbl'))}<input class="input" id="lTitle" value="${esc(S.settings.lockTitle || '')}" placeholder="${esc(t('lock.titlePh'))}"></label>
      <label class="field">${esc(t('lock.textLbl'))}<textarea class="input" id="lText" rows="3" placeholder="${esc(t('lock.textPh'))}">${esc(S.settings.lockText || '')}</textarea></label>
      <label class="check"><input type="checkbox" id="lSave"> ${esc(t('lock.saveDefault'))}</label></div>`,
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('lock.screen'), icon: 'lock', primary: true, onClick: async bd => {
      const title = $('#lTitle', bd).value.trim(), text = $('#lText', bd).value.trim();
      if ($('#lSave', bd).checked) saveSettings({ lockTitle: title, lockText: text });
      await act('lock', { mode: 'screen', title, text }, list);
    } }],
  });
}

const POLL_PRESETS = {
  yesno: ['poll.yes', 'poll.no'], abcd: ['A', 'B', 'C', 'D'], scale: ['1', '2', '3', '4', '5'],
  understand: ['poll.u1', 'poll.u2', 'poll.u3'], truefalse: ['poll.true', 'poll.false'], speed: ['poll.s1', 'poll.s2', 'poll.s3'],
};

function pollDialog(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  const renderOpts = (bd, opts) => {
    $('#pOpts', bd).innerHTML = opts.map((o, i) => `<div style="display:flex;gap:8px"><input class="input" data-o value="${esc(o)}" placeholder="${esc(t('poll.option', { n: i + 1 }))}"><button class="icon-btn" data-del="${i}">${icon('trash')}</button></div>`).join('');
    $$('[data-del]', bd).forEach(b => b.onclick = () => { const v = readOpts(bd); v.splice(+b.dataset.del, 1); renderOpts(bd, v.length ? v : ['']); });
  };
  const readOpts = bd => $$('[data-o]', bd).map(i => i.value);
  modal({
    title: t('poll.title'), sub: tn('sel.to', list.length), icon: 'poll',
    body: `<div class="form"><label class="field">${esc(t('poll.question'))}<input class="input" id="pQ" placeholder="${esc(t('poll.questionPh'))}"></label>
      <div><div class="hint" style="margin-bottom:6px;font-weight:600">${esc(t('poll.presets'))}</div><div class="presets">${Object.keys(POLL_PRESETS).map(k => `<button class="chip" data-pre="${k}">${esc(t('poll.pre.' + k))}</button>`).join('')}</div></div>
      <div class="field" style="gap:8px">${esc(t('poll.options'))}<div id="pOpts" class="form" style="gap:8px"></div><button class="btn sm" id="pAdd" style="align-self:flex-start">${icon('plus')}${esc(t('poll.addOption'))}</button></div></div>`,
    onOpen: bd => {
      renderOpts(bd, [t('poll.yes'), t('poll.no')]);
      $$('[data-pre]', bd).forEach(b => b.onclick = () => renderOpts(bd, POLL_PRESETS[b.dataset.pre].map(k => k.includes('.') ? t(k) : k)));
      $('#pAdd', bd).onclick = () => { const v = readOpts(bd); if (v.length < 12) { v.push(''); renderOpts(bd, v); $$('[data-o]', bd).pop().focus(); } };
    },
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('poll.start'), icon: 'send', primary: true, onClick: async bd => {
      const options = readOpts(bd).map(s => s.trim()).filter(Boolean);
      const question = $('#pQ', bd).value.trim();
      if (!question) { $('#pQ', bd).focus(); return true; }
      if (!options.length) return true;
      const res = await api('poll', { body: { targets: list.map(c => c.id), question, options } });
      report(res.results, t('done.poll', { n: list.length }));
      setView('polls');
    } }],
  });
}

function normalizeURL(u) {
  u = u.trim();
  if (!u) return '';
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(u)) u = 'https://' + u;
  return u;
}

function favList(kind, onPick) {
  const favs = S.settings[kind] || [];
  return favs.length ? `<div><div class="hint" style="margin-bottom:6px;font-weight:600">${esc(t('fav.title'))}</div><div class="presets">${favs.map((f, i) => `<button class="chip" data-fav="${i}">${icon(kind === 'websites' ? 'globe' : 'run')}${esc(f.name)}</button>`).join('')}</div></div>` : '';
}

function websiteDialog(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  modal({
    title: t('website.title'), sub: tn('sel.to', list.length), icon: 'globe',
    body: `<div class="form">${favList('websites')}<label class="field">${esc(t('website.url'))}<input class="input" id="wUrl" placeholder="https://"></label>
      <label class="check"><input type="checkbox" id="wFav"> ${esc(t('fav.save'))}</label></div>`,
    onOpen: bd => $$('[data-fav]', bd).forEach(b => b.onclick = () => { $('#wUrl', bd).value = S.settings.websites[+b.dataset.fav].value; }),
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('website.open'), icon: 'globe', primary: true, onClick: async bd => {
      const url = normalizeURL($('#wUrl', bd).value);
      if (!url) return true;
      if ($('#wFav', bd).checked) { let name = url; try { name = new URL(url).hostname.replace(/^www\./, ''); } catch {} saveSettings({ websites: [...(S.settings.websites || []), { name, value: url }] }); }
      await act('open_url', { url }, list);
    } }],
  });
}

function runDialog(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  modal({
    title: t('run.title'), sub: tn('sel.to', list.length), icon: 'run',
    body: `<div class="form">${favList('programs')}<label class="field">${esc(t('run.program'))}<input class="input mono" id="rProg" placeholder="notepad.exe"></label>
      <label class="field">${esc(t('run.args'))}<input class="input mono" id="rArgs" placeholder=""></label>
      <div class="note">${icon('info')}<div>${esc(t('run.hint'))}</div></div>
      <label class="check"><input type="checkbox" id="rFav"> ${esc(t('fav.save'))}</label></div>`,
    onOpen: bd => $$('[data-fav]', bd).forEach(b => b.onclick = () => { const f = S.settings.programs[+b.dataset.fav]; $('#rProg', bd).value = f.value; $('#rArgs', bd).value = f.args || ''; }),
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('run.start'), icon: 'play', primary: true, onClick: async bd => {
      const program = $('#rProg', bd).value.trim(), args = $('#rArgs', bd).value.trim();
      if (!program) return true;
      if ($('#rFav', bd).checked) saveSettings({ programs: [...(S.settings.programs || []), { name: program.split(/[\\/]/).pop().replace(/\.exe$/i, ''), value: program, args }] });
      await act('run', { program, args }, list);
    } }],
  });
}

function fileDialog(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  let file = null;
  modal({
    title: t('file.title'), sub: tn('sel.to', list.length), icon: 'fileup',
    body: `<div class="form"><div class="drop" id="fDrop">${icon('fileup')}<div id="fName">${esc(t('file.drop'))}</div><input type="file" id="fIn" hidden></div>
      <label class="check"><input type="checkbox" id="fOpen"> ${esc(t('file.open'))}</label>
      <div class="hint">${esc(t('file.where'))}</div>
      <div class="progress" id="fProg" hidden><i></i></div></div>`,
    onOpen: bd => {
      const drop = $('#fDrop', bd), inp = $('#fIn', bd);
      const pick = f => { file = f; $('#fName', bd).innerHTML = `<b>${esc(f.name)}</b> · ${fmtBytes(f.size)}`; };
      drop.onclick = () => inp.click();
      inp.onchange = () => inp.files[0] && pick(inp.files[0]);
      drop.ondragover = e => { e.preventDefault(); drop.classList.add('over'); };
      drop.ondragleave = () => drop.classList.remove('over');
      drop.ondrop = e => { e.preventDefault(); drop.classList.remove('over'); if (e.dataTransfer.files[0]) pick(e.dataTransfer.files[0]); };
    },
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('btn.send'), icon: 'send', primary: true, onClick: async bd => {
      if (!file) { $('#fDrop', bd).click(); return true; }
      const fd = new FormData();
      fd.append('targets', JSON.stringify(list.map(c => c.id)));
      fd.append('open', $('#fOpen', bd).checked ? '1' : '0');
      fd.append('file', file, file.name);
      const prog = $('#fProg', bd); prog.hidden = false;
      const res = await new Promise((resolve, reject) => {
        const x = new XMLHttpRequest();
        x.open('POST', '/api/file');
        x.upload.onprogress = e => { if (e.lengthComputable) prog.firstChild.style.width = (e.loaded / e.total * 100) + '%'; };
        x.onload = () => { try { const j = JSON.parse(x.responseText); x.status < 300 ? resolve(j) : reject(new Error(j.error)); } catch (e) { reject(e); } };
        x.onerror = () => reject(new Error('upload failed'));
        x.send(fd);
      });
      report(res.results, tn('done.file', list.length, { name: file.name }));
    } }],
  });
}

async function takeScreenshots(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  toast(tn('shots.taking', list.length), 'info', { icon: 'camera' });
  try {
    const res = await api('screenshot', { body: { targets: list.map(c => c.id) } });
    const bad = Object.values(res.results).filter(Boolean).length;
    toast(tn('shots.saved', list.length - bad), bad ? 'bad' : 'good', { text: res.dir, actions: [{ label: t('shots.show'), onClick: () => setView('shots') }] });
    S.shots = null;
  } catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); }
}

const FOCUS_PRESETS = {
  browsers: ['chrome.exe', 'msedge.exe', 'firefox.exe', 'opera.exe', 'brave.exe', 'vivaldi.exe', 'iexplore.exe'],
  games: ['steam.exe', 'epicgameslauncher.exe', 'minecraft.exe', 'minecraftlauncher.exe', 'javaw.exe', 'robloxplayerbeta.exe', 'leagueclient.exe', 'riotclientservices.exe', 'battle.net.exe', 'fortniteclient-win64-shipping.exe'],
  chat: ['discord.exe', 'telegram.exe', 'whatsapp.exe', 'signal.exe', 'skype.exe', 'messenger.exe'],
  media: ['spotify.exe', 'vlc.exe', 'netflix.exe'],
};

function focusDialog(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  const cur = S.settings.focusList && S.settings.focusList.length ? S.settings.focusList : [...FOCUS_PRESETS.games, ...FOCUS_PRESETS.chat];
  const allOn = list.every(c => c.st && c.st.focus);
  modal({
    title: t('focus.title'), sub: tn('sel.to', list.length), icon: 'target', wide: true,
    body: `<div class="form"><p class="hint" style="margin:0">${esc(t('focus.text'))}</p>
      <div class="presets">${Object.keys(FOCUS_PRESETS).map(k => `<button class="chip" data-fp="${k}">${icon('plus')}${esc(t('focus.pre.' + k))}</button>`).join('')}<button class="chip" id="fClear">${icon('trash')}${esc(t('focus.clear'))}</button></div>
      <label class="field">${esc(t('focus.list'))}<textarea class="input mono" id="fList" rows="8">${esc(cur.join('\n'))}</textarea></label>
      <div class="note warn">${icon('alert')}<div>${esc(t('focus.warn'))}</div></div></div>`,
    onOpen: bd => {
      $$('[data-fp]', bd).forEach(b => b.onclick = () => { const ta = $('#fList', bd); const set = new Set(ta.value.split(/\s+/).filter(Boolean)); FOCUS_PRESETS[b.dataset.fp].forEach(x => set.add(x)); ta.value = [...set].join('\n'); });
      $('#fClear', bd).onclick = () => { $('#fList', bd).value = ''; };
    },
    buttons: [
      ...(allOn || list.some(c => c.st && c.st.focus) ? [{ label: t('focus.off'), left: true, danger: true, onClick: () => act('focus', { enabled: false }, list, 'done.focusOff') }] : []),
      { label: t('btn.cancel'), ghost: true },
      { label: t('focus.on'), icon: 'target', primary: true, onClick: async bd => {
        const items = $('#fList', bd).value.split(/[\s,;]+/).map(s => s.trim()).filter(Boolean);
        if (!items.length) return true;
        saveSettings({ focusList: items });
        await act('focus', { enabled: true, list: items }, list, 'done.focusOn');
      } }],
  });
}

async function powerDialog(mode, list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  const doIt = async (delay, text) => act('power', { mode, delay, text }, list, 'done.' + mode);
  if (!S.settings.confirmPower) return doIt(0, '');
  modal({
    title: t('power.' + mode), sub: tn('sel.to', list.length), icon: mode === 'logoff' ? 'logout' : mode === 'reboot' ? 'restart' : 'power',
    body: `<div class="form"><p class="hint" style="font-size:14px;margin:0">${esc(tn('power.confirm.' + mode, list.length))}</p>
      ${mode !== 'logoff' ? `<label class="check"><input type="checkbox" id="pwWarn" checked> ${esc(t('power.warn60'))}</label>` : ''}
      <div class="note warn">${icon('alert')}<div>${esc(t('power.unsaved'))}</div></div></div>`,
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('power.' + mode), danger: true, icon: 'power', onClick: async bd => {
      const w = $('#pwWarn', bd);
      const warn = w && w.checked;
      await doIt(warn ? 60 : 0, warn ? t('power.warnText.' + mode) : '');
    } }],
  });
}

async function wake(list) {
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  try {
    const res = await api('wake', { body: { targets: list.map(c => c.id) } });
    report(res.results, tn('done.wake', list.length));
  } catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); }
}

async function startDemo(source, mode, list) {
  list = list || targets().filter(c => c.id !== source);
  if (!list.length) return toast(t('toast.noTargets'), 'bad');
  try {
    await api('demo/start', { body: { source, mode, targets: list.map(c => c.id) } });
    toast(source === 'teacher' ? tn('demo.started', list.length) : t('demo.startedStudent', { name: (comp(source) || {}).name }), 'good', { icon: 'cast' });
  } catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); }
}

async function stopDemo() {
  try { await api('demo/stop', { body: {} }); toast(t('demo.stopped'), 'good'); } catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); }
}

function shareStudentDialog() {
  const cands = roomComputers().filter(online);
  if (!cands.length) return toast(t('toast.noTargets'), 'bad');
  const pre = S.selected.size === 1 ? [...S.selected][0] : '';
  modal({
    title: t('demo.student'), icon: 'users',
    body: `<div class="form"><p class="hint" style="margin:0">${esc(t('demo.studentText'))}</p>
      <label class="field">${esc(t('demo.whichStudent'))}<select class="input" id="dsSrc">${cands.map(c => `<option value="${esc(c.id)}" ${c.id === pre ? 'selected' : ''}>${esc(c.name)}${c.st && c.st.user ? ' · ' + esc(c.st.user) : ''}</option>`).join('')}</select></label>
      <label class="field">${esc(t('demo.mode'))}<select class="input" id="dsMode"><option value="fullscreen">${esc(t('demo.fullscreen'))}</option><option value="window">${esc(t('demo.window'))}</option></select></label></div>`,
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('demo.startBtn'), icon: 'cast', primary: true, onClick: bd => {
      const src = $('#dsSrc', bd).value;
      return startDemo(src, $('#dsMode', bd).value, roomComputers().filter(c => online(c) && c.id !== src));
    } }],
  });
}

// ===================================================================== rooms & computers

function addRoomDialog() { roomDialog(null); }

function roomDialog(r) {
  modal({
    title: r ? t('room.rename') : t('room.add'), icon: 'layers',
    body: `<label class="field">${esc(t('room.name'))}<input class="input" id="rName" value="${esc(r ? r.name : '')}" placeholder="${esc(t('room.namePh'))}"></label>`,
    onOpen: bd => $('#rName', bd).addEventListener('keydown', e => { if (e.key === 'Enter') $('[data-b="1"]', bd).click(); }),
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: r ? t('btn.save') : t('room.create'), primary: true, onClick: async bd => {
      const name = $('#rName', bd).value.trim();
      if (!name) return true;
      if (r) await api('rooms/' + r.id, { method: 'PUT', body: { name } });
      else { const nr = await api('rooms', { body: { name } }); S.room = nr.id; store.set('room', nr.id); setView('computers'); }
    } }],
  });
}

async function deleteRoom(r) {
  const n = S.state.computers.filter(c => c.room === r.id).length;
  if (!(await confirmDialog(t('room.delete'), tn('room.deleteConfirm', n, { name: r.name }), t('btn.delete')))) return;
  await api('rooms/' + r.id, { method: 'DELETE' });
}

function computerDialog(c) {
  const rooms = S.state.rooms.map(r => `<option value="${esc(r.id)}" ${(c ? c.room : S.room) === r.id ? 'selected' : ''}>${esc(r.name)}</option>`).join('');
  modal({
    title: c ? t('computer.edit') : t('computer.add'), icon: 'monitor',
    body: `<div class="form">
      <label class="field">${esc(t('computer.host'))}<input class="input mono" id="cHost" value="${esc(c ? c.host : '')}" placeholder="PC-01 / 192.168.1.21"></label>
      <div class="field-row"><label class="field">${esc(t('computer.name'))}<input class="input" id="cName" value="${esc(c ? c.name : '')}" placeholder="${esc(t('computer.namePh'))}"></label>
      <label class="field">${esc(t('computer.room'))}<select class="input" id="cRoom">${rooms}</select></label></div>
      <div class="field-row"><label class="field">${esc(t('computer.port'))}<input class="input mono" id="cPort" type="number" value="${c ? c.port : S.meta.agentPort || 21400}"></label>
      <label class="field">${esc(t('computer.mac'))}<input class="input mono" id="cMac" value="${esc(c && c.macs ? c.macs[0] || '' : '')}" placeholder="${esc(t('computer.macPh'))}"></label></div>
      <div class="hint">${esc(t('computer.hint'))}</div></div>`,
    buttons: [
      ...(c ? [{ label: t('computer.remove'), danger: true, left: true, icon: 'trash', onClick: () => removeComputers([c.id]) }] : []),
      { label: t('btn.cancel'), ghost: true },
      { label: c ? t('btn.save') : t('computer.addBtn'), primary: true, onClick: async bd => {
        const body = { host: $('#cHost', bd).value.trim(), name: $('#cName', bd).value.trim(), room: $('#cRoom', bd).value, port: +$('#cPort', bd).value || 0, mac: $('#cMac', bd).value.trim() };
        if (!body.host) { $('#cHost', bd).focus(); return true; }
        if (c) await api('computers/' + c.id, { method: 'PUT', body });
        else { await api('computers', { body }); toast(t('computer.added', { name: body.name || body.host }), 'good'); }
      } }],
  });
}

async function removeComputers(ids) {
  if (!ids.length) return;
  const name = ids.length === 1 ? (comp(ids[0]) || {}).name : '';
  if (!(await confirmDialog(t('computer.remove'), ids.length === 1 ? t('computer.removeOne', { name }) : tn('computer.removeMany', ids.length), t('btn.delete')))) return;
  for (const id of ids) await api('computers/' + id, { method: 'DELETE' });
  ids.forEach(id => S.selected.delete(id));
}

// ===================================================================== remote viewer

function openViewerById(id, control) { const c = comp(id); if (online(c)) openViewer(c, control); }

function openViewer(c, control) {
  if (!online(c)) return toast(t('state.offline'), 'bad');
  if (control && c.st && c.st.allowControl === false) { toast(t('viewer.ctlDisabled'), 'bad'); control = false; }
  const mons = (c.st && c.st.monitors) || [];
  const v = document.createElement('div');
  v.className = 'viewer';
  v.innerHTML = `<div class="vbar">
      <div class="vt"><b>${esc(c.name)}</b><span>${esc((c.st && c.st.user) || '')}${c.st && c.st.os ? ' · ' + esc(c.st.os) : ''}</span></div>
      <div class="seg" id="vMode"><button data-m="view">${icon('eye')}${esc(t('act.view'))}</button><button data-m="control">${icon('pointer')}${esc(t('act.control'))}</button></div>
      ${mons.length > 1 ? `<select id="vMon" title="${esc(t('viewer.monitor'))}"><option value="-1">${esc(t('viewer.allMonitors'))}</option>${mons.map((m, i) => `<option value="${i}">${esc(t('viewer.monitorN', { n: i + 1 }))} ${m.w}×${m.h}${m.primary ? ' ★' : ''}</option>`).join('')}</select>` : ''}
      <select id="vQ" title="${esc(t('viewer.quality'))}"><option value="">${esc(t('viewer.qAuto'))}</option><option value="40">${esc(t('viewer.qLow'))}</option><option value="65">${esc(t('viewer.qMed'))}</option><option value="85">${esc(t('viewer.qHigh'))}</option></select>
      <button class="icon-btn" id="vFit" title="${esc(t('viewer.fit'))}">${icon('fit')}</button>
      <div class="vsep"></div>
      <span id="vKeys" style="display:flex;gap:2px">
        <button class="icon-btn" data-combo="win" title="${esc(t('viewer.win'))}"><b style="font-size:12px">Win</b></button>
        <button class="icon-btn" data-combo="alttab" title="Alt+Tab"><b style="font-size:12px">Alt+Tab</b></button>
        <button class="icon-btn" data-combo="ctrlshiftesc" title="${esc(t('viewer.taskmgr'))}"><b style="font-size:12px">Ctrl+Shift+Esc</b></button>
        <button class="icon-btn" id="vType" title="${esc(t('viewer.type'))}">${icon('text')}</button>
        <div class="vsep"></div></span>
      <button class="icon-btn" id="vShot" title="${esc(t('act.screenshot'))}">${icon('camera')}</button>
      <button class="icon-btn" id="vRec" title="${esc(t('viewer.record'))}">${icon('record')}</button>
      <button class="icon-btn" id="vFull" title="${esc(t('viewer.fullscreen'))}">${icon('maximize')}</button>
      <span class="stats" id="vStats"></span>
      <button class="icon-btn" id="vClose" title="${esc(t('btn.close'))}">${icon('x')}</button>
    </div>
    <div class="vstage" id="vStage"><canvas id="vCanvas" tabindex="0"></canvas><div class="vmsg" id="vMsg"><div class="spinner"></div><div>${esc(t('viewer.connecting'))}</div></div></div>`;
  document.body.appendChild(v);
  const canvas = $('#vCanvas', v), stage = $('#vStage', v), msg = $('#vMsg', v);
  let mode = control ? 'control' : 'view', fit = true, mon = -1, q = '', stream = null, closed = false, rec = null, recChunks = [];
  let lastBytes = 0, lastFrames = 0;
  const pressed = new Set();
  let queue = [];

  const layout = () => {
    if (!canvas.width) return;
    if (fit) {
      const s = Math.min(stage.clientWidth / canvas.width, stage.clientHeight / canvas.height);
      canvas.style.width = Math.floor(canvas.width * s) + 'px'; canvas.style.height = Math.floor(canvas.height * s) + 'px';
    } else { canvas.style.width = canvas.width + 'px'; canvas.style.height = canvas.height + 'px'; }
    stage.classList.toggle('one', !fit);
  };
  const setMode = m => {
    mode = m;
    $$('#vMode button', v).forEach(b => b.classList.toggle('on', b.dataset.m === m));
    stage.classList.toggle('control', m === 'control');
    $('#vKeys', v).style.display = m === 'control' ? 'flex' : 'none';
    if (m === 'control') canvas.focus();
    releaseAll();
  };
  const connect = () => {
    if (closed) return;
    if (stream) stream.stop();
    msg.style.display = 'grid';
    msg.innerHTML = `<div class="spinner"></div><div>${esc(t('viewer.connecting'))}</div>`;
    const params = new URLSearchParams({ purpose: mode, mon: String(mon) });
    if (q) params.set('q', q);
    stream = new OrbitStream(canvas, () => { if (msg.style.display !== 'none') { msg.style.display = 'none'; layout(); } else if (canvas._w !== canvas.width) layout(); canvas._w = canvas.width; });
    const me = stream;
    stream.start(`/api/stream/${encodeURIComponent(c.id)}?${params}`).then(() => {
      if (!closed && me === stream) { msg.style.display = 'grid'; msg.innerHTML = `<div>${esc(t('viewer.lost'))}</div>`; setTimeout(connect, 1500); }
    }).catch(err => {
      if (closed || me !== stream || err.name === 'AbortError') return;
      msg.style.display = 'grid';
      msg.innerHTML = `<div>${icon('offline')}</div><div>${esc(t('viewer.error'))}<br><small>${esc(err.message)}</small></div><div><button class="btn sm" id="vRetry">${esc(t('viewer.retry'))}</button></div>`;
      $('#vRetry', v).onclick = connect;
      setTimeout(() => { if (!closed && me === stream) connect(); }, 4000);
    });
  };
  const stats = setInterval(() => {
    if (!stream) return;
    const fps = stream.frames - lastFrames, kb = (stream.bytes - lastBytes) * 8 / 1000;
    lastFrames = stream.frames; lastBytes = stream.bytes;
    $('#vStats', v).textContent = `${canvas.width}×${canvas.height} · ${fps} fps · ${kb > 1000 ? (kb / 1000).toFixed(1) + ' Mbit/s' : Math.round(kb) + ' kbit/s'}${rec ? ' · ● REC' : ''}`;
  }, 1000);

  // input
  const norm = e => { const r = canvas.getBoundingClientRect(); return { x: clamp((e.clientX - r.left) / r.width, 0, 1), y: clamp((e.clientY - r.top) / r.height, 0, 1) }; };
  const push = ev => { if (mode !== 'control') return; const last = queue[queue.length - 1]; if (ev.t === 'mm' && last && last.t === 'mm') queue[queue.length - 1] = ev; else queue.push(ev); };
  const flush = setInterval(() => {
    if (!queue.length) return;
    const batch = queue; queue = [];
    fetch(`/api/input/${encodeURIComponent(c.id)}?mon=${mon}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(batch) })
      .then(r => r.json()).then(j => { if (j && j.ok === false && j.error) toast(t('viewer.inputFailed'), 'bad', { text: j.error }); }).catch(() => {});
  }, 16);
  const releaseAll = () => { pressed.forEach(code => queue.push({ t: 'ku', c: code })); pressed.clear(); };
  canvas.addEventListener('mousemove', e => push({ t: 'mm', ...norm(e) }));
  canvas.addEventListener('mousedown', e => { if (mode !== 'control') return; e.preventDefault(); canvas.focus(); push({ t: 'md', b: e.button, ...norm(e) }); });
  canvas.addEventListener('mouseup', e => { if (mode !== 'control') return; e.preventDefault(); push({ t: 'mu', b: e.button, ...norm(e) }); });
  canvas.addEventListener('contextmenu', e => { if (mode === 'control') e.preventDefault(); });
  canvas.addEventListener('wheel', e => {
    if (mode !== 'control') return;
    e.preventDefault();
    const k = e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? 800 : 1;
    if (e.deltaY) push({ t: 'wh', d: Math.round(-e.deltaY * k * 1.2), ...norm(e) });
    if (e.deltaX) push({ t: 'hw', d: Math.round(e.deltaX * k * 1.2), ...norm(e) });
  }, { passive: false });
  const onKey = e => {
    if (mode !== 'control' || e.target.closest('.backdrop') || e.target.tagName === 'SELECT') return;
    e.preventDefault(); e.stopPropagation();
    if (e.type === 'keydown') { if (!e.repeat || pressed.has(e.code)) { pressed.add(e.code); push({ t: 'kd', c: e.code }); } }
    else { pressed.delete(e.code); push({ t: 'ku', c: e.code }); }
  };
  const onViewKey = e => { if (mode === 'view' && e.key === 'Escape' && !e.target.closest('.backdrop')) close(); };
  window.addEventListener('keydown', onKey, true);
  window.addEventListener('keyup', onKey, true);
  window.addEventListener('keydown', onViewKey);
  window.addEventListener('blur', releaseAll);
  const onResize = () => layout();
  window.addEventListener('resize', onResize);

  $$('#vMode button', v).forEach(b => b.onclick = () => {
    if (b.dataset.m === 'control' && c.st && c.st.allowControl === false) return toast(t('viewer.ctlDisabled'), 'bad');
    const changed = b.dataset.m !== mode; setMode(b.dataset.m); if (changed) connect();
  });
  const vm = $('#vMon', v); if (vm) vm.onchange = () => { mon = +vm.value; connect(); };
  $('#vQ', v).onchange = e => { q = e.target.value; connect(); };
  $('#vFit', v).onclick = () => { fit = !fit; $('#vFit', v).classList.toggle('on', !fit); layout(); };
  $$('[data-combo]', v).forEach(b => b.onclick = () => { push({ t: 'combo', s: b.dataset.combo }); canvas.focus(); });
  $('#vType', v).onclick = () => modal({
    title: t('viewer.type'), icon: 'text', body: `<label class="field">${esc(t('viewer.typeText'))}<textarea class="input" id="tText" rows="3"></textarea></label>`,
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('btn.send'), primary: true, onClick: bd => { const s = $('#tText', bd).value; if (s) push({ t: 'tx', s }); } }],
  });
  $('#vShot', v).onclick = () => takeScreenshots([c]);
  $('#vFull', v).onclick = async () => {
    if (document.fullscreenElement) return document.exitFullscreen();
    await v.requestFullscreen().catch(() => {});
    try { if (navigator.keyboard && navigator.keyboard.lock) await navigator.keyboard.lock(); } catch {}
  };
  $('#vRec', v).onclick = () => {
    if (rec) { rec.stop(); return; }
    try {
      const mime = ['video/webm;codecs=vp9', 'video/webm;codecs=vp8', 'video/webm'].find(m => window.MediaRecorder && MediaRecorder.isTypeSupported(m));
      rec = new MediaRecorder(canvas.captureStream(20), mime ? { mimeType: mime, videoBitsPerSecond: 4e6 } : undefined);
      recChunks = [];
      rec.ondataavailable = e => e.data.size && recChunks.push(e.data);
      rec.onstop = () => {
        const blob = new Blob(recChunks, { type: 'video/webm' });
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = `orbit-${c.name}-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.webm`;
        a.click();
        setTimeout(() => URL.revokeObjectURL(a.href), 10000);
        rec = null; $('#vRec', v).classList.remove('rec');
        toast(t('viewer.recSaved'), 'good');
      };
      rec.start(1000);
      $('#vRec', v).classList.add('rec');
      toast(t('viewer.recStarted'), 'info', { icon: 'record' });
    } catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); rec = null; }
  };
  const close = () => {
    closed = true;
    releaseAll();
    if (queue.length) fetch(`/api/input/${encodeURIComponent(c.id)}?mon=${mon}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(queue) }).catch(() => {});
    if (rec) rec.stop();
    stream && stream.stop();
    clearInterval(stats); clearInterval(flush);
    window.removeEventListener('keydown', onKey, true);
    window.removeEventListener('keyup', onKey, true);
    window.removeEventListener('keydown', onViewKey);
    window.removeEventListener('blur', releaseAll);
    window.removeEventListener('resize', onResize);
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    v.remove();
  };
  $('#vClose', v).onclick = close;
  setMode(mode);
  connect();
}

// ===================================================================== discover

function renderDiscover() {
  const p = $('#discoverPage');
  const d = S.discover;
  const rooms = S.state.rooms.map(r => `<option value="${esc(r.id)}" ${r.id === S.room ? 'selected' : ''}>${esc(r.name)}</option>`).join('');
  let results = '';
  if (d.busy) results = `<div class="empty" style="padding:30px"><div class="spinner"></div><p style="margin-top:14px">${esc(t(d.scan ? 'discover.scanning' : 'discover.searching'))}</p></div>`;
  else if (d.results && !d.results.length) results = `<div class="note warn" style="margin-top:14px">${icon('alert')}<div>${esc(t('discover.none'))}</div></div>`;
  else if (d.results) {
    results = `<table class="tbl" style="margin-top:14px"><thead><tr><th style="width:34px"><input type="checkbox" id="dAll"></th><th>${esc(t('discover.colName'))}</th><th>${esc(t('discover.colIp'))}</th><th>${esc(t('discover.colUser'))}</th><th>${esc(t('discover.colKey'))}</th><th>${esc(t('discover.colStatus'))}</th></tr></thead><tbody>
      ${d.results.map((f, i) => {
        const key = f.trusted === true ? `<span class="tag good">${icon('check')}${esc(t('discover.trusted'))}</span>` : f.trusted === false ? `<span class="tag bad">${icon('x')}${esc(t('discover.untrusted'))}</span>` : `<span class="tag">?</span>`;
        const known = f.known ? `<span class="tag">${esc(t('discover.inRoom', { room: roomName((comp(f.known) || {}).room) }))}</span>` : `<span class="tag warn">${esc(t('discover.new'))}</span>`;
        return `<tr><td><input type="checkbox" data-pick="${i}" ${d.picked.has(i) ? 'checked' : ''} ${f.known ? 'disabled' : ''}></td><td><b>${esc(f.name)}</b><div class="hint">${esc(f.os || '')} ${f.version ? 'v' + esc(f.version) : ''}</div></td><td class="mono">${esc(f.ip)}${f.port !== 21400 ? ':' + f.port : ''}</td><td>${esc(f.user || '–')}</td><td>${key}</td><td>${known}</td></tr>`;
      }).join('')}</tbody></table>
      <div style="display:flex;gap:10px;align-items:center;margin-top:14px;flex-wrap:wrap"><span class="hint">${esc(t('discover.addTo'))}</span><select class="input" id="dRoom" style="width:220px">${rooms}</select><button class="btn primary" id="dAdd">${icon('plus')}${esc(t('discover.addSelected'))}</button></div>`;
  }
  p.innerHTML = `
    <div class="card"><h2>${icon('radar')}${esc(t('discover.title'))}</h2><p class="hint">${esc(t('discover.text'))}</p>
      <div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap"><button class="btn primary" id="dGo" ${d.busy ? 'disabled' : ''}>${icon('radar')}${esc(t('discover.search'))}</button>
      <label class="check"><input type="checkbox" id="dScan" ${d.scan ? 'checked' : ''}> ${esc(t('discover.deep'))}</label></div>${results}</div>
    <div class="cols" style="margin-top:16px">
      <div class="card"><h2>${icon('plus')}${esc(t('computer.add'))}</h2><p class="hint">${esc(t('discover.manualText'))}</p>
        <div class="form"><input class="input mono" id="qHost" placeholder="PC-01 / 192.168.1.21"><input class="input" id="qName" placeholder="${esc(t('computer.namePh'))}">
        <button class="btn" id="qAdd" style="align-self:flex-start">${icon('plus')}${esc(t('computer.addBtn'))}</button></div></div>
      <div class="card"><h2>${icon('package')}${esc(t('setup.title'))}</h2><p class="hint">${esc(t('setup.text'))}</p>
        <ol class="steps"><li>${esc(t('setup.s1'))}</li><li>${esc(t('setup.s2'))}</li><li>${esc(t('setup.s3'))}</li></ol>
        <div style="display:flex;gap:8px;flex-wrap:wrap;margin-top:14px"><a class="btn primary" href="/api/agent-package">${icon('download')}${esc(t('setup.package'))}</a><a class="btn" href="/api/agent-config">${icon('key')}orbit-agent.json</a></div>
        ${S.meta.agentAvailable ? '' : `<div class="note warn" style="margin-top:12px">${icon('alert')}<div>${esc(t('setup.noAgent'))}</div></div>`}
        <div class="hint" style="margin-top:12px">${esc(t('setup.silent'))}</div><div class="keybox" style="margin-top:6px"><code>OrbitAgent.exe --install --silent --key ${esc(S.meta.publicKey || '')}</code><button class="icon-btn" id="copySilent">${icon('copy')}</button></div>
      </div></div>`;
  $('#dGo', p).onclick = () => runDiscover();
  $('#dScan', p).onchange = e => { d.scan = e.target.checked; };
  $$('[data-pick]', p).forEach(cb => cb.onchange = () => { cb.checked ? d.picked.add(+cb.dataset.pick) : d.picked.delete(+cb.dataset.pick); });
  const all = $('#dAll', p);
  if (all) all.onchange = () => { d.results.forEach((f, i) => { if (!f.known) all.checked ? d.picked.add(i) : d.picked.delete(i); }); renderDiscover(); };
  const add = $('#dAdd', p);
  if (add) add.onclick = async () => {
    const items = [...d.picked].map(i => d.results[i]).filter(Boolean).map(f => ({ name: f.name, host: f.ip, port: f.port, macs: f.macs }));
    if (!items.length) return toast(t('discover.pickFirst'), 'bad');
    const room = $('#dRoom', p).value;
    const res = await api('computers/bulk', { body: { room, items } });
    toast(tn('discover.added', res.added, { room: roomName(room) }), 'good', { actions: [{ label: t('discover.goRoom'), onClick: () => selectRoom(room) || setView('computers') }] });
    d.picked.clear();
    runDiscover(true);
  };
  $('#qAdd', p).onclick = async () => {
    const host = $('#qHost', p).value.trim();
    if (!host) return $('#qHost', p).focus();
    await api('computers', { body: { host, name: $('#qName', p).value.trim(), room: S.room } });
    toast(t('computer.added', { name: $('#qName', p).value.trim() || host }), 'good');
    $('#qHost', p).value = ''; $('#qName', p).value = '';
  };
  $('#copySilent', p).onclick = () => copyText(`OrbitAgent.exe --install --silent --key ${S.meta.publicKey}`);
}

async function runDiscover(quiet) {
  const d = S.discover;
  d.busy = true; d.picked.clear();
  if (S.view === 'discover') renderDiscover();
  try {
    const res = await api('discover', { body: { scan: d.scan } });
    d.results = res.found;
    d.results.forEach((f, i) => { if (!f.known && f.trusted !== false) d.picked.add(i); });
    if (!quiet) toast(tn('discover.found', d.results.length), d.results.length ? 'good' : 'info');
  } catch (e) { toast(t('toast.error'), 'bad', { text: e.message }); }
  d.busy = false;
  if (S.view === 'discover') renderDiscover();
}

function copyText(s) {
  (navigator.clipboard ? navigator.clipboard.writeText(s) : Promise.reject()).then(() => toast(t('toast.copied'), 'good')).catch(() => {
    const ta = document.createElement('textarea'); ta.value = s; document.body.appendChild(ta); ta.select(); document.execCommand('copy'); ta.remove(); toast(t('toast.copied'), 'good');
  });
}

// ===================================================================== inbox

function renderInbox() {
  const p = $('#inboxPage');
  const hands = S.state.computers.filter(c => c.st && c.st.hand).sort((a, b) => (a.st.handAt || 0) - (b.st.handAt || 0));
  const msgs = [...S.state.inbox].reverse();
  let html = '';
  if (hands.length) {
    html += `<div class="card" style="margin-bottom:16px"><h2>${icon('hand')}${esc(tn('inbox.hands', hands.length))}</h2><p class="hint">${esc(t('inbox.handsText'))}</p>
      <div class="answer-list">${hands.map(c => `<div><b>${esc(c.name)}${c.st.user ? ' · ' + esc(c.st.user) : ''}</b><span style="display:flex;gap:4px"><button class="btn sm" data-hview="${esc(c.id)}">${icon('eye')}</button><button class="btn sm" data-hlow="${esc(c.id)}">${esc(t('act.lowerHand'))}</button></span></div>`).join('')}</div>
      <button class="btn sm" id="lowerAll" style="margin-top:12px">${icon('hand')}${esc(t('more.lowerHands'))}</button></div>`;
  }
  if (!msgs.length) html += emptyState('inbox', t('inbox.emptyTitle'), t('inbox.emptyText'));
  else {
    html += `<div style="display:flex;gap:8px;justify-content:flex-end;margin-bottom:12px"><button class="btn sm" id="inClear">${icon('trash')}${esc(t('inbox.clear'))}</button></div>`;
    html += msgs.map(m => `<div class="msg ${m.read ? '' : 'unread'}"><div class="avatar">${esc(initials(m.user || m.name))}</div><div class="body">
      <div class="head"><b>${esc(m.user || m.name)}</b><span>${esc(m.name)} · ${esc(fmtTime(m.at))}</span></div><p>${esc(m.text)}</p>
      <div style="display:flex;gap:6px"><button class="btn sm" data-reply="${esc(m.computer)}">${icon('message')}${esc(t('inbox.reply'))}</button><button class="btn sm" data-mview="${esc(m.computer)}">${icon('eye')}${esc(t('act.view'))}</button></div></div></div>`).join('');
  }
  if (p._html === html) return;
  p._html = html;
  p.innerHTML = html;
  $$('[data-hview]', p).forEach(b => b.onclick = () => openViewerById(b.dataset.hview, false));
  $$('[data-mview]', p).forEach(b => b.onclick = () => openViewerById(b.dataset.mview, false));
  $$('[data-hlow]', p).forEach(b => b.onclick = () => act('hand_lower', {}, [comp(b.dataset.hlow)].filter(Boolean), 'done.hand_lower'));
  $$('[data-reply]', p).forEach(b => b.onclick = () => { const c = comp(b.dataset.reply); if (online(c)) messageDialog([c]); else toast(t('state.offline'), 'bad'); });
  const la = $('#lowerAll', p); if (la) la.onclick = () => act('hand_lower', {}, hands, 'done.hand_lower');
  const ic = $('#inClear', p); if (ic) ic.onclick = () => api('inbox/clear', { body: {} });
}

// ===================================================================== polls

function renderPolls() {
  const p = $('#pollsPage');
  const poll = S.state.poll;
  let html;
  if (!poll) html = emptyState('poll', t('polls.emptyTitle'), t('polls.emptyText'), [`<button class="btn primary" id="pNew">${icon('plus')}${esc(t('polls.new'))}</button>`]);
  else {
    const max = Math.max(1, ...poll.counts);
    const answered = poll.answered || 0;
    const answers = Object.entries(poll.answers || {}).map(([id, a]) => ({ c: comp(id), a })).filter(x => x.c);
    const waiting = (poll.targets || []).map(comp).filter(c => c && !(c.id in (poll.answers || {})));
    html = `<div class="card"><div style="display:flex;gap:12px;align-items:flex-start"><div style="flex:1"><h2>${icon('poll')}${esc(poll.question || t('poll.title'))}</h2>
      <div class="pollmeta"><span>${poll.active ? `<span class="tag good">● ${esc(t('polls.live'))}</span>` : `<span class="tag">${esc(t('polls.closed'))}</span>`}</span><span>${esc(t('polls.answered', { a: answered, n: poll.total }))}</span><span>${esc(fmtTime(poll.started))}</span></div></div>
      <div style="display:flex;gap:8px">${poll.active ? `<button class="btn" id="pClose">${icon('stop')}${esc(t('polls.close'))}</button>` : ''}<button class="btn" id="pCsv">${icon('download')}CSV</button><button class="btn primary" id="pNew">${icon('plus')}${esc(t('polls.new'))}</button></div></div>
      <div class="bars">${poll.options.map((o, i) => { const n = poll.counts[i] || 0; const pct = answered ? Math.round(n / answered * 100) : 0; return `<div class="bar"><i style="width:${n / max * 100}%"></i><span>${esc(o)}</span><b>${n} · ${pct}%</b></div>`; }).join('')}</div>
      ${answers.length ? `<h3 style="margin:18px 0 0;font-size:14px">${esc(t('polls.byStudent'))}</h3><div class="answer-list">${answers.map(x => `<div><b>${esc(x.c.name)}${x.c.st && x.c.st.user ? ' · ' + esc(x.c.st.user) : ''}</b><span>${esc(poll.options[x.a] || '?')}</span></div>`).join('')}</div>` : ''}
      ${poll.active && waiting.length ? `<h3 style="margin:18px 0 0;font-size:14px;color:var(--muted)">${esc(tn('polls.waiting', waiting.length))}</h3><div class="answer-list">${waiting.map(c => `<div><b>${esc(c.name)}</b><span class="hint">…</span></div>`).join('')}</div>` : ''}
      <div style="margin-top:16px"><button class="btn sm ghost" id="pClear">${icon('trash')}${esc(t('polls.clear'))}</button></div></div>`;
  }
  if (p._html === html) return;
  p._html = html;
  p.innerHTML = html;
  const pn = $('#pNew', p); if (pn) pn.onclick = () => { setView('computers'); pollDialog(targets()); };
  const pc = $('#pClose', p); if (pc) pc.onclick = () => api('poll/close', { body: {} });
  const pl = $('#pClear', p); if (pl) pl.onclick = () => api('poll/clear', { body: {} });
  const cs = $('#pCsv', p); if (cs) cs.onclick = () => exportPollCSV(poll);
}

function exportPollCSV(poll) {
  const rows = [[t('discover.colName'), t('discover.colUser'), t('polls.answer')]];
  for (const [id, a] of Object.entries(poll.answers || {})) { const c = comp(id); rows.push([c ? c.name : id, c && c.st ? c.st.user : '', poll.options[a] || '']); }
  rows.push([], [poll.question], ...poll.options.map((o, i) => [o, poll.counts[i]]));
  const csv = '﻿' + rows.map(r => r.map(v => `"${String(v ?? '').replace(/"/g, '""')}"`).join(';')).join('\r\n');
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([csv], { type: 'text/csv' }));
  a.download = `orbit-poll-${new Date().toISOString().slice(0, 16).replace(/[:T]/g, '-')}.csv`;
  a.click();
}

// ===================================================================== screenshots

async function loadShots() {
  try { S.shots = await api('screenshots'); } catch (e) { S.shots = { items: [], dir: '' }; }
  renderShots();
}

function renderShots() {
  const p = $('#shotsPage');
  const s = S.shots || { items: [] };
  p.innerHTML = `<div style="display:flex;gap:10px;align-items:center;margin-bottom:16px;flex-wrap:wrap"><span class="hint mono">${esc(s.dir || '')}</span><div class="spacer"></div>
    <button class="btn" id="shTake">${icon('camera')}${esc(t('shots.takeRoom'))}</button><button class="btn" id="shOpen">${icon('folder')}${esc(t('shots.openFolder'))}</button><button class="btn ghost" id="shRef">${icon('refresh')}</button></div>
    ${s.items.length ? `<div class="shots">${s.items.map(x => `<div class="shotcard" data-n="${esc(x.name)}"><img loading="lazy" src="/api/screenshots/file/${encodeURIComponent(x.name)}" alt=""><div><span title="${esc(x.name)}">${esc(x.name.replace(/\.png$/, '').replace(/_/g, ' '))}</span><button class="icon-btn" data-del="${esc(x.name)}" title="${esc(t('btn.delete'))}">${icon('trash')}</button></div></div>`).join('')}</div>`
      : emptyState('camera', t('shots.emptyTitle'), t('shots.emptyText'))}`;
  $('#shTake', p).onclick = () => takeScreenshots(roomComputers().filter(online)).then(loadShots);
  $('#shOpen', p).onclick = () => api('open-folder', { body: { which: 'screenshots' } }).catch(e => toast(t('toast.error'), 'bad', { text: e.message }));
  $('#shRef', p).onclick = loadShots;
  $$('.shotcard', p).forEach(el => el.onclick = e => {
    if (e.target.closest('[data-del]')) return;
    const lb = document.createElement('div');
    lb.className = 'lightbox';
    lb.innerHTML = `<img src="/api/screenshots/file/${encodeURIComponent(el.dataset.n)}">`;
    lb.onclick = () => lb.remove();
    document.body.appendChild(lb);
  });
  $$('[data-del]', p).forEach(b => b.onclick = async () => { await api('screenshots/file/' + encodeURIComponent(b.dataset.del), { method: 'DELETE' }); loadShots(); });
}

// ===================================================================== help

async function renderHelp() {
  const p = $('#helpPage');
  const lang = S.docLang || I18N.lang;
  let md = '';
  try { md = await (await fetch('/api/docs/' + lang)).text(); } catch { md = '# Help\n\nCould not load the guide.'; }
  const { html, toc } = renderMarkdown(md);
  const langBtns = S.langs.map(l => `<button class="${l.code === lang ? 'on' : ''}" data-dl="${esc(l.code)}">${esc(l.code.toUpperCase())}</button>`).join('');
  p.innerHTML = `<div class="help"><nav class="toc"><input class="input" id="hSearch" placeholder="${esc(t('help.search'))}" style="margin-bottom:8px"><div class="seg" style="margin-bottom:10px">${langBtns}</div>
    ${toc.map(x => `<a href="#${x.id}" class="h${x.lvl}" data-id="${x.id}">${esc(x.text)}</a>`).join('')}</nav><article class="doc" id="hDoc">${html}</article></div>`;
  $$('[data-dl]', p).forEach(b => b.onclick = () => { S.docLang = b.dataset.dl; renderHelp(); });
  $$('.toc a', p).forEach(a => a.onclick = e => { e.preventDefault(); const el = document.getElementById(a.dataset.id); el && el.scrollIntoView({ behavior: 'smooth', block: 'start' }); });
  const doc = $('#hDoc', p);
  $('#hSearch', p).oninput = debounce(e => {
    const q = e.target.value.trim().toLowerCase();
    doc.innerHTML = html;
    if (!q) return;
    const walker = document.createTreeWalker(doc, NodeFilter.SHOW_TEXT);
    const hits = [];
    while (walker.nextNode()) if (walker.currentNode.nodeValue.toLowerCase().includes(q)) hits.push(walker.currentNode);
    hits.forEach(n => {
      const span = document.createElement('span');
      span.innerHTML = esc(n.nodeValue).replace(new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi'), m => `<mark>${m}</mark>`);
      n.replaceWith(span);
    });
    const first = $('mark', doc); first && first.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }, 250);
}

// ===================================================================== settings

function renderSettings() {
  const p = $('#settingsPage');
  const s = S.settings;
  const sw = (id, on) => `<label class="switch"><input type="checkbox" id="${id}" ${on ? 'checked' : ''}><i></i></label>`;
  const row = (title, desc, ctl) => `<div class="set-row"><div class="lbl"><b>${esc(title)}</b>${desc ? `<span>${esc(desc)}</span>` : ''}</div><div class="ctl">${ctl}</div></div>`;
  const range = (id, min, max, step, val, fmt) => `<input type="range" id="${id}" min="${min}" max="${max}" step="${step}" value="${val}"><output id="${id}O">${fmt(val)}</output>`;
  const favRows = kind => (s[kind] || []).map((f, i) => `<div class="item"><input class="input" data-fk="${kind}" data-fi="${i}" data-ff="name" value="${esc(f.name)}"><input class="input mono" data-fk="${kind}" data-fi="${i}" data-ff="value" value="${esc(f.value)}"><button class="icon-btn" data-fdel="${kind}" data-fi="${i}">${icon('trash')}</button></div>`).join('');
  const caRows = (s.customActions || []).map((a, i) => `<div class="item ca"><input class="input" data-ci="${i}" data-cf="name" value="${esc(a.name)}" placeholder="${esc(t('custom.name'))}">
    <select class="input" data-ci="${i}" data-cf="type">${['run', 'url', 'message'].map(x => `<option value="${x}" ${a.type === x ? 'selected' : ''}>${esc(t('custom.type.' + x))}</option>`).join('')}</select>
    <input class="input mono" data-ci="${i}" data-cf="value" value="${esc(a.value)}" placeholder="${esc(t('custom.value'))}"><input class="input mono" data-ci="${i}" data-cf="args" value="${esc(a.args || '')}" placeholder="${esc(t('custom.args'))}"><button class="icon-btn" data-cdel="${i}">${icon('trash')}</button></div>`).join('');
  const sec = v => (v / 1000).toFixed(v < 1000 ? 2 : 1).replace(/\.?0+$/, '') + ' s';
  p.innerHTML = `
  <div class="card"><h2>${icon('palette')}${esc(t('settings.general'))}</h2><p class="hint"></p>
    ${row(t('settings.language'), t('settings.languageHint'), `<select class="input" id="sLang" style="width:200px">${S.langs.map(l => `<option value="${esc(l.code)}" ${l.code === I18N.lang ? 'selected' : ''}>${esc(l.name)}</option>`).join('')}</select>`)}
    ${row(t('settings.theme'), '', `<div class="seg" id="sTheme">${['dark', 'light', 'auto'].map(x => `<button data-v="${x}" class="${(s.theme || 'dark') === x ? 'on' : ''}">${icon(x === 'dark' ? 'moon' : x === 'light' ? 'sun' : 'sparkles')}${esc(t('theme.' + x))}</button>`).join('')}</div>`)}
    ${row(t('settings.accent'), '', `<div class="swatches">${Object.entries(ACCENTS).map(([k, v]) => `<span class="swatch ${s.accent === k || (!s.accent && k === 'indigo') ? 'on' : ''}" data-acc="${k}" title="${k}" style="background:linear-gradient(135deg,${v[0]},${v[2]})"></span>`).join('')}</div>`)}
    ${row(t('settings.sounds'), t('settings.soundsHint'), sw('sSounds', s.sounds))}
    ${row(t('settings.activity'), t('settings.activityHint'), sw('sAct', s.showActivity))}
    ${row(t('settings.confirmPower'), t('settings.confirmPowerHint'), sw('sConf', s.confirmPower))}
  </div>
  <div class="card"><h2>${icon('zap')}${esc(t('settings.performance'))}</h2><p class="hint">${esc(t('settings.performanceHint'))}</p>
    ${row(t('settings.thumbInterval'), t('settings.thumbIntervalHint'), range('sThumb', 250, 5000, 250, s.thumbInterval, sec))}
    ${row(t('settings.viewQuality'), t('settings.viewQualityHint'), range('sVQ', 20, 95, 5, s.viewQuality, v => v + '%'))}
    ${row(t('settings.viewFps'), '', range('sVF', 2, 30, 1, s.viewFps, v => v + ' fps'))}
    ${row(t('settings.viewMax'), t('settings.viewMaxHint'), `<select class="input" id="sVM" style="width:140px">${[1280, 1600, 1920, 2560, 3840].map(w => `<option ${s.viewMaxW === w ? 'selected' : ''}>${w}</option>`).join('')}</select>`)}
    ${row(t('settings.demoQuality'), '', range('sDQ', 20, 95, 5, s.demoQuality, v => v + '%'))}
    ${row(t('settings.demoFps'), t('settings.demoFpsHint'), range('sDF', 2, 30, 1, s.demoFps, v => v + ' fps'))}
    ${row(t('settings.demoMax'), '', `<select class="input" id="sDM" style="width:140px">${[1280, 1600, 1920, 2560, 3840].map(w => `<option ${s.demoMaxW === w ? 'selected' : ''}>${w}</option>`).join('')}</select>`)}
  </div>
  <div class="card"><h2>${icon('lock')}${esc(t('settings.lock'))}</h2><p class="hint">${esc(t('settings.lockHint'))}</p>
    <div class="form"><label class="field">${esc(t('lock.titleLbl'))}<input class="input" id="sLT" value="${esc(s.lockTitle || '')}" placeholder="${esc(t('lock.titlePh'))}"></label>
    <label class="field">${esc(t('lock.textLbl'))}<input class="input" id="sLX" value="${esc(s.lockText || '')}" placeholder="${esc(t('lock.textPh'))}"></label></div></div>
  <div class="card" id="set-key"><h2>${icon('key')}${esc(t('key.title'))}</h2><p class="hint">${esc(t('key.text'))}</p>
    <div class="keybox"><code id="pubKey">${esc(S.meta.publicKey)}</code><button class="icon-btn" id="copyPub" title="${esc(t('toast.copy'))}">${icon('copy')}</button></div>
    <div class="hint" style="margin:8px 0 14px">${esc(t('key.fingerprint'))}: <b class="mono">${esc(S.meta.fingerprint)}</b></div>
    <div style="display:flex;gap:8px;flex-wrap:wrap"><a class="btn primary" href="/api/agent-package">${icon('package')}${esc(t('setup.package'))}</a><a class="btn" href="/api/agent-config">${icon('download')}orbit-agent.json</a>
      <button class="btn" id="kBackup">${icon('shield')}${esc(t('key.backup'))}</button><button class="btn" id="kImport">${icon('key')}${esc(t('key.import'))}</button><button class="btn danger" id="kRegen">${icon('refresh')}${esc(t('key.regen'))}</button></div></div>
  <div class="card"><h2>${icon('star')}${esc(t('settings.favorites'))}</h2><p class="hint">${esc(t('settings.favoritesHint'))}</p>
    <h3 style="font-size:13px;margin:6px 0 8px">${esc(t('act.website'))}</h3><div class="list-edit">${favRows('websites')}</div><button class="btn sm" data-fadd="websites" style="margin-top:8px">${icon('plus')}${esc(t('btn.add'))}</button>
    <h3 style="font-size:13px;margin:16px 0 8px">${esc(t('act.run'))}</h3><div class="list-edit">${favRows('programs')}</div><button class="btn sm" data-fadd="programs" style="margin-top:8px">${icon('plus')}${esc(t('btn.add'))}</button></div>
  <div class="card" id="set-custom"><h2>${icon('sparkles')}${esc(t('custom.title'))}</h2><p class="hint">${esc(t('custom.text'))}</p>
    <div class="list-edit">${caRows || `<div class="hint">${esc(t('custom.none'))}</div>`}</div><button class="btn sm" id="caAdd" style="margin-top:10px">${icon('plus')}${esc(t('custom.add'))}</button></div>
  <div class="card"><h2>${icon('api')}${esc(t('api.title'))}</h2><p class="hint">${esc(t('api.text'))}</p>
    ${row(t('api.token'), t('api.tokenHint'), `<code class="mono" style="max-width:300px;overflow:hidden;text-overflow:ellipsis">${esc(s.apiToken || '—')}</code><button class="btn sm" id="apiGen">${esc(s.apiToken ? t('api.regen') : t('api.gen'))}</button>${s.apiToken ? `<button class="btn sm" id="apiCopy">${icon('copy')}</button><button class="btn sm danger" id="apiOff">${icon('x')}</button>` : ''}`)}
    <pre class="keybox mono" style="white-space:pre-wrap;margin-top:10px">curl -H "Authorization: Bearer ${esc(s.apiToken || '<token>')}" http://127.0.0.1:${location.port}/api/state
curl -X POST -H "Authorization: Bearer ${esc(s.apiToken || '<token>')}" -d "{\\"targets\\":[\\"ID\\"],\\"action\\":\\"lock\\"}" http://127.0.0.1:${location.port}/api/action</pre></div>
  <div class="card"><h2>${icon('folder')}${esc(t('settings.storage'))}</h2><p class="hint"></p>
    ${row(t('settings.configDir'), S.meta.configDir, `<button class="btn sm" id="openCfg">${icon('folder')}${esc(t('btn.open'))}</button>`)}
    <div class="set-row"><div class="lbl"><b>${esc(t('settings.shotsDir'))}</b><span>${esc(t('settings.shotsDirHint'))}</span></div><div class="ctl" style="flex:1;max-width:420px"><input class="input mono" id="sShots" value="${esc(s.screenshotDir || '')}" placeholder="${esc(S.meta.screenshotsDir)}"></div></div></div>
  <div class="card"><h2>${icon('info')}${esc(t('about.title'))}</h2>
    <p class="hint">Orbit Classroom ${esc(S.meta.version)} · ${esc(S.meta.masterName || '')} · ${esc(t('about.ports', { tcp: S.meta.agentPort, udp: S.meta.discoveryPort }))}</p>
    <div style="display:flex;gap:8px;flex-wrap:wrap"><button class="btn" id="aOnb">${icon('sparkles')}${esc(t('about.onboarding'))}</button><button class="btn" id="aHelp">${icon('help')}${esc(t('nav.help'))}</button><button class="btn danger" id="aQuit">${icon('power')}${esc(t('about.quit'))}</button></div></div>`;

  const bindRange = (id, key, fmt) => { const el = $('#' + id, p); el.oninput = () => { $('#' + id + 'O', p).textContent = fmt(+el.value); }; el.onchange = () => saveSettings({ [key]: +el.value }); };
  $('#sLang', p).onchange = e => switchLang(e.target.value);
  $$('#sTheme button', p).forEach(b => b.onclick = () => { saveSettings({ theme: b.dataset.v }); applyTheme(); $('#sidebar')._html = ''; renderSidebar(); renderSettings(); });
  $$('[data-acc]', p).forEach(b => b.onclick = () => { saveSettings({ accent: b.dataset.acc }); applyTheme(); renderSettings(); });
  $('#sSounds', p).onchange = e => { saveSettings({ sounds: e.target.checked }); if (e.target.checked) sound('hand'); };
  $('#sAct', p).onchange = e => saveSettings({ showActivity: e.target.checked });
  $('#sConf', p).onchange = e => saveSettings({ confirmPower: e.target.checked });
  bindRange('sThumb', 'thumbInterval', sec); bindRange('sVQ', 'viewQuality', v => v + '%'); bindRange('sVF', 'viewFps', v => v + ' fps');
  bindRange('sDQ', 'demoQuality', v => v + '%'); bindRange('sDF', 'demoFps', v => v + ' fps');
  $('#sVM', p).onchange = e => saveSettings({ viewMaxW: +e.target.value });
  $('#sDM', p).onchange = e => saveSettings({ demoMaxW: +e.target.value });
  $('#sLT', p).onchange = e => saveSettings({ lockTitle: e.target.value.trim() });
  $('#sLX', p).onchange = e => saveSettings({ lockText: e.target.value.trim() });
  $('#sShots', p).onchange = e => saveSettings({ screenshotDir: e.target.value.trim() });
  $('#copyPub', p).onclick = () => copyText(S.meta.publicKey);
  $('#kBackup', p).onclick = async () => {
    if (!(await confirmDialog(t('key.backup'), t('key.backupWarn'), t('key.show'), false))) return;
    const r = await api('key/secret');
    modal({ title: t('key.backup'), icon: 'shield', body: `<div class="form"><div class="note warn">${icon('alert')}<div>${esc(t('key.backupText'))}</div></div><div class="keybox"><code>${esc(r.secret)}</code></div></div>`,
      buttons: [{ label: t('toast.copy'), icon: 'copy', onClick: () => { copyText(r.secret); return true; } }, { label: t('btn.close'), primary: true }] });
  };
  $('#kImport', p).onclick = () => modal({
    title: t('key.import'), icon: 'key', body: `<div class="form"><p class="hint" style="margin:0">${esc(t('key.importText'))}</p><textarea class="input mono" id="kIn" rows="3" placeholder="OTK1-…"></textarea></div>`,
    buttons: [{ label: t('btn.cancel'), ghost: true }, { label: t('key.import'), primary: true, onClick: async bd => { applyMeta(await api('key/import', { body: { key: $('#kIn', bd).value } })); toast(t('key.imported'), 'good'); renderSettings(); } }],
  });
  $('#kRegen', p).onclick = async () => {
    if (!(await confirmDialog(t('key.regen'), t('key.regenWarn'), t('key.regen')))) return;
    applyMeta(await api('key/regenerate', { body: {} }));
    toast(t('key.regenerated'), 'good');
    renderSettings();
  };
  const favs = kind => (S.settings[kind] || []).map(f => ({ ...f }));
  $$('[data-fk]', p).forEach(inp => inp.onchange = () => { const list = favs(inp.dataset.fk); list[+inp.dataset.fi][inp.dataset.ff] = inp.value.trim(); saveSettings({ [inp.dataset.fk]: list }); });
  $$('[data-fdel]', p).forEach(b => b.onclick = () => { const list = favs(b.dataset.fdel); list.splice(+b.dataset.fi, 1); saveSettings({ [b.dataset.fdel]: list }); renderSettings(); });
  $$('[data-fadd]', p).forEach(b => b.onclick = () => { saveSettings({ [b.dataset.fadd]: [...favs(b.dataset.fadd), { name: '', value: '' }] }); renderSettings(); });
  const cas = () => (S.settings.customActions || []).map(a => ({ ...a }));
  $$('[data-ci]', p).forEach(inp => inp.onchange = () => { const list = cas(); list[+inp.dataset.ci][inp.dataset.cf] = inp.value; saveSettings({ customActions: list }); });
  $$('[data-cdel]', p).forEach(b => b.onclick = () => { const list = cas(); list.splice(+b.dataset.cdel, 1); saveSettings({ customActions: list }); renderSettings(); });
  $('#caAdd', p).onclick = () => { saveSettings({ customActions: [...cas(), { name: t('custom.newName'), type: 'run', value: 'notepad.exe', args: '' }] }); renderSettings(); };
  $('#apiGen', p).onclick = () => { const a = new Uint8Array(24); crypto.getRandomValues(a); saveSettings({ apiToken: 'orb_' + [...a].map(x => x.toString(16).padStart(2, '0')).join('') }); setTimeout(renderSettings, 400); };
  const ac = $('#apiCopy', p); if (ac) ac.onclick = () => copyText(s.apiToken);
  const ao = $('#apiOff', p); if (ao) ao.onclick = () => { saveSettings({ apiToken: '' }); setTimeout(renderSettings, 400); };
  $('#openCfg', p).onclick = () => api('open-folder', { body: { which: 'config' } }).catch(e => toast(t('toast.error'), 'bad', { text: e.message }));
  $('#aOnb', p).onclick = onboarding;
  $('#aHelp', p).onclick = () => setView('help');
  $('#aQuit', p).onclick = async () => { if (await confirmDialog(t('about.quit'), t('about.quitText'), t('about.quit'))) { await api('quit', { body: {} }); showClosed(); } };
}

// ===================================================================== command palette

function paletteItems() {
  const items = [];
  const add = (group, ic, label, run, hint = '', kw = '') => items.push({ group, icon: ic, label, run, hint, kw: (label + ' ' + kw).toLowerCase() });
  TOOLS.filter(x => !x.sep && x.id !== 'more' && x.id !== 'power' && x.id !== 'demo' && x.id !== 'lock').forEach(x => add(t('pal.actions'), x.icon, t(x.label), () => runTool(x.id, $(`[data-tool="${x.id}"]`))));
  add(t('pal.actions'), 'lock', t('lock.screen'), () => act('lock', { mode: 'screen' }), 'L');
  add(t('pal.actions'), 'unlock', t('lock.unlock'), () => act('unlock', {}), 'U');
  add(t('pal.actions'), 'cast', t('demo.fullscreen'), () => startDemo('teacher', 'fullscreen'));
  add(t('pal.actions'), 'monitor', t('demo.window'), () => startDemo('teacher', 'window'));
  if (S.state.demo) add(t('pal.actions'), 'stop', t('demo.stop'), stopDemo);
  add(t('pal.actions'), 'zap', t('power.on'), () => wake(targets(true).filter(c => !online(c))));
  add(t('pal.actions'), 'restart', t('power.reboot'), () => powerDialog('reboot', targets()));
  add(t('pal.actions'), 'power', t('power.shutdown'), () => powerDialog('shutdown', targets()));
  add(t('pal.actions'), 'logout', t('power.logoff'), () => powerDialog('logoff', targets()));
  add(t('pal.actions'), 'hand', t('more.lowerHands'), () => act('hand_lower', {}, roomComputers().filter(c => c.st && c.st.hand), 'done.hand_lower'));
  add(t('pal.actions'), 'star', S.spot ? t('more.spotOff') : t('more.spotOn'), toggleSpot);
  (S.settings.customActions || []).forEach(a => add(t('more.custom'), 'sparkles', a.name, () => runCustom(a)));
  NAV.concat([{ id: 'help', icon: 'help', label: 'nav.help' }, { id: 'settings', icon: 'settings', label: 'nav.settings' }]).forEach(n => add(t('pal.go'), n.icon, t(n.label), () => setView(n.id)));
  S.state.rooms.forEach(r => add(t('nav.rooms'), 'layers', r.name, () => { selectRoom(r.id); setView('computers'); }));
  S.state.computers.forEach(c => add(t('pal.computers'), online(c) ? 'monitor' : 'offline', c.name, () => { if (online(c)) openViewer(c, false); else { selectRoom(c.room); setView('computers'); } }, c.st && c.st.user ? c.st.user : roomName(c.room), `${c.host} ${(c.st && c.st.user) || ''}`));
  add(t('settings.theme'), 'sun', t('theme.light'), () => { saveSettings({ theme: 'light' }); applyTheme(); $('#sidebar')._html = ''; renderSidebar(); });
  add(t('settings.theme'), 'moon', t('theme.dark'), () => { saveSettings({ theme: 'dark' }); applyTheme(); $('#sidebar')._html = ''; renderSidebar(); });
  S.langs.forEach(l => add(t('settings.language'), 'language', l.name, () => switchLang(l.code), l.code.toUpperCase()));
  add(t('computer.add'), 'plus', t('computer.add'), () => computerDialog());
  add(t('room.add'), 'plus', t('room.add'), addRoomDialog);
  return items;
}

function fuzzy(q, s) {
  if (!q) return 1;
  if (s.includes(q)) return 100 - s.indexOf(q);
  let i = 0, score = 0;
  for (const ch of s) { if (ch === q[i]) { i++; score++; } if (i === q.length) return score; }
  return 0;
}

function openPalette() {
  if ($('.palette')) return;
  const all = paletteItems();
  const el = document.createElement('div');
  el.className = 'palette';
  el.innerHTML = `<div class="box"><div class="pin">${icon('search')}<input id="palIn" placeholder="${esc(t('pal.placeholder'))}"><kbd>Esc</kbd></div><div class="res" id="palRes"></div></div>`;
  document.body.appendChild(el);
  const inp = $('#palIn', el), res = $('#palRes', el);
  let hl = 0, shown = [];
  const draw = () => {
    const q = inp.value.trim().toLowerCase();
    shown = all.map(x => ({ x, s: fuzzy(q, x.kw) })).filter(o => o.s > 0).sort((a, b) => b.s - a.s).slice(0, 60).map(o => o.x);
    if (!q) shown = all.slice(0, 40);
    hl = clamp(hl, 0, shown.length - 1);
    let grp = '';
    res.innerHTML = shown.map((x, i) => { const g = x.group !== grp ? `<div class="grp">${esc(x.group)}</div>` : ''; grp = x.group; return g + `<button data-i="${i}" class="${i === hl ? 'hl' : ''}">${icon(x.icon)}<span>${esc(x.label)}</span>${x.hint ? `<small>${esc(x.hint)}</small>` : ''}</button>`; }).join('') || `<div class="grp">${esc(t('pal.none'))}</div>`;
    $$('button', res).forEach(b => { b.onclick = () => run(+b.dataset.i); b.onmousemove = () => { if (hl !== +b.dataset.i) { hl = +b.dataset.i; $$('button', res).forEach(x => x.classList.toggle('hl', +x.dataset.i === hl)); } }; });
    const cur = $('button.hl', res); cur && cur.scrollIntoView({ block: 'nearest' });
  };
  const close = () => el.remove();
  const run = i => { const x = shown[i]; close(); x && x.run(); };
  inp.oninput = () => { hl = 0; draw(); };
  inp.onkeydown = e => {
    if (e.key === 'ArrowDown') { e.preventDefault(); hl = Math.min(hl + 1, shown.length - 1); draw(); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); hl = Math.max(hl - 1, 0); draw(); }
    else if (e.key === 'Enter') { e.preventDefault(); run(hl); }
    else if (e.key === 'Escape') { e.preventDefault(); close(); }
  };
  el.addEventListener('mousedown', e => { if (e.target === el) close(); });
  draw();
  inp.focus();
}

// ===================================================================== keyboard shortcuts

function bindKeys() {
  document.addEventListener('keydown', e => {
    if ($('.viewer') || $('.backdrop')) return;
    const typing = e.target.closest('input, textarea, select, [contenteditable]');
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); openPalette(); return; }
    if (typing || $('.palette')) return;
    if (e.key === 'Escape') { closeMenu(); if (S.selected.size) clearSelection(); return; }
    if (S.view !== 'computers') { if (e.key === '?') setView('help'); return; }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') { e.preventDefault(); selectAll(); return; }
    if (e.ctrlKey || e.metaKey || e.altKey) {
      if (e.altKey && /^[1-9]$/.test(e.key)) { const r = S.state.rooms[+e.key - 1]; if (r) { e.preventDefault(); selectRoom(r.id); } }
      return;
    }
    const k = e.key.toLowerCase();
    if (k === 'l') act('lock', { mode: 'screen' });
    else if (k === 'u') act('unlock', {});
    else if (k === 'm') messageDialog(targets());
    else if (k === 'p') pollDialog(targets());
    else if (k === 'v') runTool('view');
    else if (k === 'c') runTool('control');
    else if (k === 'd') runTool('demo', $('[data-tool="demo"]'));
    else if (k === 's') toggleSpot();
    else if (k === '?') setView('help');
    else if (k === '+' || k === '=') zoomBy(20);
    else if (k === '-') zoomBy(-20);
  });
}

function zoomBy(d) {
  const v = clamp((S.settings.tileSize || 300) + d, 180, 560);
  saveSettings({ tileSize: v });
  document.documentElement.style.setProperty('--tile', v + 'px');
  const z = $('#zoom'); if (z) z.value = v;
  sendView();
}

// ===================================================================== onboarding

function onboarding() {
  let step = 0;
  const steps = [
    () => `<div class="onb-hero"><div class="brand-logo">${icon('logo')}</div><h2>${esc(t('onb.welcome'))}</h2><p>${esc(t('onb.welcomeText'))}</p></div>
      <div class="lang-pick">${S.langs.map(l => `<button data-ol="${esc(l.code)}" class="${l.code === I18N.lang ? 'on' : ''}"><b>${esc(l.name)}</b><span>${esc(l.code.toUpperCase())}</span></button>`).join('')}</div>`,
    () => `<div class="form"><h3 style="margin:0">${esc(t('onb.howTitle'))}</h3><ol class="steps"><li>${esc(t('onb.how1'))}</li><li>${esc(t('onb.how2'))}</li><li>${esc(t('onb.how3'))}</li></ol>
      <div class="keybox"><code>${esc(S.meta.fingerprint)}</code><span class="hint">${esc(t('key.fingerprint'))}</span></div>
      <div style="display:flex;gap:8px;flex-wrap:wrap"><a class="btn primary" href="/api/agent-package">${icon('package')}${esc(t('setup.package'))}</a></div>
      <div class="note">${icon('shield')}<div>${esc(t('onb.security'))}</div></div></div>`,
    () => `<div class="onb-hero"><div class="brand-logo">${icon('radar')}</div><h2>${esc(t('onb.findTitle'))}</h2><p>${esc(t('onb.findText'))}</p></div>`,
  ];
  const m = modal({ title: t('onb.title'), icon: 'sparkles', wide: false, dismiss: true, body: '<div id="onbBody"></div><div class="dots-nav" id="onbDots" style="margin-top:18px"></div>', buttons: [
    { label: t('onb.skip'), ghost: true, left: true, onClick: () => { saveSettings({ onboarded: true }); } },
    { label: t('onb.back'), onClick: () => { step = Math.max(0, step - 1); draw(); return true; } },
    { label: t('onb.next'), primary: true, onClick: () => {
      if (step < steps.length - 1) { step++; draw(); return true; }
      saveSettings({ onboarded: true });
      setView('discover'); runDiscover();
    } },
  ] });
  const draw = () => {
    $('#onbBody', m.el).innerHTML = steps[step]();
    $('#onbDots', m.el).innerHTML = steps.map((_, i) => `<i class="${i === step ? 'on' : ''}"></i>`).join('');
    const btns = $$('footer .btn', m.el);
    btns[1].style.visibility = step ? 'visible' : 'hidden';
    btns[2].lastChild.textContent = step === steps.length - 1 ? t('onb.finish') : t('onb.next');
    $$('[data-ol]', m.el).forEach(b => b.onclick = async () => { await switchLang(b.dataset.ol); m.close(); onboarding(); });
  };
  draw();
}

init();

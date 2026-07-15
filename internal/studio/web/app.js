/* GramGrabber Studio frontend */
'use strict';

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const api = {
  async get(path) {
    const r = await fetch(path);
    const data = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(data.error || r.statusText);
    return data;
  },
  async send(method, path, body) {
    const r = await fetch(path, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
    });
    const data = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(data.error || r.statusText);
    return data;
  },
};

/* ---------- formatting ---------- */
const fmtSize = (b) => {
  if (!b) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (b >= 1024 && i < units.length - 1) { b /= 1024; i++; }
  return `${b.toFixed(b >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
};
const fmtDur = (s) => {
  if (!s) return '—';
  s = Math.round(s);
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
           : `${m}:${String(sec).padStart(2, '0')}`;
};
const fmtDate = (unix) => unix
  ? new Date(unix * 1000).toLocaleDateString(undefined, { day: '2-digit', month: 'short', year: '2-digit' })
  : '—';
const esc = (s) => s.replace(/[&<>"']/g, (c) => (
  { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
// "M0A1_Introdução_Ao_Branding_By_@xEistibus_💘.mp4" reads better as
// "M0A1 Introdução Ao Branding": drop extension, underscores and the
// repeated "By @author" suffix (kept in the title attribute).
const cleanName = (n) => n
  .replace(/\.[a-z0-9]{2,4}$/i, '')
  .replace(/_/g, ' ')
  .replace(/[\s\-–—·]*by\s+@\S+.*$/i, '')
  .replace(/\s+/g, ' ')
  .replace(/[\s\-–—·]+$/, '')
  .trim() || cleanBasic(n);
const cleanBasic = (n) => n.replace(/\.[a-z0-9]{2,4}$/i, '').replace(/_/g, ' ').trim();

/* duration cache: filesystem listings don't know durations, so remember
   them whenever a video's metadata loads (library thumbs, the player). */
let durCache = {};
try { durCache = JSON.parse(localStorage.getItem('ggdur') || '{}'); } catch { /* fresh */ }
function saveDur(path, d) {
  if (!d || !isFinite(d) || durCache[path]) return;
  durCache[path] = Math.round(d);
  localStorage.setItem('ggdur', JSON.stringify(durCache));
}
const getDur = (item) => item.duration || durCache[item.path] || 0;

/* watched-position memory (per file, local to this machine) */
const posKey = (path) => 'ggpos:' + path;
const loadPos = (path) => {
  try { return JSON.parse(localStorage.getItem(posKey(path))) || null; } catch { return null; }
};
const savePos = (path, t, d) => {
  if (!d) return;
  if (t >= d - 20) localStorage.setItem(posKey(path), JSON.stringify({ t: 0, d, done: 1 }));
  else if (t > 10) localStorage.setItem(posKey(path), JSON.stringify({ t, d }));
};
// Telegram answers bursts with FLOOD_WAIT (n seconds); translate for humans.
const friendlyError = (msg) => {
  const flood = msg.match(/FLOOD_WAIT \((\d+)\)/);
  if (flood) return `Telegram asked us to slow down — retrying automatically. If it persists, wait ${flood[1]}s and try again.`;
  return msg;
};

/* ---------- toasts ---------- */
function toast(msg, type = 'info') {
  const el = document.createElement('div');
  el.className = `toast toast-${type}`;
  el.textContent = msg;
  $('#toasts').appendChild(el);
  setTimeout(() => el.classList.add('out'), 3600);
  setTimeout(() => el.remove(), 4000);
}

/* ---------- busy buttons ---------- */
function setBusy(btn, busy, busyText = 'Working…') {
  if (busy) {
    btn.dataset.label = btn.textContent;
    btn.textContent = busyText;
    btn.disabled = true;
  } else {
    if (btn.dataset.label) btn.textContent = btn.dataset.label;
    btn.disabled = false;
  }
}

/* ---------- skeletons & empty states ---------- */
const skeletonList = (n, cls) =>
  Array.from({ length: n }, () => `<div class="skel ${cls}"></div>`).join('');
const emptyState = (icon, title, hint) => `
  <div class="empty">
    <svg viewBox="0 0 24 24" aria-hidden="true"><path d="${icon}"/></svg>
    <h3>${title}</h3>
    <p>${hint}</p>
  </div>`;
const ICONS = {
  play: 'M4 4h16a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1Zm6 4.5v7l6-3.5-6-3.5Z',
  down: 'M12 3v10.6l3.8-3.8 1.4 1.4-6.2 6.2-6.2-6.2 1.4-1.4L10 13.6V3h2Zm-8 16h16v2H4v-2Z',
  chat: 'M4 3h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H8l-4 4V4a1 1 0 0 1 1-1Z',
  off: 'M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2Zm0 5v6m0 4h.01',
};

/* ---------- navigation ---------- */
let currentView = 'channels';
$$('.nav-btn').forEach((btn) => btn.addEventListener('click', () => {
  showView(btn.dataset.view);
  setNavDrawer(false);
}));

/* app nav drawer (narrow screens) */
function setNavDrawer(on) {
  document.body.classList.toggle('nav-open', on);
  $('#navToggle').setAttribute('aria-expanded', on);
}
$('#navToggle').addEventListener('click', () =>
  setNavDrawer(!document.body.classList.contains('nav-open')));
$('#navScrim').addEventListener('click', () => setNavDrawer(false));
document.addEventListener('keydown', (ev) => {
  if (ev.key === 'Escape' && document.body.classList.contains('nav-open') &&
      $('#playerOverlay').hidden) setNavDrawer(false);
});
function showView(name) {
  currentView = name;
  $$('.nav-btn').forEach((b) => b.classList.toggle('active', b.dataset.view === name));
  $$('.view').forEach((v) => { v.hidden = v.id !== `view-${name}`; });
  if (name === 'library') loadLibrary();
  if (name === 'downloads') refreshJobs();
  if (name === 'settings') loadSettings();
  renderState();
}

/* ---------- state / auth ---------- */
let state = { phase: 'unconfigured' };
let channelsLoaded = false;

async function refreshState() {
  try { state = await api.get('/api/state'); } catch { state = { phase: 'offline' }; }
  renderState();
}

function renderState() {
  const dot = $('#connStatus .dot');
  const txt = $('#connText');
  const phases = {
    ready: ['dot-on', state.user ? `Connected · ${state.user}` : 'Connected'],
    connecting: ['dot-wait', 'Connecting…'],
    need_phone: ['dot-wait', 'Waiting for sign-in'],
    need_code: ['dot-wait', 'Waiting for code'],
    need_password: ['dot-wait', 'Waiting for 2FA'],
    error: ['dot-off', 'Error — click to retry'],
    unconfigured: ['dot-off', 'Not configured'],
    offline: ['dot-off', 'Server offline'],
  };
  const [cls, label] = phases[state.phase] || phases.offline;
  dot.className = `dot ${cls}`;
  txt.textContent = label;

  const overlay = $('#authOverlay');
  const steps = {
    unconfigured: 'auth-credentials',
    connecting: 'auth-connecting',
    need_phone: 'auth-phone',
    need_code: 'auth-code',
    need_password: 'auth-password',
    error: 'auth-error',
  };
  // Library and Settings work offline; only Telegram-backed views force auth.
  const needsTelegram = currentView === 'channels' || currentView === 'downloads';
  const step = steps[state.phase];
  if (step && needsTelegram) {
    overlay.hidden = false;
    $$('.auth-step').forEach((el) => { el.hidden = el.id !== step; });
    if (step === 'auth-error') $('#authErrMsg').textContent = friendlyError(state.error || 'Unknown error.');
    const input = $(`#${step} input`);
    if (input && document.activeElement?.tagName !== 'INPUT') input.focus();
  } else {
    overlay.hidden = true;
  }

  if (state.phase === 'ready' && !channelsLoaded) {
    channelsLoaded = true;
    loadChannels();
  }
  if (state.phase !== 'ready' && state.phase !== 'connecting') channelsLoaded = false;
}

$('#connStatus').addEventListener('click', async () => {
  if (state.phase === 'error') {
    await api.send('POST', '/api/retry').catch(() => {});
    toast('Reconnecting…');
    refreshState();
  }
});

/* auth forms */
function showAuthError(form, msg) {
  const el = $('.auth-err', form);
  if (el) { el.textContent = friendlyError(msg); el.hidden = false; }
}
$$('#authOverlay form[data-step]').forEach((form) => {
  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const input = $('input', form);
    const btn = $('button[type=submit]', form);
    $('.auth-err', form).hidden = true;
    setBusy(btn, true, 'Sending…');
    try {
      await api.send('POST', `/api/auth/${form.dataset.step}`, { value: input.value });
      input.value = '';
    } catch (err) {
      showAuthError(form, err.message);
    }
    setBusy(btn, false);
    refreshState();
  });
});
$('#credForm').addEventListener('submit', async (ev) => {
  ev.preventDefault();
  const btn = $('button[type=submit]', ev.target);
  $('.auth-err', ev.target).hidden = true;
  setBusy(btn, true, 'Connecting…');
  try {
    await api.send('PUT', '/api/settings', {
      appId: $('#credAppId').value.trim(),
      appHash: $('#credAppHash').value.trim(),
    });
  } catch (err) {
    showAuthError(ev.target, err.message);
  }
  setBusy(btn, false);
  refreshState();
});
$('#authRetry').addEventListener('click', async () => { await api.send('POST', '/api/retry'); refreshState(); });
$('#authEditCreds').addEventListener('click', () => {
  $$('.auth-step').forEach((el) => { el.hidden = el.id !== 'auth-credentials'; });
});

/* ---------- channels ---------- */
let channels = [];
let activeChannel = null;
let videos = [];

const initials = (title) => title.split(/\s+/).slice(0, 2).map((w) => w[0] || '').join('').toUpperCase();
const hue = (id) => Number(BigInt(id) % 360n);

async function loadChannels() {
  const list = $('#chList');
  list.innerHTML = skeletonList(8, 'skel-ch');
  try {
    channels = await api.get('/api/channels');
    renderChannels();
  } catch (err) {
    list.innerHTML = emptyState(ICONS.off, 'Could not load channels', esc(friendlyError(err.message)));
  }
}
function renderChannels() {
  const q = $('#chSearch').value.toLowerCase();
  const list = $('#chList');
  const filtered = channels.filter((c) =>
    c.title.toLowerCase().includes(q) || (c.username || '').toLowerCase().includes(q));
  if (!filtered.length) {
    list.innerHTML = emptyState(ICONS.chat, q ? 'No matches' : 'No channels',
      q ? 'No channel matches that filter.' : 'Join channels on Telegram and hit Refresh.');
    return;
  }
  list.innerHTML = '';
  filtered.forEach((c) => {
    const btn = document.createElement('button');
    btn.className = 'ch-item' + (activeChannel?.id === c.id ? ' active' : '');
    btn.setAttribute('role', 'option');
    btn.setAttribute('aria-selected', activeChannel?.id === c.id);
    btn.innerHTML = `
      <span class="avatar" style="--h:${hue(c.id)}" aria-hidden="true">${esc(initials(c.title))}</span>
      <span class="ch-text">
        <span class="t">${esc(c.title)}</span>
        ${c.username ? `<span class="u">@${esc(c.username)}</span>` : ''}
      </span>`;
    btn.addEventListener('click', () => openChannel(c));
    list.appendChild(btn);
  });
}
$('#chSearch').addEventListener('input', renderChannels);
$('#chRefresh').addEventListener('click', async () => {
  setBusy($('#chRefresh'), true, 'Refreshing…');
  await loadChannels();
  setBusy($('#chRefresh'), false);
});

async function openChannel(c) {
  activeChannel = c;
  renderChannels();
  const panel = $('#vidPanel');
  panel.innerHTML = `
    <div class="vid-head"><h2>${esc(c.title)}</h2><span class="hint">scanning history…</span></div>
    ${skeletonList(6, 'skel-row')}`;
  try {
    videos = await api.get(`/api/videos?channel=${c.id}&limit=200`);
  } catch (err) {
    panel.innerHTML = `<div class="vid-head"><h2>${esc(c.title)}</h2></div>` +
      emptyState(ICONS.off, 'Could not scan this channel', esc(friendlyError(err.message)));
    return;
  }
  renderVideos();
}

function renderVideos() {
  const panel = $('#vidPanel');
  if (!videos.length) {
    panel.innerHTML = `<div class="vid-head"><h2>${esc(activeChannel.title)}</h2></div>` +
      emptyState(ICONS.play, 'No videos here',
        'Nothing video-shaped in the recent history of this channel.');
    return;
  }
  const chips = { done: ['chip-done', 'Downloaded'], partial: ['chip-partial', 'Partial'], none: ['chip-none', '—'] };
  const rows = videos.map((v, i) => {
    const [cls, label] = chips[v.status] || chips.none;
    const playTitle = v.status === 'done' ? 'Play local file' : 'Watch now — streams from Telegram, no download';
    return `<tr data-row="${i}">
      <td><input type="checkbox" data-i="${i}" ${v.status === 'done' ? '' : 'checked'} aria-label="Select ${esc(v.name)}"></td>
      <td><button class="btn-play" data-play="${i}" title="${playTitle}" aria-label="Play ${esc(v.name)}">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5.5v13l11-6.5-11-6.5Z"/></svg>
      </button></td>
      <td class="vid-name" title="${esc(v.name)}">${esc(v.name)}</td>
      <td class="num">${fmtDur(v.duration)}</td>
      <td class="num">${fmtSize(v.size)}</td>
      <td class="num">${fmtDate(v.date)}</td>
      <td><span class="chip ${cls}">${label}</span></td>
    </tr>`;
  }).join('');
  panel.innerHTML = `
    <div class="vid-head">
      <h2>${esc(activeChannel.title)}</h2>
      <span class="hint">${videos.length} videos</span>
      <span class="spacer"></span>
      <button class="btn btn-primary btn-sm" id="dlSelected">Download</button>
    </div>
    <div class="table-scroll">
    <table class="vid-table">
      <thead><tr>
        <th><input type="checkbox" id="selHead" aria-label="Select all videos"></th>
        <th></th><th>File</th><th>Duration</th><th>Size</th><th>Date</th><th>Status</th>
      </tr></thead>
      <tbody>${rows}</tbody>
    </table>
    </div>`;

  const boxes = () => $$('#vidPanel tbody input[type=checkbox]');
  const updateSelection = () => {
    const sel = boxes().filter((c) => c.checked);
    const bytes = sel.reduce((sum, c) => sum + (videos[+c.dataset.i].size || 0), 0);
    const btn = $('#dlSelected');
    btn.textContent = sel.length ? `Download ${sel.length} (${fmtSize(bytes)})` : 'Download';
    btn.disabled = !sel.length;
    const head = $('#selHead');
    head.checked = sel.length === boxes().length;
    head.indeterminate = sel.length > 0 && sel.length < boxes().length;
  };
  updateSelection();

  $('#selHead').addEventListener('change', (ev) => {
    boxes().forEach((c) => { c.checked = ev.target.checked; });
    updateSelection();
  });
  $('#vidPanel tbody').addEventListener('change', updateSelection);
  // Row click toggles selection (except on the play button / checkbox).
  $$('#vidPanel tbody tr').forEach((tr) => tr.addEventListener('click', (ev) => {
    if (ev.target.closest('button, input')) return;
    const box = $('input[type=checkbox]', tr);
    box.checked = !box.checked;
    updateSelection();
  }));
  $$('#vidPanel [data-play]').forEach((btn) => btn.addEventListener('click', () => {
    const queue = videos.map((v) => ({
      name: v.name,
      size: v.size,
      duration: v.duration,
      path: v.mediaPath || `/stream/${activeChannel.id}/${v.msgId}`,
      streamed: !v.mediaPath,
    }));
    openPlayer(queue, +btn.dataset.play);
  }));
  $('#dlSelected').addEventListener('click', async () => {
    const picked = boxes().filter((c) => c.checked).map((c) => videos[+c.dataset.i]);
    if (!picked.length) return;
    const btn = $('#dlSelected');
    setBusy(btn, true, 'Queueing…');
    try {
      const res = await api.send('POST', '/api/download', {
        channelId: activeChannel.id, videos: picked,
      });
      toast(`${res.queued} ${res.queued === 1 ? 'video' : 'videos'} queued`, 'ok');
      showView('downloads');
    } catch (err) {
      toast(friendlyError(err.message), 'err');
    }
    setBusy(btn, false);
  });
}

/* ---------- downloads ---------- */
async function refreshJobs() {
  let jobs = [];
  try { jobs = await api.get('/api/downloads'); } catch { return; }
  const running = jobs.filter((j) => j.state === 'running');
  const queued = jobs.filter((j) => j.state === 'queued');
  const finished = jobs.length - running.length - queued.length;
  const badge = $('#dlBadge');
  badge.hidden = running.length + queued.length === 0;
  badge.textContent = running.length + queued.length;

  if (currentView !== 'downloads') return;

  const speed = running.reduce((s, j) => s + (j.speed || 0), 0);
  $('#dlSummary').textContent = jobs.length
    ? [
        running.length && `${running.length} downloading`,
        speed && `${fmtSize(speed)}/s`,
        queued.length && `${queued.length} queued`,
        finished && `${finished} finished`,
      ].filter(Boolean).join(' · ')
    : '';
  $('#dlClear').hidden = finished === 0;

  const list = $('#jobList');
  if (!jobs.length) {
    list.innerHTML = emptyState(ICONS.down, 'No downloads yet',
      'Pick videos in Channels and they show up here with live progress.');
    return;
  }
  list.innerHTML = jobs.map((j) => {
    const pct = j.size ? Math.min(100, (j.done / j.size) * 100) : 0;
    const stateLabel = { queued: 'Queued', running: 'Downloading', done: 'Complete', error: 'Failed', canceled: 'Canceled' }[j.state];
    const cancelable = j.state === 'queued' || j.state === 'running';
    return `<div class="job state-${j.state}">
      <div class="job-top">
        <span class="name">${esc(j.name)}</span>
        <span class="ch">${esc(j.channel)}</span>
        <span class="right">
          <span class="chip chip-${j.state}">${stateLabel}</span>
          ${cancelable ? `<button class="btn btn-ghost btn-sm" data-cancel="${j.id}">Cancel</button>` : ''}
        </span>
      </div>
      <div class="progress"><i style="width:${j.state === 'done' ? 100 : pct}%"></i></div>
      <div class="job-meta">
        <span>${fmtSize(j.done)} / ${fmtSize(j.size)} (${pct.toFixed(0)}%)</span>
        ${j.state === 'running' ? `<span>${fmtSize(j.speed)}/s</span>` : ''}
      </div>
      ${j.error ? `<div class="err-msg">${esc(friendlyError(j.error))}</div>` : ''}
    </div>`;
  }).join('');
  $$('#jobList [data-cancel]').forEach((btn) =>
    btn.addEventListener('click', () => api.send('POST', `/api/downloads/${btn.dataset.cancel}/cancel`)));
}
$('#dlClear').addEventListener('click', async () => {
  await api.send('POST', '/api/downloads/clear').catch(() => {});
  refreshJobs();
});

/* ---------- library & player ---------- */
let library = [];
let playQueue = [];
let playIndex = -1;

async function loadLibrary() {
  try { library = await api.get('/api/library'); } catch { return; }
  renderLibrary();
}
function renderLibrary() {
  const list = $('#libList');
  const q = $('#libSearch').value.toLowerCase();
  const total = library.reduce((n, g) => n + g.videos.filter((v) => !v.partial).length, 0);
  $('#libSummary').textContent = total ? `${total} ${total === 1 ? 'video' : 'videos'}` : '';
  if (!library.length) {
    list.innerHTML = emptyState(ICONS.play, 'Your library is empty',
      'Videos you download land here, organized by channel — and play right in the app.');
    return;
  }
  list.innerHTML = '';
  let shown = 0;
  library.forEach((group) => {
    const playable = group.videos.filter((v) => !v.partial);
    const visible = q
      ? playable.filter((v) => cleanName(v.name).toLowerCase().includes(q))
      : playable;
    if (!visible.length) return;
    shown += visible.length;
    const bytes = playable.reduce((s, v) => s + v.size, 0);
    const section = document.createElement('div');
    section.className = 'lib-group';
    section.innerHTML = `<h2>${esc(group.channel || 'Downloads')}
      <span class="hint">· ${playable.length} ${playable.length === 1 ? 'video' : 'videos'} · ${fmtSize(bytes)}</span></h2>`;
    const grid = document.createElement('div');
    grid.className = 'lib-grid';
    visible.forEach((v) => {
      const pos = loadPos(v.path);
      const pct = pos && !pos.done && pos.d ? Math.min(100, (pos.t / pos.d) * 100) : 0;
      const card = document.createElement('button');
      card.className = 'lib-card';
      card.innerHTML = `
        <span class="lib-thumb">
          <video preload="metadata" muted src="${v.path}#t=1" tabindex="-1"></video>
          <span class="thumb-dur" hidden></span>
          ${pos?.done ? '<span class="thumb-watched" title="Watched">✓</span>' : ''}
          ${pct ? `<span class="thumb-progress" style="width:${pct}%"></span>` : ''}
          <span class="play" aria-hidden="true"><span>▶</span></span>
        </span>
        <span class="lib-info">
          <span class="n" title="${esc(v.name)}">${esc(cleanName(v.name))}</span>
          <span class="s">${fmtSize(v.size)}${pct ? ` · ${Math.round(pct)}% watched` : ''}</span>
        </span>`;
      const thumbVideo = $('video', card);
      thumbVideo.addEventListener('loadedmetadata', () => {
        const badge = $('.thumb-dur', card);
        badge.textContent = fmtDur(thumbVideo.duration);
        badge.hidden = false;
        saveDur(v.path, thumbVideo.duration);
      }, { once: true });
      card.setAttribute('aria-label', `Play ${v.name}`);
      card.addEventListener('click', () => openPlayer(playable, playable.indexOf(v)));
      grid.appendChild(card);
    });
    section.appendChild(grid);
    list.appendChild(section);
  });
  if (!shown) {
    list.innerHTML = emptyState(ICONS.play, 'No matches', 'No video matches that filter.');
  }
}
$('#libRefresh').addEventListener('click', loadLibrary);
$('#libSearch').addEventListener('input', renderLibrary);

function openPlayer(queue, index) {
  playQueue = queue;
  playIndex = index;
  $('#playerOverlay').hidden = false;
  setTheater(!!localStorage.getItem('ggtheater'));
  playCurrent();
}
function playCurrent() {
  const v = playQueue[playIndex];
  if (!v) return;
  const video = $('#playerVideo');
  video.src = v.path;
  // Resume where the viewer stopped (skipped for nearly-finished videos).
  const pos = loadPos(v.path);
  if (pos && !pos.done && pos.t > 10) {
    video.addEventListener('loadedmetadata', () => { video.currentTime = pos.t; }, { once: true });
  }
  video.playbackRate = +(localStorage.getItem('ggspeed') ?? 1);
  video.play().catch(() => {});
  $('#playerTitle').textContent = cleanName(v.name);
  $('#playerTitle').title = v.name;
  $('#playerSub').textContent = fmtSize(v.size) +
    (v.streamed ? ' · streaming from Telegram' : '') +
    (pos && !pos.done && pos.t > 10 ? ` · resuming at ${fmtDur(pos.t)}` : '');
  $('#playerCount').textContent = `${playIndex + 1} / ${playQueue.length}`;
  $('#prevBtn').disabled = playIndex <= 0;
  $('#nextBtn').disabled = playIndex >= playQueue.length - 1;
  const pl = $('#playlist');
  pl.innerHTML = `<div class="pl-head">Playlist <span class="hint">${playQueue.length} videos</span></div>`;
  playQueue.forEach((item, i) => {
    const watched = loadPos(item.path)?.done;
    const btn = document.createElement('button');
    btn.className = 'pl-item' + (i === playIndex ? ' active' : '') + (watched ? ' watched' : '');
    const marker = i === playIndex
      ? '<span class="eq" aria-label="Playing"><i></i><i></i><i></i></span>'
      : `<span class="idx">${i + 1}</span>`;
    const dur = getDur(item);
    btn.innerHTML = `
      ${marker}
      <span class="pl-text">
        <span class="n" title="${esc(item.name)}">${esc(cleanName(item.name))}</span>
        <span class="s">${dur ? `${fmtDur(dur)} · ` : ''}${fmtSize(item.size)}${watched && i !== playIndex ? ' · watched ✓' : ''}</span>
      </span>`;
    btn.addEventListener('click', () => { playIndex = i; playCurrent(); setDrawer(false); });
    pl.appendChild(btn);
    if (i === playIndex) btn.scrollIntoView({ block: 'nearest' });
  });
}
function stepPlayer(delta) {
  const next = playIndex + delta;
  if (next < 0 || next >= playQueue.length) return;
  playIndex = next;
  playCurrent();
}
function closePlayer() {
  const video = $('#playerVideo');
  const item = playQueue[playIndex];
  if (item && video.duration) savePos(item.path, video.currentTime, video.duration);
  video.pause();
  video.removeAttribute('src');
  video.load();
  setDrawer(false);
  $('#playerOverlay').hidden = true;
}
$('#prevBtn').addEventListener('click', () => stepPlayer(-1));
$('#nextBtn').addEventListener('click', () => stepPlayer(1));
$('#closePlayer').addEventListener('click', closePlayer);

const playerVideo = $('#playerVideo');
playerVideo.addEventListener('ended', () => {
  const item = playQueue[playIndex];
  if (item) savePos(item.path, playerVideo.duration || 0, playerVideo.duration || 1);
  if ($('#autoNext').checked) stepPlayer(1);
});
// Remember position every few seconds while watching.
let lastPosSave = 0;
playerVideo.addEventListener('timeupdate', () => {
  const now = Date.now();
  if (now - lastPosSave < 3000) return;
  lastPosSave = now;
  const item = playQueue[playIndex];
  if (item && playerVideo.duration) savePos(item.path, playerVideo.currentTime, playerVideo.duration);
});
playerVideo.addEventListener('waiting', () => { $('#buffering').hidden = false; });
['playing', 'canplay', 'seeked'].forEach((ev) =>
  playerVideo.addEventListener(ev, () => { $('#buffering').hidden = true; }));

/* ---------- custom video controls (YouTube-style) ---------- */
const wrap = $('#videoWrap');
const SPEEDS = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];
const fmtSpeed = (s) => (s === 1 ? '1×' : `${s}×`.replace('.', ','));

function togglePlay() { playerVideo.paused ? playerVideo.play() : playerVideo.pause(); }
function toggleFullscreen() {
  document.fullscreenElement ? document.exitFullscreen() : wrap.requestFullscreen();
}
async function togglePip() {
  try {
    if (document.pictureInPictureElement) await document.exitPictureInPicture();
    else await playerVideo.requestPictureInPicture();
  } catch { toast('Picture-in-picture not available', 'err'); }
}

/* play/pause button + icon state */
$('#vPlay').addEventListener('click', togglePlay);
playerVideo.addEventListener('click', togglePlay);
playerVideo.addEventListener('dblclick', toggleFullscreen);
function syncPlayIcon() {
  const paused = playerVideo.paused;
  $('#vPlay .i-play').hidden = !paused;
  $('#vPlay .i-pause').hidden = paused;
  wrap.classList.toggle('paused', paused);
}
playerVideo.addEventListener('play', syncPlayIcon);
playerVideo.addEventListener('pause', syncPlayIcon);

/* progress bar: played + buffered + scrubbing */
const prog = $('#vprog');
function syncProgress() {
  const d = playerVideo.duration || 0;
  const t = playerVideo.currentTime || 0;
  $('#vplayed').style.width = d ? `${(t / d) * 100}%` : '0%';
  const buf = playerVideo.buffered;
  let bufEnd = 0;
  for (let i = 0; i < buf.length; i++) {
    if (buf.start(i) <= t + 0.5 && buf.end(i) > bufEnd) bufEnd = buf.end(i);
  }
  $('#vbuf').style.width = d ? `${(bufEnd / d) * 100}%` : '0%';
  $('#vTime').textContent = `${fmtDur(t)} / ${fmtDur(d)}`;
}
['timeupdate', 'progress', 'loadedmetadata', 'seeked'].forEach((ev) =>
  playerVideo.addEventListener(ev, syncProgress));

function seekToEvent(ev) {
  const r = prog.getBoundingClientRect();
  const pct = Math.min(1, Math.max(0, (ev.clientX - r.left) / r.width));
  if (playerVideo.duration) playerVideo.currentTime = pct * playerVideo.duration;
}
prog.addEventListener('pointerdown', (ev) => {
  prog.setPointerCapture(ev.pointerId);
  seekToEvent(ev);
  const move = (e) => seekToEvent(e);
  const up = () => {
    prog.removeEventListener('pointermove', move);
    prog.removeEventListener('pointerup', up);
  };
  prog.addEventListener('pointermove', move);
  prog.addEventListener('pointerup', up);
});
prog.addEventListener('keydown', (ev) => {
  if (ev.key === 'ArrowRight') { playerVideo.currentTime += 5; ev.preventDefault(); }
  if (ev.key === 'ArrowLeft') { playerVideo.currentTime -= 5; ev.preventDefault(); }
});

/* volume (persisted) */
const vol = $('#vVol');
function syncVolume() {
  const muted = playerVideo.muted || playerVideo.volume === 0;
  $('#vMute .i-vol').hidden = muted;
  $('#vMute .i-muted').hidden = !muted;
  vol.value = playerVideo.muted ? 0 : playerVideo.volume;
}
vol.addEventListener('input', () => {
  playerVideo.volume = +vol.value;
  playerVideo.muted = +vol.value === 0;
  localStorage.setItem('ggvol', vol.value);
});
$('#vMute').addEventListener('click', () => { playerVideo.muted = !playerVideo.muted; });
playerVideo.addEventListener('volumechange', syncVolume);
playerVideo.volume = +(localStorage.getItem('ggvol') ?? 1);

/* speed menu (persisted, YouTube-style) */
const speedMenu = $('#vSpeedMenu');
speedMenu.innerHTML = SPEEDS.map((s) =>
  `<button role="menuitemradio" data-speed="${s}">${fmtSpeed(s)}</button>`).join('');
function applySpeed(s) {
  playerVideo.playbackRate = s;
  localStorage.setItem('ggspeed', s);
  $('#vSpeedBtn').textContent = fmtSpeed(s);
  $$('#vSpeedMenu [data-speed]').forEach((b) =>
    b.setAttribute('aria-checked', +b.dataset.speed === s));
}
$('#vSpeedBtn').addEventListener('click', (ev) => {
  ev.stopPropagation();
  speedMenu.hidden = !speedMenu.hidden;
});
speedMenu.addEventListener('click', (ev) => {
  const btn = ev.target.closest('[data-speed]');
  if (btn) { applySpeed(+btn.dataset.speed); speedMenu.hidden = true; }
});
document.addEventListener('click', () => { speedMenu.hidden = true; });

applySpeed(+(localStorage.getItem('ggspeed') ?? 1));

/* cinema (theater) mode: full-width video, playlist hidden */
function setTheater(on) {
  $('.player').classList.toggle('theater', on);
  $('#vTheater').setAttribute('aria-pressed', on);
  localStorage.setItem('ggtheater', on ? '1' : '');
}
$('#vTheater').addEventListener('click', () => setTheater(!$('.player').classList.contains('theater')));

/* portrait videos (9:16): drop the 16:9 stage and let the video stand tall */
playerVideo.addEventListener('loadedmetadata', () => {
  wrap.classList.toggle('portrait', playerVideo.videoHeight > playerVideo.videoWidth);
  const item = playQueue[playIndex];
  if (item) saveDur(item.path, playerVideo.duration);
});

/* playlist drawer (narrow screens; on wide+cinema it restores the sidebar) */
function setDrawer(on) {
  $('.player').classList.toggle('drawer-open', on);
  $('#plToggle').setAttribute('aria-expanded', on);
}
$('#plToggle').addEventListener('click', () => {
  const wide = matchMedia('(min-width: 901px)').matches;
  if (wide && $('.player').classList.contains('theater')) {
    setTheater(false);
    return;
  }
  setDrawer(!$('.player').classList.contains('drawer-open'));
});
$('#plScrim').addEventListener('click', () => setDrawer(false));

/* PiP + fullscreen buttons */
if (!document.pictureInPictureEnabled) $('#vPip').hidden = true;
$('#vPip').addEventListener('click', togglePip);
$('#vFull').addEventListener('click', toggleFullscreen);

/* auto-hide controls while playing */
let hideTimer = null;
function pokeControls() {
  wrap.classList.remove('idle');
  clearTimeout(hideTimer);
  hideTimer = setTimeout(() => {
    if (!playerVideo.paused && speedMenu.hidden) wrap.classList.add('idle');
  }, 2600);
}
['mousemove', 'pointerdown', 'touchstart'].forEach((ev) =>
  wrap.addEventListener(ev, pokeControls, { passive: true }));
playerVideo.addEventListener('pause', pokeControls);
playerVideo.addEventListener('play', pokeControls);

document.addEventListener('keydown', (ev) => {
  if ($('#playerOverlay').hidden || ev.target.tagName === 'INPUT') return;
  pokeControls();
  switch (ev.key) {
    case 'Escape':
      if ($('.player').classList.contains('drawer-open')) { setDrawer(false); break; }
      if (!speedMenu.hidden) { speedMenu.hidden = true; break; }
      closePlayer();
      break;
    case ' ':
      ev.preventDefault();
      togglePlay();
      break;
    case 'ArrowRight': playerVideo.currentTime += 10; break;
    case 'ArrowLeft': playerVideo.currentTime -= 10; break;
    case 'ArrowUp':
      ev.preventDefault();
      playerVideo.muted = false;
      playerVideo.volume = Math.min(1, playerVideo.volume + 0.1);
      break;
    case 'ArrowDown':
      ev.preventDefault();
      playerVideo.volume = Math.max(0, playerVideo.volume - 0.1);
      break;
    case 'm': case 'M': playerVideo.muted = !playerVideo.muted; break;
    case 'i': case 'I': togglePip(); break;
    case 't': case 'T':
      if (!ev.shiftKey) setTheater(!$('.player').classList.contains('theater'));
      break;
    case 'f': case 'F':
      if (!ev.shiftKey) toggleFullscreen();
      break;
    case 'N': if (ev.shiftKey) stepPlayer(1); break;
    case 'P': if (ev.shiftKey) stepPlayer(-1); break;
  }
});
$('#playerOverlay').addEventListener('click', (ev) => {
  if (ev.target === ev.currentTarget) closePlayer();
});

/* ---------- settings ---------- */
async function loadSettings() {
  try {
    const s = await api.get('/api/settings');
    $('#setAppId').value = s.appId || '';
    $('#setAppHash').value = s.appHash || '';
    $('#setDir').value = s.downloadsDir || '';
    $('#setParallel').value = s.parallelDownloads || 2;
  } catch { /* ignore */ }
}
$('#settingsForm').addEventListener('submit', async (ev) => {
  ev.preventDefault();
  const msg = $('#settingsMsg');
  const btn = $('button[type=submit]', ev.target);
  msg.className = 'form-msg';
  msg.textContent = '';
  setBusy(btn, true, 'Saving…');
  try {
    await api.send('PUT', '/api/settings', {
      appId: $('#setAppId').value.trim(),
      appHash: $('#setAppHash').value.trim(),
      downloadsDir: $('#setDir').value.trim(),
      parallelDownloads: +$('#setParallel').value || 2,
    });
    msg.textContent = 'Saved. Reconnecting…';
    msg.classList.add('ok');
    refreshState();
  } catch (err) {
    msg.textContent = err.message;
    msg.classList.add('err');
  }
  setBusy(btn, false);
});

// Two-step logout: first click arms, second confirms.
const logoutBtn = $('#logoutBtn');
let logoutArmed = null;
logoutBtn.addEventListener('click', async () => {
  if (!logoutArmed) {
    logoutBtn.textContent = 'Click again to confirm';
    logoutBtn.classList.add('btn-danger');
    logoutArmed = setTimeout(() => {
      logoutBtn.textContent = 'Log out of Telegram';
      logoutBtn.classList.remove('btn-danger');
      logoutArmed = null;
    }, 3000);
    return;
  }
  clearTimeout(logoutArmed);
  logoutArmed = null;
  logoutBtn.textContent = 'Log out of Telegram';
  logoutBtn.classList.remove('btn-danger');
  await api.send('POST', '/api/logout');
  toast('Logged out of Telegram');
  refreshState();
});

// Quit (two-step, like logout): stops the local server / the .app.
const quitBtn = $('#quitBtn');
let quitArmed = null;
quitBtn.addEventListener('click', async () => {
  if (!quitArmed) {
    quitBtn.textContent = 'Click again to quit';
    quitBtn.classList.add('btn-danger');
    quitArmed = setTimeout(() => {
      quitBtn.textContent = 'Quit Studio';
      quitBtn.classList.remove('btn-danger');
      quitArmed = null;
    }, 3000);
    return;
  }
  clearTimeout(quitArmed);
  await api.send('POST', '/api/quit').catch(() => {});
  document.body.innerHTML = '<div class="empty" style="padding-top:20vh"><h3>GramGrabber Studio stopped</h3><p>You can close this tab.</p></div>';
});

/* ---------- boot & polling ---------- */
const initialView = new URLSearchParams(location.search).get('view');
if (initialView && $(`#view-${initialView}`)) showView(initialView);
$('#chList').innerHTML = skeletonList(8, 'skel-ch');
refreshState();
setInterval(refreshState, 1500);
setInterval(refreshJobs, 1000);

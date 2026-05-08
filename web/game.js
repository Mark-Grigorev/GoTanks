'use strict';

const tg = window.Telegram?.WebApp;
if (tg) tg.ready();

// ── Constants ──────────────────────────────────────────────────────────────
const TILE_EMPTY = 0, TILE_WALL = 1, TILE_BRICK = 2;
const TILE_COLORS = { [TILE_EMPTY]: '#2a2a3a', [TILE_WALL]: '#555566', [TILE_BRICK]: '#7a3b10' };
const PLAYER_COLORS = ['#4caf50','#f44336','#2196f3','#ff9800','#e040fb','#00bcd4','#ffeb3b'];
const DIR_DEG = { 0: 0, 1: 90, 2: 180, 3: 270 };
const TICK_MS = 50; // server 20 TPS

// ── State ──────────────────────────────────────────────────────────────────
const S = {
  token: null, ws: null,
  tanks: [], maps: [],
  selectedTank: null,
  myPlayerID: null, roomID: null,
  maxPlayers: 4, playerCount: 0,
  map: null, players: [], kills: 0,
};

// interpolation: two latest snapshots + timestamps
const interp = { prev: null, curr: null, prevTime: 0, currTime: 0 };
let rafId = null;

function lerp(a, b, t) { return a + (b - a) * t; }

// ── Sprites ────────────────────────────────────────────────────────────────
const spriteCache = {};

function loadSprites(tankCfg) {
  if (spriteCache[tankCfg.id]) return Promise.resolve();
  spriteCache[tankCfg.id] = { hull: null, gun: null };
  const promises = ['hull', 'gun'].map(layer => {
    const path = tankCfg[layer];
    if (!path) return Promise.resolve();
    return new Promise(res => {
      const img = new Image();
      img.onload  = () => { spriteCache[tankCfg.id][layer] = img; res(); };
      img.onerror = () => res();
      img.src = `/sprites/${path}`;
    });
  });
  return Promise.all(promises);
}

function getSprites(tankType) { return spriteCache[tankType] || null; }

// ── Screen helpers ─────────────────────────────────────────────────────────
function showScreen(id) {
  document.querySelectorAll('.screen').forEach(s => s.classList.remove('active'));
  document.getElementById(id).classList.add('active');
}

// ── Boot ───────────────────────────────────────────────────────────────────
async function boot() {
  try {
    await doAuth();
    await Promise.all([fetchTanks(), fetchMaps()]);
    renderTankCards();
    await renderRoomList();
    showScreen('screen-lobby');
  } catch (e) {
    document.body.innerHTML = `<div style="padding:24px;color:#f44">${e.message}</div>`;
  }
}

async function doAuth() {
  const initData = tg?.initData || '';
  if (!initData) throw new Error('Открой приложение через Telegram-бота, а не напрямую по ссылке.');
  const r = await fetch('/api/auth', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ init_data: initData }),
  });
  if (!r.ok) throw new Error('Ошибка авторизации: ' + (await r.text()));
  S.token = (await r.json()).token;
}

async function fetchTanks() {
  const r = await fetch('/api/tanks');
  S.tanks = await r.json();
  if (S.tanks.length) {
    S.selectedTank = S.tanks[0].id;
    await Promise.all(S.tanks.map(loadSprites));
  }
}

async function fetchMaps() {
  const r = await fetch('/api/maps');
  S.maps = await r.json();
}

async function fetchRooms() {
  return (await fetch('/api/rooms')).json();
}

// ── Lobby ──────────────────────────────────────────────────────────────────
function renderTankCards() {
  const el = document.getElementById('tank-list');
  el.innerHTML = '';
  S.tanks.forEach(t => {
    const card = document.createElement('div');
    card.className = 'tank-card' + (t.id === S.selectedTank ? ' selected' : '');
    card.innerHTML = `
      <div class="tank-preview">
        <img class="preview-hull" src="/sprites/${t.hull}" onerror="this.style.display='none'">
        <img class="preview-gun"  src="/sprites/${t.gun}"  onerror="this.style.display='none'">
      </div>
      <div class="name">${t.name}</div>
      <div class="stat">❤️ ${t.hp}</div>
      <div class="stat">💨 ${t.speed}</div>
      <div class="stat">💥 ${t.bullet_damage}</div>`;
    card.onclick = () => {
      S.selectedTank = t.id;
      document.querySelectorAll('.tank-card').forEach(c => c.classList.remove('selected'));
      card.classList.add('selected');
    };
    el.appendChild(card);
  });
}

async function renderRoomList() {
  const el = document.getElementById('room-list');
  el.innerHTML = '<span style="color:#666;font-size:.85rem">Загрузка...</span>';
  try {
    const rooms = await fetchRooms();
    el.innerHTML = '';
    if (!rooms?.length) {
      el.innerHTML = '<span style="color:#666;font-size:.85rem">Нет открытых комнат</span>';
      return;
    }
    rooms.forEach(room => {
      const div = document.createElement('div');
      div.className = 'room-item';
      div.innerHTML = `<span class="room-info">🗺 ${room.map_id}</span>
                       <span class="room-count">${room.player_count}/${room.max_players}</span>`;
      div.onclick = () => connectAndJoin(room.id);
      el.appendChild(div);
    });
  } catch {
    el.innerHTML = '<span style="color:#f44">Ошибка загрузки</span>';
  }
}

document.getElementById('btn-refresh').onclick = renderRoomList;
document.getElementById('btn-create').onclick  = () => connectAndCreate();
document.getElementById('btn-start').onclick   = () => sendWS({ type: 'start_game' });
document.getElementById('btn-leave').onclick   = leaveRoom;
document.getElementById('btn-quit').onclick    = leaveRoom;
document.getElementById('btn-back').onclick    = () => { renderRoomList(); showScreen('screen-lobby'); };

// ── WebSocket ──────────────────────────────────────────────────────────────
function openWS() {
  return new Promise((res, rej) => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${location.host}/ws?token=${S.token}`);
    ws.onopen  = () => res(ws);
    ws.onerror = () => rej(new Error('WebSocket: не удалось подключиться'));
  });
}

async function connectAndCreate() {
  const ws = await openWS();
  S.ws = ws; setupWS(ws);
  sendWS({ type: 'create_room', map_id: S.maps[0]?.id || 'arena', tank_type: S.selectedTank });
}

async function connectAndJoin(roomID) {
  const ws = await openWS();
  S.ws = ws; setupWS(ws);
  sendWS({ type: 'join_room', room_id: roomID, tank_type: S.selectedTank });
}

function setupWS(ws) {
  ws.onmessage = ev => { try { handleMsg(JSON.parse(ev.data)); } catch {} };
  ws.onclose   = () => { S.ws = null; };
}

function sendWS(obj) {
  if (S.ws?.readyState === WebSocket.OPEN) S.ws.send(JSON.stringify(obj));
}

function leaveRoom() {
  stopRenderLoop();
  resetTouchMove();
  sendWS({ type: 'leave_room' });
  S.ws?.close(); S.ws = null;
  S.roomID = S.myPlayerID = null;
  renderRoomList(); showScreen('screen-lobby');
}

// ── Server messages ────────────────────────────────────────────────────────
function handleMsg(msg) {
  const p = msg.payload;
  switch (msg.type) {
    case 'room_created': S.roomID = p.room_id; break;
    case 'room_joined':
      S.myPlayerID  = p.player_id;
      S.roomID      = p.room_id;
      S.playerCount = p.player_count;
      S.maxPlayers  = p.max_players;
      refreshWaitScreen(); showScreen('screen-waiting'); break;
    case 'player_joined':
    case 'player_left':
      S.playerCount = p.player_count; refreshWaitScreen(); break;
    case 'game_start':
      S.map = p.map; S.players = p.players; S.kills = 0;
      initGame(); break;
    case 'state':
      // tile changes applied immediately — S.map is the source of truth for rendering
      p.tile_changes?.forEach(ch => { S.map.tiles[ch.y][ch.x] = ch.t; });
      // kills counted once per incoming state
      p.kills?.forEach(k => { if (k.killer_id === S.myPlayerID) S.kills++; });
      // HUD update
      updateHUD(p);
      // advance interpolation window
      interp.prev     = interp.curr;
      interp.prevTime = interp.currTime;
      interp.curr     = p;
      interp.currTime = performance.now();
      break;
    case 'game_over':
      endGame(p.winner_id); break;
    case 'error':
      console.warn('server:', p?.message); break;
  }
}

function updateHUD(snap) {
  const me = snap.tanks?.find(t => t.id === S.myPlayerID);
  if (me) {
    const cfg = S.tanks.find(tc => tc.id === me.type);
    document.getElementById('hud-hp').textContent = `HP ${me.hp}/${cfg?.hp ?? '?'}`;
  }
  document.getElementById('hud-kills').textContent = `☠ ${S.kills}`;
}

function refreshWaitScreen() {
  document.getElementById('wait-count').textContent =
    `Игроков: ${S.playerCount} / ${S.maxPlayers}`;
}

// ── Game init ──────────────────────────────────────────────────────────────
const canvas = document.getElementById('game-canvas');
const ctx    = canvas.getContext('2d');

let tileSize = 32;

function initGame() {
  calcTileSize();
  window.addEventListener('resize', calcTileSize);
  interp.prev = null; interp.curr = null;
  S.kills = 0;
  startRenderLoop();
  showScreen('screen-game');
}

function calcTileSize() {
  if (!S.map) return;
  const vw = window.innerWidth;
  const vh = window.innerHeight - 130;
  tileSize = Math.max(8, Math.min(
    Math.floor(vw / S.map.width),
    Math.floor(vh / S.map.height)
  ));
  canvas.width  = tileSize * S.map.width;
  canvas.height = tileSize * S.map.height;
}

// ── RAF render loop ────────────────────────────────────────────────────────
function startRenderLoop() {
  if (rafId) return;
  function loop(now) {
    rafId = requestAnimationFrame(loop);
    if (!interp.curr || !S.map) return;
    // alpha: 0 = at prev snapshot position, 1 = at curr snapshot position
    const alpha = interp.prev
      ? Math.min(1, (now - interp.currTime) / TICK_MS)
      : 1;
    renderFrame(alpha);
  }
  rafId = requestAnimationFrame(loop);
}

function stopRenderLoop() {
  if (rafId) { cancelAnimationFrame(rafId); rafId = null; }
}

// ── Render ─────────────────────────────────────────────────────────────────
function renderFrame(alpha) {
  const snap = interp.curr;
  if (!snap || !S.map) return;
  const T = tileSize;

  // Map (S.map.tiles already has tile_changes applied)
  for (let y = 0; y < S.map.height; y++)
    for (let x = 0; x < S.map.width; x++) {
      ctx.fillStyle = TILE_COLORS[S.map.tiles[y][x]] ?? TILE_COLORS[0];
      ctx.fillRect(x * T, y * T, T, T);
    }

  // Bullets (no interpolation — fast, barely visible between ticks)
  ctx.fillStyle = '#ffe';
  snap.bullets?.forEach(b => {
    ctx.beginPath();
    ctx.arc(b.x * T + T/2, b.y * T + T/2, Math.max(2, T * 0.1), 0, Math.PI * 2);
    ctx.fill();
  });

  // Tanks with position interpolation
  snap.tanks?.forEach((t, i) => {
    if (t.hp <= 0) return;

    const prev = interp.prev?.tanks?.find(p => p.id === t.id);
    const rx = prev ? lerp(prev.x, t.x, alpha) : t.x;
    const ry = prev ? lerp(prev.y, t.y, alpha) : t.y;

    const cx = rx * T + T / 2;
    const cy = ry * T + T / 2;
    const sp = getSprites(t.type);
    const isMe = t.id === S.myPlayerID;
    const TS = T * 0.55;

    ctx.save();
    ctx.translate(cx, cy);
    ctx.rotate((DIR_DEG[t.dir] ?? 0) * Math.PI / 180);

    if (sp?.hull) {
      ctx.drawImage(sp.hull, -TS/2, -TS/2, TS, TS);
    } else {
      ctx.fillStyle = isMe ? '#ffe082' : PLAYER_COLORS[i % PLAYER_COLORS.length];
      ctx.fillRect(-TS/2, -TS/2, TS, TS);
    }
    if (sp?.gun) {
      ctx.drawImage(sp.gun, -TS/2, -TS/2, TS, TS);
    } else {
      ctx.fillStyle = isMe ? '#ffc107' : '#bbb';
      ctx.fillRect(-TS * 0.12, -TS/2, TS * 0.24, TS * 0.4);
    }
    if (isMe) {
      ctx.strokeStyle = 'rgba(255,255,255,0.7)';
      ctx.lineWidth = 1.5;
      ctx.strokeRect(-TS/2, -TS/2, TS, TS);
    }

    ctx.restore();

    // HP bar follows interpolated position
    const cfg = S.tanks.find(tc => tc.id === t.type);
    const maxHp = cfg?.hp || 1;
    const barW = T - 4;
    ctx.fillStyle = '#222';
    ctx.fillRect(rx * T + 2, ry * T - 5, barW, 3);
    ctx.fillStyle = t.hp / maxHp > 0.5 ? '#4caf50' : '#f44336';
    ctx.fillRect(rx * T + 2, ry * T - 5, barW * (t.hp / maxHp), 3);
  });
}

function endGame(winnerID) {
  stopRenderLoop();
  resetTouchMove();
  const win  = winnerID === S.myPlayerID;
  const draw = winnerID === 'draw';
  document.getElementById('go-title').textContent = draw ? '🤝 Ничья' : win ? '🏆 Победа!' : '💀 Поражение';
  document.getElementById('go-sub').textContent   = draw ? '' : win ? 'Ты лучший!' : 'В следующий раз повезёт';
  showScreen('screen-gameover');
}

// ── Input ──────────────────────────────────────────────────────────────────
const inp = { move: 'none', shoot: false };
let inputTimer = null;

function flushInput() {
  sendWS({ type: 'input', input: { move: inp.move, shoot: inp.shoot } });
  inp.shoot = false;
}

function startMove(dir) {
  inp.move = dir;
  flushInput();
  if (inputTimer) clearInterval(inputTimer);
  inputTimer = setInterval(flushInput, 50);
}

function stopMove() {
  inp.move = 'none';
  if (inputTimer) { clearInterval(inputTimer); inputTimer = null; }
  flushInput();
}

function resetTouchMove() { stopMove(); }

// D-pad: move while pressed, stop on release
document.querySelectorAll('[data-dir]').forEach(btn => {
  btn.addEventListener('pointerdown', e => { e.preventDefault(); startMove(btn.dataset.dir); });
  ['pointerup', 'pointerleave', 'pointercancel'].forEach(ev =>
    btn.addEventListener(ev, e => { e.preventDefault(); stopMove(); })
  );
});

// Shoot button
document.getElementById('btn-shoot').addEventListener('pointerdown', e => {
  e.preventDefault();
  inp.shoot = true;
  flushInput();
});

// Keyboard: hold to move (standard PC behavior)
const KEY_MAP = {
  ArrowUp:'up', ArrowDown:'down', ArrowLeft:'left', ArrowRight:'right',
  KeyW:'up', KeyS:'down', KeyA:'left', KeyD:'right',
};
const keysHeld = new Set();

window.addEventListener('keydown', e => {
  if (e.code === 'Space') { e.preventDefault(); inp.shoot = true; flushInput(); return; }
  const dir = KEY_MAP[e.code];
  if (!dir || keysHeld.has(e.code)) return;
  e.preventDefault();
  keysHeld.add(e.code);
  startMove(dir);
});

window.addEventListener('keyup', e => {
  keysHeld.delete(e.code);
  const remaining = [...keysHeld].find(k => KEY_MAP[k]);
  remaining ? startMove(KEY_MAP[remaining]) : stopMove();
});

boot();

'use strict';

const tg = window.Telegram?.WebApp;
if (tg) tg.ready();

// ── Constants ──────────────────────────────────────────────────────────────
const TILE_EMPTY = 0, TILE_WALL = 1, TILE_BRICK = 2;
const DIR_DEG = { 0: 0, 1: 90, 2: 180, 3: 270 };
const TICK_MS = 50;

// 90s NES-style palette per player slot
const PLAYER_COLORS = ['#c8c800', '#00b432', '#c82000', '#0050c8'];

// Visual parameters per tank type (fractions of tile size)
// tw=trackWidth, bw=bodyWidth, turW=turretWidth, gunW=gunWidth, gunL=gunLength
const TANK_DEFS = {
  scout:  { tw: 0.13, bw: 0.68, turW: 0.28, gunW: 0.08, gunL: 0.50 },
  light:  { tw: 0.14, bw: 0.70, turW: 0.30, gunW: 0.09, gunL: 0.46 },
  medium: { tw: 0.15, bw: 0.72, turW: 0.32, gunW: 0.10, gunL: 0.44 },
  rapid:  { tw: 0.14, bw: 0.70, turW: 0.28, gunW: 0.08, gunL: 0.48 },
  sniper: { tw: 0.13, bw: 0.68, turW: 0.26, gunW: 0.07, gunL: 0.52 },
  heavy:  { tw: 0.17, bw: 0.74, turW: 0.36, gunW: 0.13, gunL: 0.36 },
  siege:  { tw: 0.19, bw: 0.76, turW: 0.40, gunW: 0.15, gunL: 0.32 },
};

// ── Color helpers ──────────────────────────────────────────────────────────
function hexMult(hex, f) {
  const r = Math.round(parseInt(hex.slice(1,3), 16) * f);
  const g = Math.round(parseInt(hex.slice(3,5), 16) * f);
  const b = Math.round(parseInt(hex.slice(5,7), 16) * f);
  return `rgb(${Math.min(255,r)},${Math.min(255,g)},${Math.min(255,b)})`;
}
function hexAdd(hex, a) {
  const r = Math.min(255, parseInt(hex.slice(1,3), 16) + a);
  const g = Math.min(255, parseInt(hex.slice(3,5), 16) + a);
  const b = Math.min(255, parseInt(hex.slice(5,7), 16) + a);
  return `rgb(${r},${g},${b})`;
}

// ── Tile rendering (pixel art) ─────────────────────────────────────────────
function drawTile(ctx, type, px, py, T) {
  switch (type) {
    case TILE_EMPTY: {
      ctx.fillStyle = '#0c0c10';
      ctx.fillRect(px, py, T, T);
      // subtle dot grid texture
      if (T >= 12) {
        ctx.fillStyle = '#18181f';
        const step = Math.max(4, Math.round(T / 4));
        for (let dy = step >> 1; dy < T; dy += step)
          for (let dx = step >> 1; dx < T; dx += step)
            ctx.fillRect(px + dx, py + dy, 1, 1);
      }
      break;
    }
    case TILE_BRICK: {
      const m = Math.max(1, Math.round(T * 0.07)); // mortar
      const bw = Math.floor((T - 3 * m) / 2);
      const bh = Math.floor((T - 3 * m) / 2);
      ctx.fillStyle = '#4a1200'; // mortar
      ctx.fillRect(px, py, T, T);
      ctx.fillStyle = '#b03010';
      // 4 bricks (2×2)
      const xs = [px + m, px + m + bw + m];
      const ys = [py + m, py + m + bh + m];
      for (const bx of xs) for (const by of ys) {
        ctx.fillRect(bx, by, bw, bh);
        // highlight
        ctx.fillStyle = '#d84020';
        ctx.fillRect(bx, by, bw, 1);
        ctx.fillRect(bx, by, 1, bh);
        ctx.fillStyle = '#781800';
        ctx.fillRect(bx + bw - 1, by, 1, bh);
        ctx.fillRect(bx, by + bh - 1, bw, 1);
        ctx.fillStyle = '#b03010';
      }
      break;
    }
    case TILE_WALL: {
      const h2 = Math.floor(T / 2);
      const pad = Math.max(1, Math.round(T * 0.07));
      ctx.fillStyle = '#303030';
      ctx.fillRect(px, py, T, T);
      const panels = [
        [px + pad,      py + pad,      h2 - pad - 1, h2 - pad - 1],
        [px + h2 + 1,   py + pad,      h2 - pad - 1, h2 - pad - 1],
        [px + pad,      py + h2 + 1,   h2 - pad - 1, h2 - pad - 1],
        [px + h2 + 1,   py + h2 + 1,   h2 - pad - 1, h2 - pad - 1],
      ];
      for (const [x, y, w, h] of panels) {
        if (w <= 0 || h <= 0) continue;
        ctx.fillStyle = '#707080';
        ctx.fillRect(x, y, w, h);
        ctx.fillStyle = '#9090a0';
        ctx.fillRect(x, y, w, 1);
        ctx.fillRect(x, y, 1, h);
        ctx.fillStyle = '#505060';
        ctx.fillRect(x + w - 1, y, 1, h);
        ctx.fillRect(x, y + h - 1, w, 1);
      }
      break;
    }
    default:
      ctx.fillStyle = '#0c0c10';
      ctx.fillRect(px, py, T, T);
  }
}

// ── Tank rendering (pixel art, centered at 0,0, facing UP) ────────────────
function drawTankPixel(ctx, tankType, color, isMe, T) {
  const def = TANK_DEFS[tankType] || TANK_DEFS.medium;
  const h = T; // half extents: ±h/2

  const dark1 = hexMult(color, 0.45);  // tracks (darkest)
  const dark2 = hexMult(color, 0.65);  // track ticks
  const body  = color;
  const turret = hexMult(color, 0.78); // turret slightly darker
  const gun   = hexMult(color, 0.55);
  const lite  = hexAdd(color, 50);

  const tw  = h * def.tw;
  const bw  = h * def.bw;
  const turW = h * def.turW;
  const gunW = h * def.gunW;
  const gunL = h * def.gunL;
  const turOff = -h * 0.08; // turret shifted toward front

  // ── Tracks ──
  ctx.fillStyle = dark1;
  ctx.fillRect(-h/2, -h/2, tw, h);          // left
  ctx.fillRect( h/2 - tw, -h/2, tw, h);     // right

  // track link ticks
  ctx.fillStyle = dark2;
  const segs = 5;
  for (let i = 1; i < segs; i++) {
    const ty = -h/2 + i * (h / segs);
    ctx.fillRect(-h/2,      ty, tw, 1);
    ctx.fillRect( h/2 - tw, ty, tw, 1);
  }

  // ── Body ──
  ctx.fillStyle = body;
  ctx.fillRect(-bw/2, -h/2, bw, h);
  // left body highlight
  ctx.fillStyle = lite;
  ctx.fillRect(-bw/2, -h/2, 1, h);
  // top highlight
  ctx.fillRect(-bw/2, -h/2, bw, 1);

  // ── Gun (drawn before turret so turret overlaps its base) ──
  ctx.fillStyle = gun;
  ctx.fillRect(-gunW/2, -h/2, gunW, gunL);

  // ── Turret ──
  ctx.fillStyle = turret;
  ctx.fillRect(-turW/2, turOff - turW/2, turW, turW);
  ctx.fillStyle = hexAdd(color, 20);
  ctx.fillRect(-turW/2, turOff - turW/2, turW, 1);
  ctx.fillRect(-turW/2, turOff - turW/2, 1, turW);

  // ── "Me" star marker (center of body) ──
  if (isMe) {
    const sc = Math.max(1, Math.round(h * 0.08));
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(-sc/2, h * 0.15 - sc,     sc, sc * 2);
    ctx.fillRect(-sc,   h * 0.15 - sc/2,   sc * 2, sc);
  }
}

// ── Power-up rendering ────────────────────────────────────────────────────
function drawPowerUp(ctx, type, px, py, T, now) {
  const pulse = 0.86 + 0.14 * Math.sin(now * 0.005);
  const r = T * 0.36 * pulse;
  const cx = px + T / 2, cy = py + T / 2;

  // Subtle tinted background
  const bgColors = {
    damage: 'rgba(255,80,0,0.18)',
    armor:  'rgba(30,100,255,0.18)',
    speed:  'rgba(255,220,0,0.18)',
    heal:   'rgba(0,180,40,0.18)',
  };
  ctx.fillStyle = bgColors[type] || 'rgba(255,255,255,0.1)';
  ctx.fillRect(px, py, T, T);

  ctx.save();
  ctx.translate(cx, cy);

  switch (type) {
    case 'damage': {
      // Orange burst: cross + diagonal cross + bright center
      const w = r * 0.28;
      ctx.fillStyle = '#c84000';
      ctx.fillRect(-w, -r, w * 2, r * 2);
      ctx.fillRect(-r, -w, r * 2, w * 2);
      ctx.save(); ctx.rotate(Math.PI / 4);
      ctx.fillStyle = '#ff6800';
      ctx.fillRect(-w * 0.8, -r * 0.85, w * 1.6, r * 1.7);
      ctx.fillRect(-r * 0.85, -w * 0.8, r * 1.7, w * 1.6);
      ctx.restore();
      ctx.fillStyle = '#ffcc00';
      ctx.fillRect(-w * 0.9, -w * 0.9, w * 1.8, w * 1.8);
      break;
    }
    case 'armor': {
      // Shield: wide top + narrowing bottom
      const sw = r * 0.82, sh = r * 0.9;
      ctx.fillStyle = '#1050d0';
      ctx.fillRect(-sw, -sh, sw * 2, sh * 1.1);
      ctx.fillRect(-sw * 0.6, sh * 0.1, sw * 1.2, sh * 0.5);
      ctx.fillRect(-sw * 0.25, sh * 0.6, sw * 0.5, sh * 0.4);
      // Highlight stripe
      ctx.fillStyle = '#5090ff';
      ctx.fillRect(-sw * 0.45, -sh * 0.8, sw * 0.3, sh * 0.85);
      break;
    }
    case 'speed': {
      // Double chevron →
      const aw = r * 0.52, ah = r * 0.78;
      ctx.fillStyle = '#c09000';
      for (let i = 0; i < 2; i++) {
        const ox = -r * 0.35 + i * r * 0.42;
        ctx.fillRect(ox, -ah * 0.14, aw * 0.5, ah * 0.28);
        ctx.save(); ctx.translate(ox + aw * 0.5, 0);
        ctx.fillStyle = i === 1 ? '#ffee40' : '#e8c000';
        ctx.beginPath();
        ctx.moveTo(0, -ah / 2); ctx.lineTo(aw * 0.55, 0); ctx.lineTo(0, ah / 2);
        ctx.closePath(); ctx.fill();
        ctx.restore();
        ctx.fillStyle = '#c09000';
      }
      break;
    }
    case 'heal': {
      // Green cross
      const cw = r * 0.32, cl = r;
      ctx.fillStyle = '#009820';
      ctx.fillRect(-cw, -cl, cw * 2, cl * 2);
      ctx.fillRect(-cl, -cw, cl * 2, cw * 2);
      ctx.fillStyle = '#40e060';
      ctx.fillRect(-cw * 0.65, -cw * 0.65, cw * 1.3, cw * 1.3);
      break;
    }
  }

  ctx.restore();
}

// ── State ──────────────────────────────────────────────────────────────────
const S = {
  token: null, ws: null,
  tanks: [], maps: [],
  selectedTank: null,
  myPlayerID: null, roomID: null,
  maxPlayers: 4, playerCount: 0,
  map: null, players: [], kills: 0,
};

const interp = { prev: null, curr: null, prevTime: 0, currTime: 0 };
let rafId = null;

function lerp(a, b, t) { return a + (b - a) * t; }

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
  if (S.tanks.length) S.selectedTank = S.tanks[0].id;
}

async function fetchMaps() {
  const r = await fetch('/api/maps');
  S.maps = await r.json();
}

async function fetchRooms() {
  return (await fetch('/api/rooms')).json();
}

// ── Lobby ──────────────────────────────────────────────────────────────────
function drawTankPreview(canvas, tankType) {
  const size = 64;
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext('2d');
  ctx.fillStyle = '#111118';
  ctx.fillRect(0, 0, size, size);
  ctx.save();
  ctx.translate(size / 2, size / 2);
  drawTankPixel(ctx, tankType, PLAYER_COLORS[0], false, size * 0.8);
  ctx.restore();
}

function renderTankCards() {
  const el = document.getElementById('tank-list');
  el.innerHTML = '';
  S.tanks.forEach(t => {
    const card = document.createElement('div');
    card.className = 'tank-card' + (t.id === S.selectedTank ? ' selected' : '');
    const canvas = document.createElement('canvas');
    canvas.className = 'tank-preview-canvas';
    card.appendChild(canvas);
    card.innerHTML += `
      <div class="name">${t.name}</div>
      <div class="stat">❤ ${t.hp}</div>
      <div class="stat">⚡ ${t.speed}</div>
      <div class="stat">💥 ${t.bullet_damage}</div>`;
    card.insertBefore(canvas, card.firstChild);
    drawTankPreview(canvas, t.id);
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
document.getElementById('btn-back').onclick    = leaveRoom;

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
  ws.onmessage = ev => {
    try { handleMsg(JSON.parse(ev.data)); }
    catch(e) { console.error('handleMsg:', e); }
  };
  ws.onclose = () => {
    S.ws = null;
    // If connection drops while in game — show game over instead of freezing.
    if (document.getElementById('screen-game').classList.contains('active')) {
      endGame('');
    }
  };
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
      p.tile_changes?.forEach(ch => { S.map.tiles[ch.y][ch.x] = ch.t; });
      p.kills?.forEach(k => { if (k.killer_id === S.myPlayerID) S.kills++; });
      updateHUD(p);
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
    const maxHp = cfg?.hp ?? 1;
    const pct = Math.round(me.hp / maxHp * 100);
    document.getElementById('hud-hp').textContent = `HP ${pct}%`;

    const buffs = document.getElementById('hud-buffs');
    buffs.innerHTML = '';
    if (me.damage_boost) {
      const b = document.createElement('span');
      b.className = 'buff-badge buff-damage'; b.textContent = 'ATK×2';
      buffs.appendChild(b);
    }
    if (me.armor_active) {
      const b = document.createElement('span');
      b.className = 'buff-badge buff-armor'; b.textContent = 'ARMOR';
      buffs.appendChild(b);
    }
    if (me.speed_boost) {
      const b = document.createElement('span');
      b.className = 'buff-badge buff-speed'; b.textContent = 'SPEED';
      buffs.appendChild(b);
    }
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
  showScreen('screen-game');
  calcTileSize();
  window.addEventListener('resize', calcTileSize);
  interp.prev = null; interp.curr = null;
  S.kills = 0;
  startRenderLoop();
}

function calcTileSize() {
  if (!S.map) return;
  // Use canvas layout dimensions (set by CSS flex)
  const vw = canvas.clientWidth  || window.innerWidth;
  const vh = canvas.clientHeight || Math.floor(window.innerHeight * 0.72);
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

  // Tiles
  for (let y = 0; y < S.map.height; y++)
    for (let x = 0; x < S.map.width; x++)
      drawTile(ctx, S.map.tiles[y][x], x * T, y * T, T);

  // Power-ups
  const now = performance.now();
  snap.power_ups?.forEach(pu => {
    drawPowerUp(ctx, pu.type, pu.x * T, pu.y * T, T, now);
  });

  // Bullets — small white squares (Battle City style)
  const bSize = Math.max(2, Math.round(T * 0.18));
  ctx.fillStyle = '#ffffff';
  snap.bullets?.forEach(b => {
    ctx.fillRect(b.x * T + (T - bSize) / 2, b.y * T + (T - bSize) / 2, bSize, bSize);
  });

  // Tanks
  snap.tanks?.forEach((t, i) => {
    if (t.hp <= 0) return;

    const prev = interp.prev?.tanks?.find(p => p.id === t.id);
    const rx = prev ? lerp(prev.x, t.x, alpha) : t.x;
    const ry = prev ? lerp(prev.y, t.y, alpha) : t.y;

    const cx = rx * T + T / 2;
    const cy = ry * T + T / 2;
    const isMe = t.id === S.myPlayerID;
    const color = PLAYER_COLORS[i % PLAYER_COLORS.length];

    ctx.save();
    ctx.translate(cx, cy);
    ctx.rotate((DIR_DEG[t.dir] ?? 0) * Math.PI / 180);
    drawTankPixel(ctx, t.type, color, isMe, T);
    ctx.restore();

    // HP bar — % based on tank config
    const cfg = S.tanks.find(tc => tc.id === t.type);
    const maxHp = cfg?.hp || 1;
    const barW = T - 4;
    const barY = ry * T - 4;
    ctx.fillStyle = '#111';
    ctx.fillRect(rx * T + 2, barY, barW, 3);
    ctx.fillStyle = t.hp / maxHp > 0.5 ? '#4caf50' : '#f44336';
    ctx.fillRect(rx * T + 2, barY, Math.round(barW * t.hp / maxHp), 3);
  });
}

function endGame(winnerID) {
  stopRenderLoop();
  resetTouchMove();
  const win  = winnerID !== '' && winnerID === S.myPlayerID;
  const draw = winnerID === 'draw';
  const lost = winnerID !== '' && !win && !draw;
  const disc = winnerID === '';
  document.getElementById('go-title').textContent =
    disc ? '⚡ Связь потеряна' : draw ? '🤝 Ничья' : win ? '🏆 Победа!' : '💀 Поражение';
  document.getElementById('go-sub').textContent =
    disc ? 'Соединение разорвано' : draw ? '' : win ? 'Ты лучший!' : 'В следующий раз повезёт';
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

document.querySelectorAll('[data-dir]').forEach(btn => {
  btn.addEventListener('pointerdown', e => { e.preventDefault(); startMove(btn.dataset.dir); });
  ['pointerup', 'pointerleave', 'pointercancel'].forEach(ev =>
    btn.addEventListener(ev, e => { e.preventDefault(); stopMove(); })
  );
});

document.getElementById('btn-shoot').addEventListener('pointerdown', e => {
  e.preventDefault();
  inp.shoot = true;
  flushInput();
});

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

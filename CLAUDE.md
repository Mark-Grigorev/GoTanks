# GoTanks — архитектурный гайд для Claude

## Общая схема

```
Telegram Mini App
      ↕ HTTPS / WSS
 Go monolith
 ├── REST API  (аккаунты, каталог танков/карт, статистика, лидерборд)
 ├── WS Hub    (игровые комнаты, game loop)
 └── Auth      (валидация Telegram initData через HMAC-SHA256)
      ↕
 PostgreSQL    (всё персистентное)
```

Монолит — правильный выбор на старте. Сервисы добавлять когда и если упрёшься в реальный bottleneck.

---

## Хранение данных

| Что | Где | Почему |
|---|---|---|
| Аккаунты, статистика | PostgreSQL | реляции, запросы |
| Типы танков, карты | YAML-конфиги | меняются редко, удобно редактировать |
| Активные игры | in-memory (Go structs) | скорость, персистентность не нужна |
| Сессии | JWT (из Telegram initData) | Redis не нужен |

---

## Аутентификация

```
Фронт → POST /api/auth { init_data }
Сервер → проверяет HMAC-SHA256(HMAC("WebAppData", botToken), dataCheckString)
       → upsert users + user_stats
       → возвращает JWT
```

Токен бота нигде не хранится на клиенте. JWT подписывается JWT_SECRET (минимум 16 символов).

---

## Схема БД

```sql
users         (id, telegram_id, username, created_at)
user_stats    (user_id, wins, losses, kills, deaths)
matches       (id, map_id, started_at, finished_at)
match_players (match_id, user_id, tank_type, result)
```

Миграции через golang-migrate, файлы в `migrations/`, embedded через `embed.FS`.

---

## YAML-конфиги

YAML-файлы **embedded** в бинарник через `//go:embed` в `internal/loader/loader.go`:
```go
//go:embed tanks maps
var embeddedFS embed.FS

func Load() (*Loader, error) {
    return LoadFrom(embeddedFS, "tanks", "maps")
}
func LoadFrom(fsys fs.FS, tanksDir, mapsDir string) (*Loader, error) { ... }
```
`Load()` — для продакшена (embedded). `LoadFrom(os.DirFS(...))` — для тестов.

### Танк (пример: `tanks/heavy.yaml`)
```yaml
id: heavy
name: "Тяжёлый"
speed: 2
hp: 10
bullet_speed: 6
bullet_damage: 2
shoot_cooldown: 14
hull: "PNG/Hulls_Color_B/Hull_06.png"
gun:  "PNG/Weapon_Color_B_256X256/Gun_06.png"
```

Загружается при старте в `map[string]*TankConfig` — O(1) доступ. Поля `hull`/`gun` сохранены для совместимости, рендеринг не использует PNG.

### Баланс танков (актуальные значения)

| ID | Имя | Speed | HP | BulletDmg | Cooldown |
|---|---|---|---|---|---|
| scout | Разведчик | 4 | 2 | 1 | 6 |
| light | Лёгкий | 3 | 4 | 1 | 8 |
| medium | Средний | 2 | 6 | 2 | 10 |
| rapid | Скорострел | 3 | 4 | 1 | 4 |
| sniper | Снайпер | 2 | 4 | 3 | 20 |
| heavy | Тяжёлый | 2 | 10 | 2 | 14 |
| siege | Осадный | 1 | 12 | 4 | 22 |

`moveInterval = tickRate / speed` — количество тиков между шагами. При 20 TPS: speed=4 → каждые 5 тиков, speed=1 → каждые 20 тиков.

Текущие 7 танков: `scout`, `light`, `medium`, `rapid`, `sniper`, `heavy`, `siege`.

### Карта (пример: `maps/arena.yaml`)
```yaml
id: arena
name: "Арена"
width: 16
height: 12
tiles:       # 0=пусто, 1=стена, 2=кирпич
  - [1,1,...]
spawns:      # [x, y] точки спавна
  - [1, 1]
```

Текущие 3 карты: `arena`, `fort`, `jungle`.

---

## Game loop

- Каждая комната — отдельная горутина с `time.Ticker`
- Тикрейт: 20 TPS (конфигурируется через `TICK_RATE`)
- Направления: 0=вверх, 1=вправо, 2=вниз, 3=влево
- Мьютекс на Room освобождается через `defer` — не вручную
- **Баг-фикс**: при спавне пули проверяется `IsSolid(bx, by)` — если клетка перед танком занята стеной или кирпичом, пуля не создаётся (кирпич уничтожается сразу). Это устраняет баг стрельбы сквозь стену вплотную.

### winner()

Победитель определяется по факту: есть хотя бы один мёртвый игрок.
```go
func (r *Room) winner() string {
    var alive []*Player
    var dead int
    for _, p := range r.Players {
        if p.Tank.Alive { alive = append(alive, p) } else { dead++ }
    }
    if dead == 0 { return "" }
    switch len(alive) {
    case 0: return "draw"
    case 1: return alive[0].ID
    }
    return ""
}
```

### RemovePlayer() во время игры

Если игрок отключается во время боя — танк помечается мёртвым (не удаляется из `r.Players`), иначе `dead` не посчитается и `winner()` не сработает:
```go
if r.State == RoomPlaying {
    p.Tank.Alive = false
    p.Tank.HP = 0
} else {
    delete(r.Players, playerID)
}
```

### KillEvent — JSON-теги обязательны

```go
type KillEvent struct {
    KillerPlayerID string `json:"killer_id"`
    VictimPlayerID string `json:"victim_id"`
    KillerUserID   int64  `json:"killer_user_id"`
    VictimUserID   int64  `json:"victim_user_id"`
}
```

---

## Hub — ограничения хоста

`roomEntry` хранит `hostPlayerID`. `start_game` принимается только от хоста:
```go
if entry.hostPlayerID != c.PlayerID {
    c.Send(ServerMsg{Type: "error", Payload: ErrorPayload{Message: "only host can start"}})
    return
}
```
`RoomJoinedPayload` содержит `IsHost bool json:"is_host"` — клиент скрывает кнопку старта для не-хостов.

---

## WebSocket протокол

**Клиент → Сервер:**
```json
{ "type": "create_room", "map_id": "arena", "tank_type": "heavy" }
{ "type": "join_room",   "room_id": "...",  "tank_type": "scout" }
{ "type": "start_game" }
{ "type": "input",       "input": { "move": "up", "shoot": false } }
{ "type": "leave_room" }
```

**Сервер → Клиент:**
```json
{ "type": "room_joined",  "payload": { "player_id": "...", "room_id": "...", "is_host": true, ... } }
{ "type": "game_start",   "payload": { "map": {...}, "players": [...] } }
{ "type": "state",        "payload": { "tanks": [...], "bullets": [...], "tile_changes": [...], "kills": [...] } }
{ "type": "game_over",    "payload": { "winner_id": "..." } }
```

Движение: клиент шлёт `input` каждые 50 мс пока кнопка зажата (`setInterval`). Один раз при нажатии — недостаточно (сервер применяет движение только на тиках, где получен input).

---

## Клиентский рендеринг (Canvas) — стиль Battle City 90-х

PNG-спрайты **не используются**. Всё рисуется на Canvas API программно.

### Цвета игроков
`PLAYER_COLORS = ['#c49a3c', '#1e7a32', '#a82010', '#1848b0']` — военная палитра (песочный, зелёный, красный, синий).

### Тайлы — `drawTile(ctx, type, px, py, T)`
- `TILE_EMPTY`: тёмный фон `#0c0c10` + точечная текстура (при T ≥ 12)
- `TILE_BRICK`: морtar-фон `#4a1200` + 4 кирпичика 2×2 с highlight/shadow
- `TILE_WALL`: 4 стальные панели с рельефом (светлая верх/лево, тёмная низ/право)

### Танки — `drawTankPixel(ctx, tankType, color, isMe, T)`
Параметры формы берутся из `TANK_DEFS[tankType]` (доли от T):
- `tw` — ширина гусениц
- `bw` — ширина корпуса
- `turW` — ширина башни
- `gunW` / `gunL` — толщина/длина ствола

Порядок отрисовки: гусеницы → корпус → ствол → башня (башня перекрывает основание ствола).
Танк всегда рисуется лицом вверх в локальном пространстве, затем поворачивается через `ctx.rotate(DIR_DEG[dir] * Math.PI / 180)`.
Свой танк помечается белым крестом на корпусе.

### Пули
Белые квадраты `bSize = max(2, round(T * 0.18))`, без интерполяции.

### HP-бар
Отображается в % от максимального HP (`t.hp / maxHp`). HUD тоже показывает `HP X%`.

### Адаптивный размер
CSS-лейаут: `#screen-game` — flex-колонка (через `.screen.active { display: flex }`), `#game-canvas` — `flex: 1` (занимает всё место между HUD и dpad). `calcTileSize()` читает `canvas.clientWidth/clientHeight` из CSS, не использует `innerHeight - px`.
Все размеры в CSS через `clamp()`, `vw`, `dvh`.

### CSS — критические правила

**`#screen-game` НЕ должен содержать `display: flex`** — только `.screen.active { display: flex }` управляет видимостью. ID-селектор имеет специфичность (0,1,0,0) и переопределяет `.screen { display: none }` (0,0,1,0), что приводит к тому что экран игры всегда виден и перекрывает экран окончания игры.

### Fallback-таймер game over на клиенте

Если `game_over` WS-сообщение теряется (буфер `c.send` заполнен), клиент самостоятельно определяет конец игры через `gameOverTimer` по состоянию HP танков в очередном `state`-пакете. Задержка 1500 мс.

---

## Статические файлы (web)

Фронтенд (`cmd/web/`) embedded в бинарник через `//go:embed web` в `cmd/main.go`:
```go
//go:embed web
var webFiles embed.FS

webRoot, _ := fs.Sub(webFiles, "web")
hand := handler.New(authSvc, db, l, h, webRoot)
```
`Handler` принимает `web fs.FS` как параметр — статика не захардкожена в handler-пакете.

---

## Логгер (`internal/logger`)

```go
log := logger.New(false) // false=JSON/INFO (prod), true=Text/DEBUG (local)
log.Infof("loaded %d tanks", n)
log.Errorf("db: %v", err)
child := log.With("room", roomID) // структурные поля
```

Никаких прямых вызовов `slog` вне пакета `logger`.

---

## Тесты

Покрыты чистыми unit-тестами (без БД и внешних сервисов):

| Пакет | Что тестируется |
|---|---|
| `internal/auth` | HMAC-валидация initData, выдача/парсинг JWT (expired, wrong secret, tampered) |
| `internal/config` | Дефолты, кастомные значения, ошибка при отсутствии required полей |
| `internal/loader` | Загрузка YAML (embedded + `LoadFrom`), пустые директории, невалидный YAML, игнор не-.yaml файлов |
| `internal/game` | `IsSolid/IsWalkable/TileAt/DestroyBrick`, движение танка, коллизия со стеной, спавн пули (нормальный / стена / кирпич), кулдаун, движение пули, winner-логика |
| `internal/logger` | `New`, все методы, `With` |

Используется `github.com/stretchr/testify` (`assert` / `require`).

Запуск: `go test ./internal/...`

---

## Конфиг приложения

Используется `envconfig.Process("", &cfg)` из `github.com/kelseyhightower/envconfig`. Никаких `os.Getenv` вручную.

```go
type Config struct {
    AppPort      string        `envconfig:"APP_PORT"        default:"8080"`
    AppEnv       string        `envconfig:"APP_ENV"         default:"local"`
    DBConnString string        `envconfig:"DB_CONN_STRING"  required:"true"`
    BotToken     string        `envconfig:"BOT_TOKEN"       required:"true"`
    JWTSecret    string        `envconfig:"JWT_SECRET"      required:"true"`
    JWTDuration  time.Duration `envconfig:"JWT_DURATION"    default:"24h"`
    TickRate     int           `envconfig:"TICK_RATE"       default:"20"`
    MaxPlayers   int           `envconfig:"MAX_PLAYERS"     default:"4"`
}
```

---

## Стек

| | |
|---|---|
| Go 1.26+ | Chi + gorilla/websocket |
| PostgreSQL 16 | pgx/v5 (не database/sql) |
| Миграции | golang-migrate + iofs driver + embed.FS |
| JWT | golang-jwt/jwt/v5 |
| Конфиг | kelseyhightower/envconfig |
| .env | joho/godotenv (только локально) |
| YAML | gopkg.in/yaml.v3 |
| Тесты | testify (assert/require) |
| Деплой | Docker Compose |
| TLS | Caddy / Nginx (обязателен для Mini App) |
| CI/CD | GitHub Actions → GHCR (на теге `v*`) |

---

## Важные решения и ограничения

- **Redis не нужен** — JWT сессии, in-memory игры, 10 GB RAM хватит на сотни матчей
- **PNG-спрайты не используются в рендеринге** — танки и тайлы рисуются на Canvas API. Поля `hull`/`gun` в YAML-конфигах остаются для возможного будущего использования
- **Dockerfile** — путь сборки `./cmd/` (main.go в корне `cmd/`), не `./cmd/server/`; образ финальный Alpine
- **DB_CONN_STRING** внутри Docker — хост `db`, не `localhost`
- **Файлы миграций** — формат `{version}_{name}.up.sql` (например `000001_init.up.sql`)
- **Логгер** — только через `internal/logger.Logger`, не напрямую через `slog`
- **Embed**: статика и YAML-конфиги embedded в бинарник — отдельных `COPY` в Dockerfile не нужно

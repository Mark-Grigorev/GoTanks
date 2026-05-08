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

### Танк (пример: `tanks/heavy.yaml`)
```yaml
id: heavy
name: "Тяжёлый"
speed: 2
hp: 5
bullet_speed: 8
bullet_damage: 2
shoot_cooldown: 15
hull: "PNG/Hulls_Color_B/Hull_06.png"
gun:  "PNG/Weapon_Color_B_256X256/Gun_06.png"
```

Загружается при старте в `map[string]*TankConfig` — O(1) доступ. Пути спрайтов относительны `tanks-sprites/`.

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
- `initialPlayerCount` фиксируется при старте — одиночный игрок никогда не получает "победу" автоматически
- `winner()` срабатывает только если `initialPlayerCount > 1` и есть хотя бы один погибший
- Направления: 0=вверх, 1=вправо, 2=вниз, 3=влево
- Мьютекс на Room освобождается через `defer` — не вручную

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
{ "type": "room_joined",  "payload": { "player_id": "...", "room_id": "...", ... } }
{ "type": "game_start",   "payload": { "map": {...}, "players": [...] } }
{ "type": "state",        "payload": { "tanks": [...], "bullets": [...], "tile_changes": [...], "kills": [...] } }
{ "type": "game_over",    "payload": { "winner_id": "..." } }
```

Движение: клиент шлёт `input` каждые 50 мс пока кнопка зажата (`setInterval`). Один раз при нажатии — недостаточно (сервер применяет движение только на тиках, где получен input).

---

## Клиентский рендеринг (Canvas)

- Все 14 спрайтов (7 танков × hull+gun) загружаются **до** открытия лобби через `Promise.all`
- Танк рендерится двумя слоями: hull → gun, оба `ctx.drawImage` 256×256
- Размер танка: 55% от тайла (`TS = T * 0.55`)
- Ротация по направлению: `ctx.rotate(DIR_DEG[dir] * Math.PI / 180)`
- Fallback на цветные прямоугольники если спрайт не загрузился
- HP-бар рендерится вне трансформации (всегда горизонтальный)

---

## Конфиг приложения

Используется `envconfig.Process("", &cfg)` из `github.com/kelseyhightower/envconfig`. Никаких `os.Getenv` вручную.

```go
type Config struct {
    Port        int           `envconfig:"APP_PORT"        default:"8080"`
    Env         string        `envconfig:"APP_ENV"         default:"local"`
    DBConn      string        `envconfig:"DB_CONN_STRING"  required:"true"`
    BotToken    string        `envconfig:"BOT_TOKEN"       required:"true"`
    JWTSecret   string        `envconfig:"JWT_SECRET"      required:"true"`
    JWTDuration time.Duration `envconfig:"JWT_DURATION"    default:"24h"`
    TickRate    int           `envconfig:"TICK_RATE"       default:"20"`
    MaxPlayers  int           `envconfig:"MAX_PLAYERS"     default:"4"`
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
| Деплой | Docker Compose |
| TLS | Caddy / Nginx (обязателен для Mini App) |
| CI/CD | GitHub Actions → GHCR (на теге `v*`) |

---

## Важные решения и ограничения

- **Redis не нужен** — JWT сессии, in-memory игры, 10 GB RAM хватит на сотни матчей
- **Спрайты из `tanks-sprites/PNG/`** — использовать только папки `_256X256` для орудий (не `Weapon_Color_X/` — там 36×90, не квадратные)
- **Dockerfile** — флаг `-o app`, не `-a app`; `tanks-sprites/` копируется в финальный образ
- **DB_CONN_STRING** внутри Docker — хост `db`, не `localhost`
- **Файлы миграций** — формат `{version}_{name}.up.sql` (например `000001_init.up.sql`)

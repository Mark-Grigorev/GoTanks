# GoTanks 🎮
🇷🇺 Русский · 🇬🇧 English

## О проекте
GoTanks — мультиплеерная браузерная игра «Танки» в виде Telegram Mini App. Игроки создают комнаты, выбирают один из 7 типов танков, сражаются на 3 картах и отслеживают свою статистику.

## Функциональность
- **Аккаунты** — вход через Telegram без паролей и регистрации
- **7 типов танков** — Разведчик, Лёгкий, Средний, Скорострельный, Снайпер, Тяжёлый, Осадный — каждый с уникальными характеристиками (скорость, HP, урон, кулдаун)
- **3 карты** — Арена, Форт, Джунгли; задаются YAML-конфигами
- **Матчи** — комнаты до 4 игроков с real-time game loop на 20 TPS
- **Pixel art рендеринг** — танки и тайлы рисуются на Canvas API в стиле Battle City 90-х, без PNG-спрайтов
- **Статистика** — победы, поражения, убийства, смерти
- **Лидерборд** — топ игроков по статистике

## Стек технологий

| Слой | Технология |
|---|---|
| Язык | Go 1.26+ |
| HTTP Router | Chi |
| WebSocket | gorilla/websocket |
| База данных | PostgreSQL 16 |
| Аутентификация | Telegram initData (HMAC-SHA256) + JWT |
| Конфиги танков/карт | YAML |
| Reverse proxy | Caddy / Nginx |
| Контейнеры | Docker / Docker Compose |
| CI/CD | GitHub Actions → GHCR |

## Структура проекта

```
.
├── cmd/                  # точка входа (main.go)
├── internal/
│   ├── auth/             # валидация Telegram initData, выдача JWT
│   ├── config/           # envconfig-конфиг приложения
│   ├── handler/          # HTTP-хэндлеры (Chi routes)
│   ├── hub/              # WS-хаб, жизненный цикл комнат
│   ├── game/             # game loop, состояние комнаты, сущности
│   ├── store/            # запросы к PostgreSQL
│   ├── loader/           # загрузка YAML-конфигов танков и карт
│   └── logger/           # обёртка над slog с printf-стилем
├── tanks/                # 7 YAML-конфигов танков
├── maps/                 # 3 YAML-конфига карт
├── web/                  # Telegram Mini App (HTML / JS / CSS)
├── migrations/           # миграции БД (golang-migrate)
├── .github/workflows/    # CI (lint + test) + CD (docker push на тег)
├── docker-compose.yaml
└── Dockerfile.gotanks
```

## Быстрый старт
Требования: Docker и Docker Compose

```bash
cp .env.example .env       # заполнить BOT_TOKEN и JWT_SECRET
docker compose up --build -d
```

Приложение поднимется на порту `APP_PORT` (по умолчанию 8080). TLS обязателен для Telegram Mini App — используй Caddy/Nginx или туннель (например, localhost.run).

Локальная разработка (Go 1.26+):

```bash
make run                      # запустить приложение
make build                    # собрать бинарник в bin/server
go test ./internal/...        # unit-тесты
make lint                     # go vet
```

## Переменные окружения

| Переменная | По умолчанию | Описание |
|---|---|---|
| APP_PORT | 8080 | Порт приложения |
| APP_ENV | local | Окружение (local, prod) |
| DB_CONN_STRING | — | Строка подключения к PostgreSQL |
| BOT_TOKEN | — | Токен Telegram-бота для валидации initData |
| JWT_SECRET | — | Секрет для подписи JWT (минимум 16 символов) |
| JWT_DURATION | 24h | Время жизни JWT |
| TICK_RATE | 20 | Тикрейт game loop (TPS) |
| MAX_PLAYERS | 4 | Максимум игроков в комнате |

## CI/CD

- **CI** — запускается на пуш в `main`, `dev`, `feature*` и PR: lint + тесты
- **CD** — запускается на тег `v*`: собирает Docker-образ и пушит в `ghcr.io/mark-grigorev/gotanks`

## Коды завершения

| Код | Причина |
|---|---|
| 0 | Штатное завершение (graceful shutdown по сигналу) |
| 2 | Ошибка загрузки конфигурации (не задан BOT_TOKEN и т.п.) |
| 3 | Ошибка инициализации JWT (неверный JWT_SECRET) |
| 4 | Нет подключения к PostgreSQL |
| 5 | Ошибка миграций БД |
| 6 | Graceful shutdown не завершился в срок |

Код 1 зарезервирован рантаймом Go (unrecovered panic).

## Лицензия
MIT

---

## GoTanks 🎮 — Multiplayer Tank Game
Battle your friends directly in Telegram — no installation, no sign-up.

## About
GoTanks is a multiplayer browser tank game built as a Telegram Mini App. Players create rooms, pick one of 7 tank types, fight on 3 maps, and track their stats.

## Features
- **Accounts** — sign in via Telegram, no passwords or registration
- **7 tank types** — Scout, Light, Medium, Rapid, Sniper, Heavy, Siege — each with unique stats (speed, HP, damage, cooldown)
- **3 maps** — Arena, Fort, Jungle; defined by YAML configs
- **Matches** — rooms for up to 4 players with a real-time 20 TPS game loop
- **Pixel art rendering** — tanks and tiles drawn via Canvas API in Battle City 90s style, no PNG sprites
- **Statistics** — wins, losses, kills, deaths
- **Leaderboard** — top players by stats

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.26+ |
| HTTP Router | Chi |
| WebSocket | gorilla/websocket |
| Database | PostgreSQL 16 |
| Auth | Telegram initData (HMAC-SHA256) + JWT |
| Tank/Map configs | YAML |
| Reverse proxy | Caddy / Nginx |
| Containers | Docker / Docker Compose |
| CI/CD | GitHub Actions → GHCR |

## Project Structure

```
.
├── cmd/                  # entry point (main.go)
├── internal/
│   ├── auth/             # Telegram initData validation, JWT issuance
│   ├── config/           # envconfig-based app config
│   ├── handler/          # HTTP handlers (Chi routes)
│   ├── hub/              # WS hub, room lifecycle management
│   ├── game/             # game loop, room state, entities
│   ├── store/            # PostgreSQL queries
│   ├── loader/           # YAML tank/map config loader
│   └── logger/           # slog wrapper with printf-style API
├── tanks/                # 7 tank YAML configs
├── maps/                 # 3 map YAML configs
├── web/                  # Telegram Mini App (HTML / JS / CSS)
├── migrations/           # DB migrations (golang-migrate)
├── .github/workflows/    # CI (lint + test) + CD (docker push on tag)
├── docker-compose.yaml
└── Dockerfile.gotanks
```

## Getting Started
Prerequisites: Docker & Docker Compose

```bash
cp .env.example .env       # fill in BOT_TOKEN and JWT_SECRET
docker compose up --build -d
```

The app listens on `APP_PORT` (default 8080). TLS is required for Telegram Mini App — use Caddy/Nginx or a tunnel (e.g. localhost.run).

Local development (Go 1.26+):

```bash
make run                      # run the app
make build                    # build binary to bin/server
go test ./internal/...        # unit tests
make lint                     # go vet
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| APP_PORT | 8080 | Application port |
| APP_ENV | local | Environment (local, prod) |
| DB_CONN_STRING | — | PostgreSQL connection string |
| BOT_TOKEN | — | Telegram bot token for initData validation |
| JWT_SECRET | — | JWT signing secret (min 16 chars) |
| JWT_DURATION | 24h | JWT TTL |
| TICK_RATE | 20 | Game loop tick rate (TPS) |
| MAX_PLAYERS | 4 | Maximum players per room |

## CI/CD

- **CI** — runs on push to `main`, `dev`, `feature*` and on PRs: lint + tests
- **CD** — runs on `v*` tags: builds Docker image and pushes to `ghcr.io/mark-grigorev/gotanks`

## Exit Codes

| Code | Reason |
|---|---|
| 0 | Clean exit (graceful shutdown on signal) |
| 2 | Config load failed (e.g. BOT_TOKEN not set) |
| 3 | JWT init failed (invalid JWT_SECRET) |
| 4 | Could not connect to PostgreSQL |
| 5 | Database migration failed |
| 6 | Graceful shutdown timed out |

Code 1 is reserved by the Go runtime (unrecovered panic).

## License
MIT

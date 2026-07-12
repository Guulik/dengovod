# 1. Общая архитектура приложения

## 1.1 Назначение

Приложение — **финансовый календарь**, а не учёт расходов. Центральный экран — календарь, где каждый день показывает:

- ожидаемый баланс;
- события (доходы и расходы);
- обязательные и необязательные расходы;
- остаток «денег на жизнь».

Главный вопрос продукта:

> «Если я проживу по своему плану до следующего дохода, сколько свободных денег у меня останется?»

## 1.2 Архитектурный стиль

**Modular Monolith** с **Clean Architecture** на backend и **Feature-Sliced** структурой на frontend.

```
┌─────────────────────────────────────────────────────────────┐
│                      Browser (React)                        │
│  FullCalendar │ TanStack Query │ Zustand │ React Hook Form  │
└──────────────────────────┬──────────────────────────────────┘
                           │ REST / JSON
┌──────────────────────────▼──────────────────────────────────┐
│                    Go HTTP Server                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Transport  │→ │   Service   │→ │      Domain         │  │
│  │  (HTTP/DTO) │  │  (Use Cases)│  │ (Entities + Logic)  │  │
│  └─────────────┘  └──────┬──────┘  └──────────▲──────────┘  │
│                          │                     │            │
│                   ┌──────▼──────┐              │            │
│                   │ Repository  │──────────────┘            │
│                   │  (SQLite)   │                           │
│                   └─────────────┘                           │
└─────────────────────────────────────────────────────────────┘
```

### Почему Modular Monolith

| Критерий | Решение |
|----------|---------|
| MVP | Один процесс, одна команда `make dev` |
| Сложность | Нет инфраструктурного overhead (K8s, Kafka, микросервисы) |
| Расширяемость | Чёткие границы пакетов → позже можно выделить сервисы или добавить GraphQL transport |

**Альтернатива:** микросервисы — отклонены: преждевременная сложность для solo/small team MVP.

**Компромисс:** весь код в одном репозитории и одном deployable; масштабирование — вертикальное + PostgreSQL позже.

## 1.3 Стек

| Слой | Технологии |
|------|------------|
| Backend | Go, REST, SQLite, Clean Architecture, Repository Pattern, Service Layer |
| Frontend | React, TypeScript, Vite, Tailwind, shadcn/ui, TanStack Query, Zustand, React Hook Form, Zod, date-fns, FullCalendar |
| Monorepo | `backend/` + `frontend/` + `docs/` + `Makefile` |

### Что сознательно НЕ используется в MVP

- GraphQL, gRPC — REST достаточен; transport-слой изолирован
- Event Sourcing, CQRS — избыточно для MVP
- Kafka, микросервисы, Kubernetes — инфраструктурный overhead
- ORM — SQL через `database/sql` + `sqlx` или `go-sqlite3`; явный контроль миграций

## 1.4 Структура monorepo

```
dengovod/
├── Makefile                    # make dev — backend + frontend одной командой
├── README.md
├── docs/
│   └── architecture/           # этот документ
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go         # composition root, DI вручную
│   ├── internal/
│   │   ├── domain/             # сущности, value objects, интерфейсы репозиториев, чистая логика прогноза
│   │   ├── service/            # use cases (оркестрация)
│   │   ├── repository/
│   │   │   └── sqlite/         # реализации интерфейсов domain
│   │   └── transport/
│   │       └── http/           # handlers, middleware, DTO, маппинг
│   ├── migrations/             # SQL-миграции (goose / golang-migrate)
│   ├── go.mod
│   └── go.sum
└── frontend/
    ├── src/
    │   ├── api/                # REST-клиент, Zod-схемы
    │   ├── features/           # calendar, events, settings, balance, auth
    │   ├── components/         # shadcn/ui + общие компоненты
    │   ├── stores/             # Zustand (UI-состояние)
    │   ├── lib/                # утилиты, date helpers
    │   └── App.tsx
    ├── package.json
    ├── vite.config.ts
    └── tailwind.config.ts
```

### Почему такая структура

- **`internal/domain`** — единственное место бизнес-правил; не зависит от HTTP/SQL
- **`internal/service`** — оркестрация: загрузка данных → вызов domain → сохранение
- **`internal/repository/sqlite`** — заменяем на `repository/postgres` без изменения service/domain
- **`internal/transport/http`** — заменяем/дополняем `transport/graphql` без изменения service
- **`frontend/src/api`** — единственное место знания о REST; features не знают URL

**Альтернатива:** Nx/Turborepo — отклонена для MVP; достаточно Makefile + два независимых пакета.

## 1.5 Запуск одной командой

```makefile
# Makefile (эскиз)
.PHONY: dev migrate test

dev:
	$(MAKE) -j2 dev-backend dev-frontend

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

migrate:
	cd backend && go run ./cmd/migrate up

test:
	cd backend && go test ./...
	cd frontend && npm test
```

Backend слушает `:8080`, frontend (Vite) проксирует `/api` на backend.

## 1.6 Пути эволюции

| Будущая функция | Как архитектура это поддерживает |
|-----------------|----------------------------------|
| PostgreSQL | Новый пакет `repository/postgres`, те же интерфейсы в `domain` |
| GraphQL | Новый пакет `transport/graphql`, те же `service` |
| Мобильный клиент | Тот же REST API |
| CSV-импорт | Новый `ImportService` → пишет в `ActualTransaction` |
| Аналитика / графики | Новые read-only endpoints или materialized views; domain не меняется |
| Уведомления | Фоновый worker как отдельный `cmd/worker`, читает те же repositories |
| Cron-повторения | Новое значение `recurrence_type` + поле `cron_expr` в `financial_events` |
| JWT-аутентификация | Middleware в `transport/http`; handlers не меняются |
| План vs факт | `ActualTransaction` уже отделён; позже — сравнительные endpoints |

## 1.7 Принципы проектирования

1. **Domain-first** — алгоритм прогноза — чистая функция, тестируется без БД
2. **Dependency Inversion** — service зависит от интерфейсов domain, repository их реализует
3. **Thin transport** — handler только парсит HTTP, вызывает service, маппит DTO
4. **Server state на клиенте** — TanStack Query; Zustand только для UI (выбранный день, модалки)
5. **Money as integer** — суммы в копейках (`int64`), форматирование на UI
6. **Explicit over magic** — без ORM, без code generation на MVP

## 1.8 Диаграмма контекста (C4 Level 1)

```plantuml
@startuml context
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Context.puml

Person(user, "Пользователь", "Планирует расходы и доходы")
System(dengovod, "Денговод", "Финансовый календарь с прогнозом баланса")

Rel(user, dengovod, "Планирует, просматривает календарь", "HTTPS/REST")

@enduml
```

## 1.9 Ограничения MVP

- Одна валюта (RUB), поле `currency` в схеме зарезервировано
- Редактирование повторяющихся событий — только вся серия (overrides для вхождений — в схеме, не в UI)
- Фактические транзакции не влияют на прогноз (только статистика)
- Идентификация по логину без пароля (не security-grade; middleware готов к замене)

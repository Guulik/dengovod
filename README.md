# Денговод

Финансовый календарь: прогнозирует баланс и «деньги на жизнь» до выбранного горизонта.

## Требования

- Go 1.26+
- Node.js 24 LTS

## Запуск

```bash
make dev
```

- Frontend: http://localhost:5173
- Backend: http://localhost:8080
- SQLite: `backend/data/dengovod.db`

## Проверка

```bash
make test
make build
```

Архитектура описана в [`docs/architecture`](docs/architecture/README.md).


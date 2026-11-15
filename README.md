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

## Удалённый доступ через Tailscale (для теста с телефона)

Нужно, когда уходишь из дома, но хочешь открыть приложение в браузере телефона.
Mac остаётся дома включённым, телефон подключается через Tailscale VPN.

### 1. Установить Tailscale на Mac

```bash
brew install tailscale
brew install --cask tailscale   # GUI-приложение (потребует пароль)
```

Открой **Tailscale** из Applications и войди в аккаунт (Google/GitHub/Microsoft).

### 2. Установить Tailscale на телефон

- [iOS App Store](https://apps.apple.com/app/tailscale/id1475387142)
- [Google Play](https://play.google.com/store/apps/details?id=com.tailscale.ipn)

Войди в **тот же аккаунт**, что на Mac.

### 3. Запустить приложение для удалённого доступа

На Mac:

```bash
make dev-remote
```

Команда проверит Tailscale и покажет URL вида `http://100.x.x.x:5173`.

### 4. Открыть на телефоне

1. Убедись, что Tailscale на телефоне включён (Connected).
2. Открой в браузере URL, который показал `make dev-remote`.
3. Логинься как обычно.

### Полезные команды

```bash
make tailscale-setup   # проверить, что Tailscale готов
make tailscale-url     # показать только URL
```

### Важно

- Mac должен быть включён и не спать (Системные настройки → Энергосбережение → «Предотвращать переход в режим сна при питании от сети»).
- Работает только пока запущен `make dev` на Mac.
- Доступ есть только на устройствах в твоём Tailscale-аккаунте.

## Проверка

```bash
make test
make build
```

Архитектура описана в [`docs/architecture`](docs/architecture/README.md).

## Работа команды и приёмка

Роли и порядок поставки: [`docs/delivery/README.md`](docs/delivery/README.md). Утверждённые сценарии: [`SPEC.md`](docs/delivery/SPEC.md), финансовые правила и API: [`CONTRACT.md`](docs/delivery/CONTRACT.md). Текущий статус: [`STATUS.md`](docs/delivery/STATUS.md).

Браузерные тесты используют собственные серверы и временную SQLite-базу:

```bash
cd frontend
npx playwright install chromium
npm run test:e2e
```

Проверяются desktop и мобильный экран 390px. Порты по умолчанию 4179 и 18089; при необходимости задать `E2E_PORT` и `E2E_API_PORT`. Тесты не подключаются к уже запущенному приложению. Команды unit/build/e2e также запускаются в GitHub Actions.


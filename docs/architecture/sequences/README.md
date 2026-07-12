# Sequence Diagrams

Диаграммы последовательностей для REST API endpoints.

Рендер: PlantUML extension или [plantuml.com](https://www.plantuml.com/plantuml).

| Файл | Endpoint | Описание |
|------|----------|----------|
| [login.puml](./login.puml) | `POST /api/v1/auth/login` | Вход / регистрация по логину |
| [create-event.puml](./create-event.puml) | `POST /api/v1/events` | Создание события |
| [update-event.puml](./update-event.puml) | `PUT /api/v1/events/{id}` | Изменение события (вся серия) |
| [delete-event.puml](./delete-event.puml) | `DELETE /api/v1/events/{id}` | Удаление события |
| [get-calendar.puml](./get-calendar.puml) | `GET /api/v1/calendar` | Получение календаря |
| [get-forecast.puml](./get-forecast.puml) | `GET /api/v1/forecast` | Получение прогноза (Money for Life) |
| [set-anchor.puml](./set-anchor.puml) | `POST /api/v1/balance/anchor` | Установка якоря баланса |
| [create-day-cost-override.puml](./create-day-cost-override.puml) | `POST /api/v1/day-cost-overrides` | Оверрайд стоимости дня |

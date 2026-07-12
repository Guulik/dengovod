# 5. REST API

Base URL: `/api/v1`

Идентификация MVP: header `Authorization: Bearer <user_uuid>` (UUID выдаётся при
login). Это сознательно временный и небезопасный механизм: UUID нельзя считать
секретом. Проверка изолирована в middleware и должна быть заменена непрозрачной
сессией или JWT до публикации приложения в недоверенной сети.

Все суммы — **integer kopecks** (140000 = 1400.00 ₽).
Все даты — **string `YYYY-MM-DD`**.

## 5.1 Общие типы

### MoneyDTO

```json
{
  "amount": 140000,
  "currency": "RUB"
}
```

### ErrorResponse

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "amount must be positive",
    "details": [{ "field": "amount", "message": "must be > 0" }]
  }
}
```

### RecurrenceDTO

```json
{
  "type": "MONTHLY_ON_DAY",
  "start_date": "2026-01-15",
  "day_of_month": 15,
  "interval_days": null,
  "until_date": null
}
```

Types: `NONE` | `MONTHLY_ON_DAY` | `EVERY_N_DAYS`

---

## 5.2 Auth

### POST /auth/login

Вход или регистрация по логину.

**Request:**
```json
{
  "login": "ivan"
}
```

**Response 200:**
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "login": "ivan",
    "created_at": "2026-07-12T10:00:00Z"
  },
  "token": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Notes:** при первом входе создаются `user_settings` (defaults) и дефолтные категории.
Якорь автоматически не создаётся: frontend обязан провести пользователя через
onboarding (текущий баланс + базовая стоимость дня).

---

## 5.3 Settings

### GET /settings

**Response 200:**
```json
{
  "base_day_cost": { "amount": 140000, "currency": "RUB" },
  "horizon_policy": "END_OF_MONTH",
  "horizon_custom_date": null,
  "currency": "RUB",
  "updated_at": "2026-07-12T10:00:00Z"
}
```

### PUT /settings

**Request:**
```json
{
  "base_day_cost": { "amount": 150000, "currency": "RUB" },
  "horizon_policy": "NEXT_INCOME",
  "horizon_custom_date": null
}
```

**Response 200:** same as GET

---

## 5.4 Balance

### GET /balance

Текущее состояние баланса.

Если якорь ещё не создан, возвращается `409 ANCHOR_REQUIRED`:

```json
{
  "error": {
    "code": "ANCHOR_REQUIRED",
    "message": "balance anchor is required",
    "details": []
  }
}
```

**Response 200:**
```json
{
  "anchor": {
    "id": "...",
    "amount": { "amount": 1000000, "currency": "RUB" },
    "anchored_at": "2026-07-07T12:00:00Z"
  },
  "computed_balance": { "amount": 300000, "currency": "RUB" },
  "as_of": "2026-07-12T10:00:00Z"
}
```

### POST /balance/anchor

Установить новый якорь баланса.

**Request:**
```json
{
  "amount": { "amount": 800000, "currency": "RUB" },
  "anchored_at": "2026-07-12T18:00:00Z"
}
```

**Response 201:** BalanceState (same as GET /balance)

---

## 5.5 Events

### GET /events

**Query:** `?kind=EXPENSE` (optional)

**Response 200:**
```json
{
  "events": [
    {
      "id": "...",
      "kind": "EXPENSE",
      "necessity": "REQUIRED",
      "amount": { "amount": 3000000, "currency": "RUB" },
      "category_id": "...",
      "comment": "Аренда",
      "recurrence": {
        "type": "MONTHLY_ON_DAY",
        "start_date": "2026-01-01",
        "day_of_month": 1,
        "interval_days": null,
        "until_date": null
      },
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

### POST /events

**Request:**
```json
{
  "kind": "EXPENSE",
  "necessity": "REQUIRED",
  "amount": { "amount": 50000, "currency": "RUB" },
  "category_id": "...",
  "comment": "Продукты",
  "recurrence": {
    "type": "NONE",
    "start_date": "2026-07-15"
  }
}
```

**Response 201:** EventResponse (single event object)

### GET /events/{id}

**Response 200:** EventResponse

### PUT /events/{id}

Обновляет всю серию (MVP).

**Request:** same fields as POST (partial update supported)

**Response 200:** EventResponse

### DELETE /events/{id}

**Response 204**

---

## 5.6 Day Cost Overrides

### GET /day-cost-overrides

**Response 200:**
```json
{
  "overrides": [
    {
      "id": "...",
      "date_from": "2026-08-01",
      "date_to": "2026-08-07",
      "daily_amount": { "amount": 30000, "currency": "RUB" },
      "comment": "У родителей",
      "created_at": "..."
    }
  ]
}
```

### POST /day-cost-overrides

**Request:**
```json
{
  "date_from": "2026-08-01",
  "date_to": "2026-08-07",
  "daily_amount": { "amount": 30000, "currency": "RUB" },
  "comment": "У родителей"
}
```

**Response 201:** OverrideResponse

### PUT /day-cost-overrides/{id}

**Response 200:** OverrideResponse

### DELETE /day-cost-overrides/{id}

**Response 204**

---

## 5.7 Categories

### GET /categories

**Query:** `?kind=EXPENSE` (optional)

**Response 200:**
```json
{
  "categories": [
    {
      "id": "...",
      "name": "Жильё",
      "kind": "EXPENSE",
      "is_default": true
    }
  ]
}
```

### POST /categories

**Request:**
```json
{
  "name": "Хобби",
  "kind": "EXPENSE"
}
```

**Response 201:** CategoryResponse

### PUT /categories/{id}

**Response 200:** CategoryResponse

### DELETE /categories/{id}

**Response 204** (409 if events reference category)

---

## 5.8 Actual Transactions

### GET /transactions

**Query:** `?from=2026-07-01&to=2026-07-31`

**Response 200:**
```json
{
  "transactions": [
    {
      "id": "...",
      "kind": "EXPENSE",
      "amount": { "amount": 45000, "currency": "RUB" },
      "category_id": "...",
      "transaction_date": "2026-07-10",
      "comment": "Магнит",
      "event_id": null,
      "created_at": "..."
    }
  ]
}
```

### POST /transactions

**Request:**
```json
{
  "kind": "EXPENSE",
  "amount": { "amount": 45000, "currency": "RUB" },
  "category_id": "...",
  "transaction_date": "2026-07-10",
  "comment": "Магнит",
  "event_id": null
}
```

**Response 201:** TransactionResponse

### PUT /transactions/{id}

**Response 200:** TransactionResponse

### DELETE /transactions/{id}

**Response 204**

---

## 5.9 Calendar

### GET /calendar

Главный endpoint для FullCalendar.

**Query:**
- `from` (required): `YYYY-MM-DD`
- `to` (required): `YYYY-MM-DD`

**Response 200:**
```json
{
  "days": [
    {
      "date": "2026-07-12",
      "expected_balance": { "amount": 300000, "currency": "RUB" },
      "day_cost": { "amount": 140000, "currency": "RUB" },
      "money_for_life": { "amount": -400000, "currency": "RUB" },
      "income_total": { "amount": 0, "currency": "RUB" },
      "required_total": { "amount": 0, "currency": "RUB" },
      "optional_total": { "amount": 0, "currency": "RUB" },
      "events": [
        {
          "event_id": "...",
          "kind": "EXPENSE",
          "necessity": "REQUIRED",
          "amount": { "amount": 50000, "currency": "RUB" },
          "category_id": "...",
          "comment": "Продукты"
        }
      ]
    }
  ],
  "summary": {
    "current_balance": { "amount": 300000, "currency": "RUB" },
    "money_for_life": { "amount": -400000, "currency": "RUB" },
    "horizon_date": "2026-07-31",
    "horizon_policy": "END_OF_MONTH",
    "required_remaining": { "amount": 0, "currency": "RUB" },
    "day_cost_remaining": { "amount": 700000, "currency": "RUB" },
    "optional_remaining": { "amount": 0, "currency": "RUB" },
    "days_until_horizon": 19
  }
}
```

---

## 5.10 Forecast

### GET /forecast

Сводка «денег на жизнь» без детализации по дням.

**Response 200:**
```json
{
  "current_balance": { "amount": 300000, "currency": "RUB" },
  "money_for_life": { "amount": -400000, "currency": "RUB" },
  "horizon_date": "2026-07-31",
  "horizon_policy": "END_OF_MONTH",
  "required_remaining": { "amount": 0, "currency": "RUB" },
  "day_cost_remaining": { "amount": 700000, "currency": "RUB" },
  "optional_remaining": { "amount": 150000, "currency": "RUB" },
  "days_until_horizon": 19
}
```

---

## 5.11 DTO Structure (Go)

```go
// transport/http/dto/money.go
type MoneyDTO struct {
    Amount   int64  `json:"amount"`
    Currency string `json:"currency"`
}

// transport/http/dto/event.go
type CreateEventRequest struct {
    Kind        string         `json:"kind"`
    Necessity   string         `json:"necessity"`
    Amount      MoneyDTO       `json:"amount"`
    CategoryID  *string        `json:"category_id"`
    Comment     string         `json:"comment"`
    Recurrence  RecurrenceDTO  `json:"recurrence"`
}

type EventResponse struct {
    ID          string         `json:"id"`
    Kind        string         `json:"kind"`
    Necessity   string         `json:"necessity"`
    Amount      MoneyDTO       `json:"amount"`
    CategoryID  *string        `json:"category_id"`
    Comment     string         `json:"comment"`
    Recurrence  RecurrenceDTO  `json:"recurrence"`
    CreatedAt   time.Time      `json:"created_at"`
    UpdatedAt   time.Time      `json:"updated_at"`
}

// transport/http/dto/calendar.go
type CalendarDayDTO struct {
    Date            string          `json:"date"`
    ExpectedBalance MoneyDTO        `json:"expected_balance"`
    DayCost         MoneyDTO        `json:"day_cost"`
    MoneyForLife    MoneyDTO        `json:"money_for_life"`
    IncomeTotal     MoneyDTO        `json:"income_total"`
    RequiredTotal   MoneyDTO        `json:"required_total"`
    OptionalTotal   MoneyDTO        `json:"optional_total"`
    Events          []DayEventDTO   `json:"events"`
}

type CalendarResponse struct {
    Days    []CalendarDayDTO  `json:"days"`
    Summary ForecastSummaryDTO `json:"summary"`
}
```

## 5.12 Mapper (DTO ↔ Domain)

```go
// transport/http/mapper/event_mapper.go
func ToDomainEvent(req dto.CreateEventRequest, userID uuid.UUID) (domain.FinancialEvent, error)
func ToEventResponse(e domain.FinancialEvent) dto.EventResponse
func ToDomainRecurrence(d dto.RecurrenceDTO) (domain.RecurrenceRule, error)
func ToRecurrenceDTO(r domain.RecurrenceRule) dto.RecurrenceDTO
```

**Почему mapper отдельно от handler:** handlers остаются thin; mapper тестируется изолированно.

**Почему DTO отделены от domain:** transport не протекает в domain; JSON tags не загрязняют entities.

## 5.13 Frontend Zod Schemas (mirror)

```typescript
// frontend/src/api/schemas/money.ts
export const MoneySchema = z.object({
  amount: z.number().int(),
  currency: z.string(),
});

// frontend/src/api/schemas/calendar.ts
export const CalendarDaySchema = z.object({
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
  expected_balance: MoneySchema,
  day_cost: MoneySchema,
  money_for_life: MoneySchema,
  events: z.array(DayEventSchema),
});
```

## 5.14 HTTP Status Codes

| Code | When |
|------|------|
| 200 | OK |
| 201 | Created |
| 204 | Deleted |
| 400 | Malformed request |
| 401 | Missing/invalid token |
| 404 | Not found |
| 409 | Conflict (delete category with events) |
| 422 | Validation error |
| 500 | Internal error |

## 5.15 API Versioning

Prefix `/api/v1` — при breaking changes добавляем `/api/v2`, v1 поддерживаем параллельно.

## 5.16 Расширяемость

| Feature | New endpoints |
|---------|---------------|
| CSV import | `POST /imports/csv` |
| Analytics | `GET /analytics/spending?from&to` |
| GraphQL | `/graphql` — same services |
| Notifications | `GET/PUT /notification-preferences` |

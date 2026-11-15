# 8. Frontend Architecture

Сценарии план/факт, изоляция сессий и зарплатный калькулятор определены в [SPEC.md](../delivery/SPEC.md), точные DTO и правила ошибок — в [CONTRACT.md](../delivery/CONTRACT.md). Ниже сохранено исходное архитектурное описание.

## 8.1 Принципы

| Принцип | Реализация |
|---------|------------|
| Server state | TanStack Query — calendar, events, forecast, settings |
| UI state | Zustand — selected date, modals, calendar view mode |
| Forms | React Hook Form + Zod |
| Styling | Tailwind CSS + shadcn/ui |
| Calendar | FullCalendar (dayGridMonth, dayGridWeek) |
| Dates | date-fns (format, parse, addDays, startOfMonth) |
| API | Единый typed client в `src/api/` |

**Почему TanStack Query, а не Zustand для серверных данных:** автоматический cache, invalidation, loading/error states, deduplication.

**Почему Zustand для UI:** минимальный boilerplate; не нужен Redux для MVP.

## 8.2 Структура каталогов

```
frontend/src/
├── api/
│   ├── client.ts              # fetch wrapper, auth header, base URL
│   ├── query-keys.ts          # ['calendar', from, to], ['events'], ...
│   ├── schemas/               # Zod schemas (mirror backend DTO)
│   │   ├── money.ts
│   │   ├── event.ts
│   │   ├── calendar.ts
│   │   ├── forecast.ts
│   │   ├── settings.ts
│   │   └── ...
│   └── hooks/                 # TanStack Query hooks
│       ├── useCalendar.ts
│       ├── useEvents.ts
│       ├── useForecast.ts
│       ├── useSettings.ts
│       ├── useBalance.ts
│       └── useDayCostOverrides.ts
├── features/
│   ├── auth/
│   │   ├── LoginPage.tsx
│   │   └── useAuth.ts
│   ├── calendar/
│   │   ├── CalendarPage.tsx       # главный экран
│   │   ├── CalendarView.tsx       # FullCalendar wrapper
│   │   ├── DayCell.tsx            # custom day rendering
│   │   ├── DayDetailPanel.tsx     # sidebar при клике на день
│   │   └── MoneyForLifeWidget.tsx # виджет MFL
│   ├── events/
│   │   ├── EventFormModal.tsx
│   │   ├── RecurrenceSelector.tsx
│   │   └── event-form.schema.ts   # Zod schema for form
│   ├── balance/
│   │   ├── BalanceWidget.tsx
│   │   └── AnchorFormModal.tsx
│   ├── settings/
│   │   ├── SettingsPage.tsx
│   │   ├── DayCostForm.tsx
│   │   ├── HorizonSelector.tsx
│   │   └── DayCostOverrideList.tsx
│   └── transactions/
│       └── TransactionList.tsx    # MVP: простой список фактов
├── components/
│   ├── ui/                        # shadcn/ui components
│   ├── layout/
│   │   ├── AppLayout.tsx
│   │   ├── Header.tsx
│   │   └── Sidebar.tsx
│   └── money/
│       ├── MoneyDisplay.tsx       # форматирование kopecks → ₽
│       └── MoneyInput.tsx
├── stores/
│   ├── uiStore.ts                 # modals, selectedDate, viewMode
│   └── authStore.ts               # token persistence
├── lib/
│   ├── dates.ts                   # date-fns helpers
│   └── money.ts                   # kopecks formatting
├── App.tsx
└── main.tsx
```

## 8.3 Feature-Sliced подход

Каждая feature — самодостаточный модуль:
- UI components
- hooks (если feature-specific)
- Zod schemas (если form-specific)

Shared код — в `components/`, `lib/`, `api/`.

**Почему features, а не pages/components/hooks flat:** масштабируемость; новая feature = новая папка, не раздувание shared.

## 8.4 Потоки данных

### Calendar Page (главный экран)

```
CalendarPage
  ├── useForecast()          → GET /forecast → MoneyForLifeWidget
  ├── useBalance()           → GET /balance → BalanceWidget
  ├── useCalendar(from, to)  → GET /calendar → CalendarView
  └── uiStore.selectedDate   → DayDetailPanel
```

### Create Event

```
EventFormModal
  ├── react-hook-form + event-form.schema (Zod)
  ├── useCreateEvent()       → POST /events
  └── onSuccess → queryClient.invalidateQueries(['events', 'calendar', 'forecast'])
```

### Data Flow Diagram

```plantuml
@startuml frontend-data-flow

package "UI Layer" {
  [CalendarPage]
  [EventFormModal]
  [SettingsPage]
  [MoneyForLifeWidget]
}

package "State" {
  [TanStack Query Cache]
  [Zustand uiStore]
  [Zustand authStore]
}

package "API Layer" {
  [api/client.ts]
  [api/hooks/*]
  [api/schemas/*]
}

package "Backend" {
  [REST API]
}

CalendarPage --> TanStack Query Cache
EventFormModal --> TanStack Query Cache
SettingsPage --> TanStack Query Cache

CalendarPage --> uiStore
EventFormModal --> uiStore

TanStack Query Cache --> api/hooks/*
api/hooks/* --> api/client.ts
api/client.ts --> REST API
authStore --> api/client.ts

MoneyForLifeWidget --> TanStack Query Cache

@enduml
```

## 8.5 FullCalendar Integration

```typescript
// features/calendar/CalendarView.tsx (эскиз)
function CalendarView({ days }: { days: CalendarDayDTO[] }) {
  const events = days.flatMap(day =>
    day.events.map(e => ({
      title: e.comment || formatMoney(e.amount),
      date: day.date,
      extendedProps: { ...e, dayBalance: day.expected_balance },
    }))
  );

  return (
    <FullCalendar
      plugins={[dayGridPlugin]}
      initialView="dayGridMonth"
      events={events}
      dayCellContent={(arg) => (
        <DayCell
          date={arg.date}
          day={days.find(d => d.date === formatDate(arg.date))}
        />
      )}
      datesSet={(info) => {
        // trigger useCalendar(from, to) refetch
      }}
    />
  );
}
```

### DayCell — ключевой UI элемент

Каждая ячейка дня показывает:
- **Expected Balance** — мелким шрифтом сверху
- **Day Cost** — если отличается от base (override indicator)
- **Money for Life** — цветовая индикация (green/yellow/red)
- **Event dots** — цвет по kind (income=green, required=red, optional=gray)

## 8.6 Zustand Stores

```typescript
// stores/uiStore.ts
interface UIState {
  selectedDate: string | null;
  isEventModalOpen: boolean;
  isAnchorModalOpen: boolean;
  calendarView: 'month' | 'week';
  setSelectedDate: (date: string | null) => void;
  openEventModal: () => void;
  closeEventModal: () => void;
}

// stores/authStore.ts
interface AuthState {
  token: string | null;
  user: UserDTO | null;
  setAuth: (token: string, user: UserDTO) => void;
  logout: () => void;
}
```

**Что НЕ хранить в Zustand:** events, calendar data, forecast — только TanStack Query.

## 8.7 TanStack Query Keys

```typescript
// api/query-keys.ts
export const queryKeys = {
  calendar: (from: string, to: string) => ['calendar', from, to] as const,
  forecast: () => ['forecast'] as const,
  balance: () => ['balance'] as const,
  events: (kind?: string) => ['events', kind] as const,
  event: (id: string) => ['events', id] as const,
  settings: () => ['settings'] as const,
  categories: (kind?: string) => ['categories', kind] as const,
  dayCostOverrides: () => ['day-cost-overrides'] as const,
  transactions: (from: string, to: string) => ['transactions', from, to] as const,
};
```

## 8.8 Routing

```typescript
// App.tsx
<Routes>
  <Route path="/login" element={<LoginPage />} />
  <Route element={<ProtectedRoute />}>
    <Route path="/" element={<CalendarPage />} />       {/* главный */}
    <Route path="/settings" element={<SettingsPage />} />
    <Route path="/transactions" element={<TransactionList />} />
  </Route>
</Routes>
```

## 8.9 Money Formatting

```typescript
// lib/money.ts
export function formatMoney(m: MoneyDTO): string {
  const rubles = m.amount / 100;
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: m.currency,
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(rubles);
}

export function parseMoneyInput(value: string): number {
  // "1 400" → 140000 kopecks
  const cleaned = value.replace(/\s/g, '').replace(',', '.');
  return Math.round(parseFloat(cleaned) * 100);
}
```

**Почему форматирование на frontend:** backend отдаёт kopecks; locale-specific formatting — ответственность UI.

## 8.10 Vite Config (proxy)

```typescript
// vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
});
```

## 8.11 Component Diagram (PlantUML)

```plantuml
@startuml frontend-components

package "Pages" {
  [CalendarPage]
  [SettingsPage]
  [LoginPage]
}

package "Features" {
  [CalendarView]
  [DayCell]
  [DayDetailPanel]
  [MoneyForLifeWidget]
  [EventFormModal]
  [AnchorFormModal]
  [DayCostOverrideList]
}

package "Shared" {
  [AppLayout]
  [MoneyDisplay]
  [shadcn/ui]
}

package "Data" {
  [TanStack Query]
  [Zustand]
  [API Client]
}

CalendarPage --> CalendarView
CalendarPage --> MoneyForLifeWidget
CalendarPage --> DayDetailPanel
CalendarPage --> EventFormModal

CalendarView --> DayCell
CalendarView --> TanStack Query

EventFormModal --> TanStack Query
MoneyForLifeWidget --> TanStack Query

SettingsPage --> DayCostOverrideList
SettingsPage --> TanStack Query

TanStack Query --> API Client
EventFormModal --> Zustand

@enduml
```

## 8.12 Расширяемость Frontend

| Feature | Frontend change |
|---------|-----------------|
| GraphQL | Replace `api/client.ts` + hooks; features unchanged if hooks API stable |
| Mobile (React Native) | Shared `api/schemas`; new UI layer |
| Analytics charts | New feature `analytics/` + recharts/visx |
| CSV import | New feature `import/` + file upload |
| Notifications | Service Worker + push; settings toggle |
| Plan vs Fact | New comparison view in `features/analytics/` |

## 8.13 MVP Scope (Frontend)

**In scope:**
- Login by username
- Calendar month view with day balances
- Create/edit/delete events (single + recurring)
- Set balance anchor
- Settings: base day cost, horizon, overrides
- Money for Life widget
- Basic transaction list (manual entry)

**Out of scope (v2):**
- Charts / analytics
- CSV import UI
- Edit single occurrence of recurring event
- Mobile responsive polish (basic responsive, not PWA)
- Dark mode (shadcn supports, but not priority)

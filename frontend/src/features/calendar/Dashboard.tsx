import FullCalendar from '@fullcalendar/react'
import dayGridPlugin from '@fullcalendar/daygrid'
import type { DayCellContentArg, DatesSetArg } from '@fullcalendar/core'
import { endOfMonth, format, parseISO, startOfMonth, subDays } from 'date-fns'
import { ru } from 'date-fns/locale'
import { CalendarDays, Clock3, Coins, Plus, TrendingUp, X } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { ApiError, errorMessage } from '../../api/client'
import { useBalance, useCalendar, useForecast } from '../../api/hooks'
import type { CalendarDay, Money } from '../../api/schemas'
import { Button, ErrorNotice, LoadingBlock } from '../../components/ui'
import { formatDate, toApiDate, today } from '../../lib/dates'
import { horizonPolicyLabels } from '../../lib/horizon'
import { formatMoney } from '../../lib/money'
import { useUIStore } from '../../stores/uiStore'
import { EventModal } from '../events/EventModal'
import { OnboardingModal } from '../balance/OnboardingModal'
import { BalanceModal } from '../balance/BalanceModal'
import { HorizonModal } from '../settings/HorizonModal'
import { SettingsModal } from '../settings/SettingsModal'
import { OccurrenceCard } from './OccurrenceCard'

function StatCard({
  label,
  value,
  icon,
  tone = 'plain',
  onClick,
  hint,
}: {
  label: string
  value: string
  icon: React.ReactNode
  tone?: 'plain' | 'accent'
  onClick?: () => void
  hint?: string
}) {
  const Tag = onClick ? 'button' : 'article'
  return (
    <Tag
      type={onClick ? 'button' : undefined}
      onClick={onClick}
      className={`text-left ${tone === 'accent' ? 'rounded-3xl bg-teal-800 p-5 text-white shadow-sm' : 'rounded-3xl border border-white/80 bg-[#fcfbf8] p-5 shadow-sm'} ${onClick ? 'cursor-pointer transition hover:ring-2 hover:ring-teal-600/25 focus-visible:outline-2 focus-visible:outline-teal-700' : ''}`}
    >
      <div className={tone === 'accent' ? 'mb-5 flex size-9 items-center justify-center rounded-xl bg-white/12 text-amber-300' : 'mb-5 flex size-9 items-center justify-center rounded-xl bg-stone-100 text-stone-600'}>{icon}</div>
      <p className={tone === 'accent' ? 'text-sm text-teal-100' : 'text-sm text-stone-500'}>{label}</p>
      <p className="mt-1 text-2xl font-black tracking-tight">{value}</p>
      {hint ? <p className={`mt-1 text-xs ${tone === 'accent' ? 'text-teal-100/90' : 'text-stone-400'}`}>{hint}</p> : null}
      {onClick ? <p className="mt-1 text-xs text-stone-400">Нажмите, чтобы изменить</p> : null}
    </Tag>
  )
}

function DayCell({ arg, day }: { arg: DayCellContentArg; day?: CalendarDay }) {
  const setSelectedDate = useUIStore((state) => state.setSelectedDate)
  const isToday = toApiDate(arg.date) === today()
  return (
    <button type="button" aria-label={`Открыть ${formatDate(toApiDate(arg.date))}`} onClick={() => setSelectedDate(toApiDate(arg.date))} className="flex min-h-[4.5rem] w-full flex-col p-1 text-left focus-visible:outline-2 focus-visible:outline-teal-700 sm:min-h-[5.25rem]">
      <span className={isToday ? 'grid size-6 place-items-center rounded-full bg-teal-800 text-xs font-bold text-white' : 'text-xs font-bold text-stone-500'}>{arg.dayNumberText}</span>
      {day ? (
        <span className="mt-1 grid w-full flex-1 gap-0.5">
          <span className="flex items-baseline gap-1 truncate text-[10px] leading-tight sm:text-[11px]">
            <span className="shrink-0 font-bold uppercase tracking-wide text-stone-400">Б</span>
            <span className="truncate font-semibold text-stone-700">{day.expected_balance ? formatMoney(day.expected_balance, true) : '—'}</span>
          </span>
          <span className="mt-auto flex min-h-[10px] items-center gap-1">
            {day.events.slice(0, 5).map((event, index) => (
              <i key={`${event.event_id}-${index}`} className={`size-1.5 shrink-0 rounded-full ${event.kind === 'INCOME' ? 'bg-emerald-500' : event.necessity === 'REQUIRED' ? 'bg-red-400' : 'bg-stone-400'}`} />
            ))}
            {day.day_cost.amount > 0 && (
              <span className="ml-auto truncate text-[9px] font-medium text-stone-400 sm:text-[10px]">{formatMoney(day.day_cost, true)}/д</span>
            )}
          </span>
        </span>
      ) : (
        <span className="mt-auto text-[10px] text-stone-300">—</span>
      )}
    </button>
  )
}

function CalendarView({ days, onRange }: { days: CalendarDay[]; onRange: (from: string, to: string) => void }) {
  const byDate = useMemo(() => new Map(days.map((day) => [day.date, day])), [days])
  const [initialDate] = useState(today)
  const datesSet = useCallback((info: DatesSetArg) => onRange(toApiDate(info.start), toApiDate(subDays(info.end, 1))), [onRange])
  return (
    <div className="money-calendar overflow-hidden rounded-3xl border border-white/80 bg-[#fcfbf8] p-2 shadow-sm sm:p-4">
      <FullCalendar
        plugins={[dayGridPlugin]}
        initialView="dayGridMonth"
        initialDate={initialDate}
        now={today()}
        firstDay={1}
        fixedWeekCount={false}
        height="auto"
        headerToolbar={{ left: 'title', center: '', right: 'prev,next today' }}
        buttonText={{ today: 'Сегодня' }}
        titleFormat={(arg) => format(arg.date.marker, 'LLLL yyyy', { locale: ru })}
        datesSet={datesSet}
        dayCellContent={(arg) => <DayCell arg={arg} day={byDate.get(toApiDate(arg.date))} />}
      />
      <div className="flex flex-wrap gap-x-5 gap-y-2 border-t border-stone-100 px-2 pt-3 text-[11px] text-stone-500">
        <span><strong className="text-stone-400">Б</strong> — баланс на конец дня</span>
        <span>₽/д — стоимость дня</span>
        <span><i className="mr-1.5 inline-block size-2 rounded-full bg-emerald-500" />Доход</span>
        <span><i className="mr-1.5 inline-block size-2 rounded-full bg-red-400" />Обязательный расход</span>
        <span><i className="mr-1.5 inline-block size-2 rounded-full bg-stone-400" />Необязательный</span>
      </div>
    </div>
  )
}

function DayDetail({ day }: { day?: CalendarDay }) {
  const selectedDate = useUIStore((state) => state.selectedDate)
  const setSelectedDate = useUIStore((state) => state.setSelectedDate)
  const openEvent = useUIStore((state) => state.openEvent)
  if (!selectedDate) return null

  return (
    <aside className="fixed inset-x-0 bottom-0 z-40 max-h-[76svh] overflow-y-auto rounded-t-3xl border border-stone-200 bg-[#fcfbf8] p-5 shadow-2xl lg:sticky lg:top-20 lg:max-h-[calc(100svh-6rem)] lg:rounded-3xl lg:shadow-sm">
      <div className="flex items-start justify-between">
        <div>
          <p className="text-xs font-bold uppercase tracking-wider text-teal-700">Детали дня</p>
          <h2 className="mt-1 text-xl font-black">{formatDate(selectedDate)}</h2>
        </div>
        <Button variant="ghost" className="size-9 p-0" aria-label="Закрыть" onClick={() => setSelectedDate(null)}><X size={18} /></Button>
      </div>
      <div className="mt-5 grid grid-cols-2 gap-2">
        <div className="rounded-2xl bg-stone-100 p-3"><p className="text-xs text-stone-500">Баланс вечером</p><p className="mt-1 font-black">{day?.expected_balance ? formatMoney(day.expected_balance) : '—'}</p></div>
        <div className="rounded-2xl bg-stone-100 p-3"><p className="text-xs text-stone-500">Стоимость дня</p><p className="mt-1 font-black">{day ? formatMoney(day.day_cost) : '—'}</p></div>
        <div className="rounded-2xl bg-stone-100 p-3"><p className="text-xs text-stone-500">Доходы</p><p className="mt-1 font-black">{day ? formatMoney(day.income_total) : '—'}</p></div>
      </div>
      <div className="mt-6 flex items-center justify-between">
        <h3 className="font-bold">События</h3>
        <Button variant="secondary" className="min-h-9 px-3 py-1" onClick={() => openEvent()}><Plus size={15} />Добавить</Button>
      </div>
      <div className="mt-3 grid gap-2">
        {!day && <p className="rounded-2xl border border-dashed border-stone-300 p-5 text-center text-sm text-stone-500">Нет расчёта для этой даты — операцию всё равно можно добавить</p>}
        {day && day.events.length === 0 && <p className="rounded-2xl border border-dashed border-stone-300 p-5 text-center text-sm text-stone-500">На этот день событий нет</p>}
        {(day?.events ?? []).map((occurrence) => <OccurrenceCard key={`${occurrence.event_id}-${occurrence.occurrence_date}`} occurrence={occurrence} />)}
      </div>
    </aside>
  )
}

function Summary({
  forecast,
  balance,
  onEditHorizon,
  onEditBalance,
}: {
  forecast: { money_for_life: Money; horizon_date: string; days_until_horizon: number; required_remaining: Money; horizon_policy: 'END_OF_MONTH' | 'NEXT_INCOME' | 'CUSTOM_DATE' }
  balance: Money
  onEditHorizon: () => void
  onEditBalance: () => void
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="Деньги на жизнь"
        value={formatMoney(forecast.money_for_life)}
        icon={<TrendingUp size={18} />}
        tone="accent"
        hint={`Прогноз на ${formatDate(forecast.horizon_date)} с учётом доходов и расходов`}
      />
      <StatCard label="Текущий баланс" value={formatMoney(balance)} icon={<Coins size={18} />} onClick={onEditBalance} />
      <StatCard
        label="До горизонта"
        value={`${forecast.days_until_horizon} дн.`}
        icon={<Clock3 size={18} />}
        onClick={onEditHorizon}
      />
      <StatCard label="Обязательные расходы" value={formatMoney(forecast.required_remaining)} icon={<CalendarDays size={18} />} />
    </div>
  )
}

export function Dashboard() {
  const [range, setRange] = useState(() => {
    const now = parseISO(today())
    return { from: toApiDate(startOfMonth(now)), to: toApiDate(endOfMonth(now)) }
  })
  const onRange = useCallback((from: string, to: string) => setRange((old) => old.from === from && old.to === to ? old : { from, to }), [])
  const [onboardingOpen, setOnboardingOpen] = useState(false)
  const forecast = useForecast()
  const balance = useBalance()
  const calendar = useCalendar(range.from, range.to)
  const selectedDate = useUIStore((state) => state.selectedDate)
  const setHorizonOpen = useUIStore((state) => state.setHorizonOpen)
  const setBalanceOpen = useUIStore((state) => state.setBalanceOpen)
  const notice = useUIStore((state) => state.notice)
  const setNotice = useUIStore((state) => state.setNotice)
  const selectedDay = calendar.data?.days.find((day) => day.date === selectedDate)
  const errors = [forecast.error, balance.error, calendar.error]
  const needsAnchor = errors.some((error) => error instanceof ApiError && error.code === 'ANCHOR_REQUIRED')
  const otherError = errors.find((error) => error && !(error instanceof ApiError && error.code === 'ANCHOR_REQUIRED'))
  const loadingSummary = forecast.isLoading || balance.isLoading

  return (
    <>
      <main className="mx-auto max-w-[1480px] px-4 py-5 sm:px-6 sm:py-7">
        <div className="mb-5">
          <p className="text-sm font-semibold text-teal-700">Финансовый календарь</p>
          <h1 className="mt-1 text-2xl font-black tracking-tight sm:text-3xl">План на месяц</h1>
          {forecast.data && (
            <button
              type="button"
              onClick={() => setHorizonOpen(true)}
              className="mt-1 text-left text-sm text-stone-500 underline decoration-stone-300 underline-offset-2 transition hover:text-teal-800 hover:decoration-teal-400"
            >
              Горизонт: {horizonPolicyLabels[forecast.data.horizon_policy].toLowerCase()} · {formatDate(forecast.data.horizon_date)} ({forecast.data.days_until_horizon} дн.)
            </button>
          )}
        </div>
        {otherError && <div className="mb-4"><ErrorNotice>{errorMessage(otherError)}</ErrorNotice></div>}
        {notice && <div role="status" className="mb-4 flex items-center justify-between gap-2 rounded-2xl bg-teal-50 p-4 text-sm"><p>{notice}</p><Button variant="ghost" aria-label="Закрыть уведомление" onClick={() => setNotice(null)}><X size={16} /></Button></div>}
        {needsAnchor && <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-teal-200 bg-teal-50 p-4 text-sm"><p>Баланс пока неизвестен — операции и прошлые даты доступны.</p><Button variant="secondary" onClick={() => setOnboardingOpen(true)}>Указать текущий остаток</Button></div>}
        {balance.data?.needs_balance_review && <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm"><p>Расчёт обновлён. Сверьте текущий остаток. Планы учитываются отдельно от фактов, поэтому прежние расчётные суммы могли измениться.</p><Button variant="secondary" onClick={() => setBalanceOpen(true)}>Сверить остаток</Button></div>}
        {loadingSummary && !needsAnchor ? <LoadingBlock /> : forecast.data && balance.data && (
            <Summary
              forecast={forecast.data}
              balance={balance.data.computed_balance}
              onEditHorizon={() => setHorizonOpen(true)}
              onEditBalance={() => setBalanceOpen(true)}
            />
        )}
        <div className={`mt-5 grid gap-5 ${selectedDate ? 'lg:grid-cols-[minmax(0,1fr)_360px]' : ''}`}>
          <CalendarView days={calendar.data?.days ?? []} onRange={onRange} />
          <DayDetail day={selectedDay} />
        </div>
      </main>
      <OnboardingModal open={needsAnchor && onboardingOpen} onClose={() => setOnboardingOpen(false)} />
      <EventModal />
      <BalanceModal />
      <HorizonModal />
      <SettingsModal />
    </>
  )
}

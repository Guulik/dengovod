import FullCalendar from '@fullcalendar/react'
import dayGridPlugin from '@fullcalendar/daygrid'
import type { DayCellContentArg, DatesSetArg } from '@fullcalendar/core'
import { addMonths, endOfMonth, format, startOfMonth, subDays } from 'date-fns'
import { ru } from 'date-fns/locale'
import { CalendarDays, CircleArrowDown, CircleArrowUp, Clock3, Coins, Plus, TrendingUp, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { ApiError, errorMessage } from '../../api/client'
import { useBalance, useCalendar, useEvents, useForecast } from '../../api/hooks'
import type { CalendarDay, Money } from '../../api/schemas'
import { Button, ErrorNotice, LoadingBlock } from '../../components/ui'
import { formatDate, toApiDate } from '../../lib/dates'
import { formatMoney } from '../../lib/money'
import { useUIStore } from '../../stores/uiStore'
import { EventModal } from '../events/EventModal'
import { OnboardingModal } from '../balance/OnboardingModal'
import { SettingsModal } from '../settings/SettingsModal'

const now = new Date()

function StatCard({ label, value, icon, tone = 'plain' }: { label: string; value: string; icon: React.ReactNode; tone?: 'plain' | 'accent' }) {
  return (
    <article className={tone === 'accent' ? 'rounded-3xl bg-teal-800 p-5 text-white shadow-sm' : 'rounded-3xl border border-white/80 bg-[#fcfbf8] p-5 shadow-sm'}>
      <div className={tone === 'accent' ? 'mb-5 flex size-9 items-center justify-center rounded-xl bg-white/12 text-amber-300' : 'mb-5 flex size-9 items-center justify-center rounded-xl bg-stone-100 text-stone-600'}>{icon}</div>
      <p className={tone === 'accent' ? 'text-sm text-teal-100' : 'text-sm text-stone-500'}>{label}</p>
      <p className="mt-1 text-2xl font-black tracking-tight">{value}</p>
    </article>
  )
}

function DayCell({ arg, day }: { arg: DayCellContentArg; day?: CalendarDay }) {
  const setSelectedDate = useUIStore((state) => state.setSelectedDate)
  const isToday = toApiDate(arg.date) === toApiDate(new Date())
  const mflTone = !day ? '' : day.money_for_life.amount < 0 ? 'text-red-600' : day.money_for_life.amount < day.day_cost.amount * 3 ? 'text-amber-700' : 'text-teal-700'
  return (
    <button type="button" onClick={() => setSelectedDate(toApiDate(arg.date))} className="flex min-h-24 w-full flex-col p-1.5 text-left focus-visible:outline-2 focus-visible:outline-teal-700 sm:min-h-28">
      <span className={isToday ? 'grid size-6 place-items-center rounded-full bg-teal-800 text-xs font-bold text-white' : 'text-xs font-bold text-stone-500'}>{arg.dayNumberText}</span>
      {day && (
        <span className="mt-auto grid w-full gap-1">
          <span className="truncate text-[10px] font-semibold text-stone-500 sm:text-xs">{formatMoney(day.expected_balance, true)}</span>
          <span className={`truncate text-xs font-black sm:text-sm ${mflTone}`}>{formatMoney(day.money_for_life, true)}</span>
          <span className="flex h-2 items-center gap-1">
            {day.events.slice(0, 5).map((event, index) => (
              <i key={`${event.event_id}-${index}`} className={`size-1.5 rounded-full ${event.kind === 'INCOME' ? 'bg-emerald-500' : event.necessity === 'REQUIRED' ? 'bg-red-400' : 'bg-stone-400'}`} />
            ))}
            {day.day_cost.amount > 0 && <span className="ml-auto text-[9px] font-medium text-stone-400">{formatMoney(day.day_cost, true)}/д</span>}
          </span>
        </span>
      )}
    </button>
  )
}

function CalendarView({ days, onRange }: { days: CalendarDay[]; onRange: (from: string, to: string) => void }) {
  const byDate = useMemo(() => new Map(days.map((day) => [day.date, day])), [days])
  const datesSet = (info: DatesSetArg) => onRange(toApiDate(info.start), toApiDate(subDays(info.end, 1)))
  return (
    <div className="money-calendar overflow-hidden rounded-3xl border border-white/80 bg-[#fcfbf8] p-2 shadow-sm sm:p-4">
      <FullCalendar
        plugins={[dayGridPlugin]}
        initialView="dayGridMonth"
        firstDay={1}
        fixedWeekCount={false}
        height="auto"
        headerToolbar={{ left: 'title', center: '', right: 'prev,next today' }}
        buttonText={{ today: 'Сегодня' }}
        titleFormat={(arg) => format(arg.date.marker, 'LLLL yyyy', { locale: ru })}
        datesSet={datesSet}
        dayCellContent={(arg) => <DayCell arg={arg} day={byDate.get(toApiDate(arg.date))} />}
      />
      <div className="flex flex-wrap gap-4 border-t border-stone-100 px-2 pt-3 text-[11px] text-stone-500">
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
  const { data: events } = useEvents()
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
      {!day ? <p className="mt-6 text-sm text-stone-500">Нет расчёта для этой даты.</p> : (
        <>
          <div className="mt-5 grid grid-cols-2 gap-2">
            <div className="rounded-2xl bg-stone-100 p-3"><p className="text-xs text-stone-500">Баланс вечером</p><p className="mt-1 font-black">{formatMoney(day.expected_balance)}</p></div>
            <div className="rounded-2xl bg-teal-50 p-3"><p className="text-xs text-teal-700">Деньги на жизнь</p><p className="mt-1 font-black text-teal-900">{formatMoney(day.money_for_life)}</p></div>
            <div className="rounded-2xl bg-stone-100 p-3"><p className="text-xs text-stone-500">Стоимость дня</p><p className="mt-1 font-black">{formatMoney(day.day_cost)}</p></div>
            <div className="rounded-2xl bg-stone-100 p-3"><p className="text-xs text-stone-500">Доходы</p><p className="mt-1 font-black">{formatMoney(day.income_total)}</p></div>
          </div>
          <div className="mt-6 flex items-center justify-between">
            <h3 className="font-bold">События</h3>
            <Button variant="secondary" className="min-h-9 px-3 py-1" onClick={() => openEvent()}><Plus size={15} />Добавить</Button>
          </div>
          <div className="mt-3 grid gap-2">
            {day.events.length === 0 && <p className="rounded-2xl border border-dashed border-stone-300 p-5 text-center text-sm text-stone-500">На этот день событий нет</p>}
            {day.events.map((occurrence, index) => {
              const source = events?.events.find((event) => event.id === occurrence.event_id)
              return (
                <button key={`${occurrence.event_id}-${index}`} type="button" onClick={() => openEvent(occurrence.event_id)} className="flex w-full items-center gap-3 rounded-2xl border border-stone-200 bg-white p-3 text-left transition hover:border-stone-300">
                  <span className={`grid size-9 shrink-0 place-items-center rounded-xl ${occurrence.kind === 'INCOME' ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-600'}`}>{occurrence.kind === 'INCOME' ? <CircleArrowDown size={18} /> : <CircleArrowUp size={18} />}</span>
                  <span className="min-w-0 flex-1"><span className="block truncate text-sm font-bold">{occurrence.comment || source?.comment || 'Без комментария'}</span><span className="text-xs text-stone-500">{occurrence.necessity === 'OPTIONAL' ? 'Необязательное' : 'Обязательное'}</span></span>
                  <strong className={occurrence.kind === 'INCOME' ? 'text-emerald-700' : 'text-stone-900'}>{occurrence.kind === 'INCOME' ? '+' : '−'}{formatMoney(occurrence.amount)}</strong>
                </button>
              )
            })}
          </div>
        </>
      )}
    </aside>
  )
}

function Summary({ forecast, balance }: { forecast: { money_for_life: Money; horizon_date: string; days_until_horizon: number; required_remaining: Money }; balance: Money }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <StatCard label="Деньги на жизнь" value={formatMoney(forecast.money_for_life)} icon={<TrendingUp size={18} />} tone="accent" />
      <StatCard label="Текущий баланс" value={formatMoney(balance)} icon={<Coins size={18} />} />
      <StatCard label="До горизонта" value={`${forecast.days_until_horizon} дн.`} icon={<Clock3 size={18} />} />
      <StatCard label="Обязательные расходы" value={formatMoney(forecast.required_remaining)} icon={<CalendarDays size={18} />} />
    </div>
  )
}

export function Dashboard() {
  const initialFrom = toApiDate(startOfMonth(now))
  const initialTo = toApiDate(endOfMonth(addMonths(now, 1)))
  const [range, setRange] = useState({ from: initialFrom, to: initialTo })
  const forecast = useForecast()
  const balance = useBalance()
  const calendar = useCalendar(range.from, range.to)
  const selectedDate = useUIStore((state) => state.selectedDate)
  const selectedDay = calendar.data?.days.find((day) => day.date === selectedDate)
  const errors = [forecast.error, balance.error, calendar.error]
  const needsAnchor = errors.some((error) => error instanceof ApiError && error.code === 'ANCHOR_REQUIRED')
  const otherError = errors.find((error) => error && !(error instanceof ApiError && error.code === 'ANCHOR_REQUIRED'))
  const loading = forecast.isLoading || balance.isLoading || calendar.isLoading

  return (
    <>
      <main className="mx-auto max-w-[1480px] px-4 py-5 sm:px-6 sm:py-7">
        <div className="mb-5">
          <p className="text-sm font-semibold text-teal-700">Финансовый календарь</p>
          <h1 className="mt-1 text-2xl font-black tracking-tight sm:text-3xl">План на месяц</h1>
          {forecast.data && <p className="mt-1 text-sm text-stone-500">Горизонт расчёта — {formatDate(forecast.data.horizon_date)}</p>}
        </div>
        {otherError && <div className="mb-4"><ErrorNotice>{errorMessage(otherError)}</ErrorNotice></div>}
        {loading && !needsAnchor ? <LoadingBlock /> : forecast.data && balance.data && (
          <>
            <Summary forecast={forecast.data} balance={balance.data.computed_balance} />
            <div className={`mt-5 grid gap-5 ${selectedDate ? 'lg:grid-cols-[minmax(0,1fr)_360px]' : ''}`}>
              <CalendarView days={calendar.data?.days ?? []} onRange={(from, to) => setRange((old) => old.from === from && old.to === to ? old : { from, to })} />
              <DayDetail day={selectedDay} />
            </div>
          </>
        )}
      </main>
      <OnboardingModal open={needsAnchor} />
      <EventModal />
      <SettingsModal />
    </>
  )
}

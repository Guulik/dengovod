import { act, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Dashboard } from './Dashboard'
import { useUIStore } from '../../stores/uiStore'
import { useCalendar } from '../../api/hooks'
import { ApiError } from '../../api/client'
import type { CalendarDay, Forecast } from '../../api/schemas'
import { useAuthStore } from '../../stores/authStore'
import { userA } from '../../test/fixtures'

const state = vi.hoisted(() => ({
  forecast: { data: undefined as Forecast | undefined, error: null as Error | null, isLoading: true },
  balance: { data: undefined as { computed_balance: { amount: number; currency: 'RUB' }; needs_balance_review: boolean } | undefined, error: null as Error | null, isLoading: true },
  calendar: { data: undefined as { days: CalendarDay[]; summary: Forecast | null } | undefined, error: null as Error | null, isLoading: true },
}))
vi.mock('../../api/hooks', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../api/hooks')>(),
  useForecast: () => state.forecast,
  useBalance: () => state.balance,
  useCalendar: vi.fn(() => state.calendar),
}))
vi.mock('../events/EventModal', () => ({ EventModal: () => null }))
vi.mock('../balance/OnboardingModal', () => ({ OnboardingModal: () => null }))
vi.mock('../balance/BalanceModal', () => ({ BalanceModal: () => null }))
vi.mock('../settings/HorizonModal', () => ({ HorizonModal: () => null }))
vi.mock('../settings/SettingsModal', () => ({ SettingsModal: () => null }))
vi.mock('./OccurrenceCard', () => ({ OccurrenceCard: () => null }))

let client: QueryClient
beforeEach(() => {
  useAuthStore.getState().setAuth('A', userA)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})
const showDashboard = () => render(<Dashboard />, { wrapper: ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider> })

afterEach(() => {
  client.clear()
  useAuthStore.getState().logout()
  vi.useRealTimers()
  useUIStore.getState().reset()
  state.forecast = { data: undefined, error: null, isLoading: true }
  state.balance = { data: undefined, error: null, isLoading: true }
  state.calendar = { data: undefined, error: null, isLoading: true }
  vi.clearAllMocks()
})

describe('calendar remains available during financial loading', () => {
  it.each([
    ['2026-11-14T20:59:00Z', '2026-11-15', /ноябрь 2026/i],
    ['2026-11-30T20:59:00Z', '2026-12-01', /декабрь 2026/i],
    ['2026-12-31T20:59:00Z', '2027-01-01', /январь 2027/i],
  ])('Today reads the current Moscow date after midnight from %s', async (before, expectedDate, expectedTitle) => {
    vi.useFakeTimers().setSystemTime(new Date(before))
    const view = showDashboard()
    fireEvent.click(view.container.querySelector('.fc-prev-button')!)
    await act(async () => { await vi.advanceTimersByTimeAsync(120_000) })
    fireEvent.click(view.container.querySelector('.fc-today-button')!)
    expect(view.container.querySelector('.fc-toolbar-title')).toHaveTextContent(expectedTitle)
    const todayCell = view.container.querySelector<HTMLElement>('.fc-day-today')!
    expect(todayCell.dataset.date).toBe(expectedDate)
    fireEvent.click(todayCell.querySelector('button')!)
    expect(useUIStore.getState().selectedDate).toBe(expectedDate)
  })

  it('preserves the selected month through loading, errors, and query refresh', () => {
    const view = showDashboard()
    const title = () => view.container.querySelector('.fc-toolbar-title')!.textContent
    const initial = title()
    fireEvent.click(view.container.querySelector('.fc-next-button')!)
    const selected = title()
    expect(selected).not.toBe(initial)
    state.calendar = { data: undefined, error: new Error('Network'), isLoading: false }
    view.rerender(<Dashboard />)
    expect(title()).toBe(selected)
    state.calendar = { data: { days: [], summary: null }, error: null, isLoading: false }
    view.rerender(<Dashboard />)
    expect(title()).toBe(selected)
    fireEvent.click(view.container.querySelector('.fc-prev-button')!)
    expect(title()).toBe(initial)
    expect(vi.mocked(useCalendar).mock.calls.length).toBeGreaterThan(2)
  })

  it('allows navigating to a past date and adding an operation before summary or anchor exists', () => {
    state.forecast = { data: undefined, error: new ApiError(409, 'ANCHOR_REQUIRED', 'Нужен остаток'), isLoading: false }
    state.balance = { data: undefined, error: new ApiError(409, 'ANCHOR_REQUIRED', 'Нужен остаток'), isLoading: false }
    const view = showDashboard()
    fireEvent.click(view.container.querySelector('.fc-prev-button')!)
    const cell = view.container.querySelector<HTMLElement>('td[data-date]')!
    const selected = cell.dataset.date!
    fireEvent.click(cell.querySelector('button')!)
    expect(useUIStore.getState().selectedDate).toBe(selected)
    expect(screen.getByText('Баланс пока неизвестен — операции и прошлые даты доступны.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
    expect(useUIStore.getState()).toMatchObject({ eventOpen: true, selectedDate: selected, editingEventId: null })
  })

  it('renders unknown daily amounts as a dash and keeps known zero totals', () => {
    const view = showDashboard()
    const cell = view.container.querySelector<HTMLElement>('td[data-date]')!
    const selected = cell.dataset.date!
    const money = { amount: 0, currency: 'RUB' as const }
    state.calendar.data = { summary: null, days: [{ date: selected, expected_balance: null, money_for_life: null, day_cost: money, income_total: money, required_total: money, optional_total: money, events: [] }] }
    view.rerender(<Dashboard />)
    fireEvent.click(cell.querySelector('button')!)
    const balanceLabel = screen.getByText('Баланс вечером')
    expect(balanceLabel.parentElement).toHaveTextContent('—')
    expect(balanceLabel.parentElement).not.toHaveTextContent('0 ₽')
    expect(screen.getByRole('button', { name: 'Добавить' })).toBeEnabled()
  })
  it('refreshes across midnight and background resume without replacing the selected month or day', async () => {
    vi.useFakeTimers().setSystemTime(new Date('2026-11-30T20:59:59Z'))
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const view = showDashboard()
    const title = () => view.container.querySelector('.fc-toolbar-title')!.textContent
    fireEvent.click(view.container.querySelector('.fc-prev-button')!)
    fireEvent.click(view.container.querySelector('td[data-date="2026-10-10"] button')!)
    const selectedMonth = title()
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(invalidate).toHaveBeenCalledTimes(1)
    expect(title()).toBe(selectedMonth)
    expect(useUIStore.getState().selectedDate).toBe('2026-10-10')
    vi.setSystemTime(new Date('2026-12-07T07:00:00Z'))
    await act(async () => {
      window.dispatchEvent(new Event('pageshow'))
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(title()).toBe(selectedMonth)
    expect(useUIStore.getState().selectedDate).toBe('2026-10-10')
    fireEvent.click(view.container.querySelector('.fc-today-button')!)
    expect(title()).toMatch(/декабрь 2026/i)
    expect(view.container.querySelector<HTMLElement>('.fc-day-today')?.dataset.date).toBe('2026-12-07')
    expect(useUIStore.getState().selectedDate).toBe('2026-10-10')
  })
})

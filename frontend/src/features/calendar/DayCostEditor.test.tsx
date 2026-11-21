import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DayCostEditor } from './DayCostEditor'
import { queryKeys, useCalendar, type OverridePayload } from '../../api/hooks'
import { currentSession, installSessionIsolation } from '../../api/session'
import type { DayCostOverride } from '../../api/schemas'
import { useAuthStore } from '../../stores/authStore'
import { deferred, userA, userB } from '../../test/fixtures'

const date = '2026-11-16'
const money = (amount: number) => ({ amount, currency: 'RUB' as const })
const calendarFor = (amount: number) => ({ summary: null, days: [{
  date, day_cost: money(amount), expected_balance: null, money_for_life: null,
  income_total: money(0), required_total: money(0), optional_total: money(0), events: [],
}] })
const override = (payload: OverridePayload, number = 1): DayCostOverride => ({
  ...payload, id: `66666666-6666-4666-8666-${String(number).padStart(12, '0')}`, created_at: `2026-11-21T12:00:${String(number).padStart(2, '0')}Z`,
})
let client: QueryClient
let stop: () => void
beforeEach(() => {
  useAuthStore.getState().setAuth('A', userA)
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } })
  stop = installSessionIsolation(client)
})
afterEach(() => { cleanup(); stop(); client.clear(); useAuthStore.getState().logout(); vi.unstubAllGlobals() })

function CostHarness() {
  const { data } = useCalendar(date, date)
  return <>
    <p>Действующая стоимость: {data?.days[0].day_cost.amount ?? 'неизвестна'}</p>
    <DayCostEditor date={date} currentCost={data?.days[0].day_cost} />
  </>
}
function SessionHarness() {
  const sessionId = useAuthStore((state) => state.sessionId)
  return <CostHarness key={sessionId} />
}
const show = () => render(<QueryClientProvider client={client}><SessionHarness /></QueryClientProvider>)
const openEditor = async () => {
  await waitFor(() => expect(screen.getByRole('button', { name: 'Изменить стоимость дня' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: 'Изменить стоимость дня' }))
}

describe('calendar single-day cost editing', () => {
  it('creates a new one-day rule on every save, so older day rules never defeat later overlapping periods', async () => {
    const oldDay = override({ date_from: date, date_to: date, daily_amount: money(90000), comment: 'Старое правило дня' })
    const period = override({ date_from: '2026-11-01', date_to: '2026-11-30', daily_amount: money(140000), comment: 'Поздний период' }, 2)
    const rules = [oldDay, period]
    let effectiveCost = 140000
    const fetchMock = vi.fn((url: string, init: RequestInit) => {
      if (init.method === 'POST') {
        const payload: OverridePayload = JSON.parse(init.body as string)
        const saved = override(payload, rules.length + 1)
        rules.push(saved)
        effectiveCost = payload.daily_amount.amount
        return Promise.resolve(Response.json(saved))
      }
      if (url.includes('/calendar')) return Promise.resolve(Response.json(calendarFor(effectiveCost)))
      throw new Error(`Unexpected request ${init.method} ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    const sessionId = currentSession().sessionId
    const balanceKey = queryKeys.balance(sessionId)
    const forecastKey = queryKeys.forecast(sessionId)
    const otherMonthKey = queryKeys.calendar(sessionId, '2026-12-01', '2026-12-31')
    client.setQueryData(balanceKey, { old: true })
    client.setQueryData(forecastKey, { old: true })
    client.setQueryData(otherMonthKey, { old: true })
    const view = show()
    await openEditor()
    expect(screen.getByText(/16 ноября 2026/)).toBeInTheDocument()
    expect(screen.getByLabelText('Стоимость этого дня, ₽')).toHaveValue('1400')
    fireEvent.change(screen.getByLabelText('Стоимость этого дня, ₽'), { target: { value: '1000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить стоимость' }))
    await waitFor(() => expect(screen.getByText('Действующая стоимость: 100000')).toBeInTheDocument())
    await waitFor(() => expect(screen.queryByRole('form')).not.toBeInTheDocument())
    expect(client.getQueryState(balanceKey)?.isInvalidated).toBe(true)
    expect(client.getQueryState(forecastKey)?.isInvalidated).toBe(true)
    expect(client.getQueryState(otherMonthKey)?.isInvalidated).toBe(true)
    // Another later period can supersede the first edit. Editing this day again
    // must append a fresh rule, never PUT the older single-day rule.
    rules.push(override({ ...period, daily_amount: money(220000) }, 4))
    effectiveCost = 220000
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.calendar(sessionId, date, date) }) })
    await openEditor()
    expect(screen.getByLabelText('Стоимость этого дня, ₽')).toHaveValue('2200')
    fireEvent.change(screen.getByLabelText('Стоимость этого дня, ₽'), { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить стоимость' }))
    await waitFor(() => expect(screen.getByText('Действующая стоимость: 0')).toBeInTheDocument())
    view.unmount()
    client.removeQueries({ queryKey: queryKeys.calendar(sessionId, date, date) })
    show()
    await openEditor()
    expect(screen.getByLabelText('Стоимость этого дня, ₽')).toHaveValue('0')
    const writes = fetchMock.mock.calls.filter(([, init]) => init.method === 'POST' || init.method === 'PUT')
    expect(writes).toHaveLength(2)
    for (const [url, init] of writes) {
      expect(url).toBe('/api/v1/day-cost-overrides')
      expect(init.method).toBe('POST')
      expect(JSON.parse(init.body as string)).toMatchObject({ date_from: date, date_to: date })
    }
    expect(oldDay.daily_amount.amount).toBe(90000)
    expect(period.daily_amount.amount).toBe(140000)
  })

  it('allows zero, blocks invalid inputs and repeated submission, and keeps an actionable API error', async () => {
    const response = deferred<Response>()
    let puts = 0
    const fetchMock = vi.fn((_url: string, init: RequestInit) => {
      if (init.method === 'POST') {
        puts++
        return puts === 1 ? response.promise : Promise.resolve(Response.json(override(JSON.parse(init.body as string))))
      }
      return Promise.resolve(Response.json(calendarFor(0)))
    })
    vi.stubGlobal('fetch', fetchMock)
    show()
    await openEditor()
    const input = screen.getByLabelText('Стоимость этого дня, ₽')
    const save = screen.getByRole('button', { name: 'Сохранить стоимость' })
    expect(input).toHaveValue('0')
    expect(save).toBeEnabled()
    for (const value of ['', '-1', '1,001', '1e3', '90071992547409,92']) {
      fireEvent.change(input, { target: { value } })
      expect(save).toBeDisabled()
    }
    fireEvent.change(input, { target: { value: '0' } })
    fireEvent.click(save)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Сохраняем…' })).toBeDisabled())
    expect(input).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Отмена' })).toBeDisabled()
    fireEvent.submit(screen.getByRole('form'))
    expect(puts).toBe(1)
    response.resolve(Response.json({ error: { code: 'CALCULATION_OVERFLOW', message: 'Сумма расчёта слишком велика' } }, { status: 409 }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Сумма расчёта слишком велика'))
    expect(input).toHaveValue('0')
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить стоимость' }))
    await waitFor(() => expect(screen.queryByRole('form')).not.toBeInTheDocument())
  })

  it('aborts an old-profile save and prevents its late response from changing the new profile', async () => {
    const oldResponse = deferred<Response>()
    let oldSignal: AbortSignal | null | undefined
    const fetchMock = vi.fn((_url: string, init: RequestInit) => {
      const oldSession = (init.headers as Record<string, string>).Authorization === 'Bearer A'
      if (init.method === 'POST') { oldSignal = init.signal; return oldResponse.promise }
      return Promise.resolve(Response.json(calendarFor(oldSession ? 140000 : 70000)))
    })
    vi.stubGlobal('fetch', fetchMock)
    show()
    await openEditor()
    fireEvent.change(screen.getByLabelText('Стоимость этого дня, ₽'), { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить стоимость' }))
    await waitFor(() => expect(oldSignal).toBeDefined())
    act(() => useAuthStore.getState().setAuth('B', userB))
    await waitFor(() => expect(screen.getByText('Действующая стоимость: 70000')).toBeInTheDocument())
    expect(oldSignal?.aborted).toBe(true)
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    await act(async () => {
      oldResponse.resolve(Response.json(override({ date_from: date, date_to: date, daily_amount: money(0), comment: '' })))
      await oldResponse.promise
    })
    expect(invalidate).not.toHaveBeenCalled()
    expect(screen.queryByRole('form')).not.toBeInTheDocument()
    await openEditor()
    expect(screen.getByLabelText('Стоимость этого дня, ₽')).toHaveValue('700')
    expect(fetchMock.mock.calls.filter(([, init]) => init.method === 'POST')).toHaveLength(1)
  })
})

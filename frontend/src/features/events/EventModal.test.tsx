import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { EventModal } from './EventModal'
import { useAuthStore } from '../../stores/authStore'
import { useUIStore } from '../../stores/uiStore'
import { installSessionIsolation, currentSession } from '../../api/session'
import { queryKeys } from '../../api/hooks'
import { eventFixture, occurrenceFixture, userA } from '../../test/fixtures'
import { today } from '../../lib/dates'

let client: QueryClient
let stop: () => void
beforeEach(() => {
  useAuthStore.getState().setAuth('token-A', userA)
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } })
  stop = installSessionIsolation(client)
  client.setQueryData(queryKeys.events(currentSession().sessionId), { events: [] })
  client.setQueryData(queryKeys.categories(currentSession().sessionId), { categories: [{ id: eventFixture.id, name: 'Зарплата', kind: 'INCOME', is_default: true, system_code: 'SALARY' }] })
  useUIStore.getState().setSelectedDate(today())
  useUIStore.getState().openEvent()
})
afterEach(() => { stop(); client.clear(); useAuthStore.getState().logout(); vi.unstubAllGlobals() })
const show = () => render(<QueryClientProvider client={client}><EventModal /></QueryClientProvider>)

describe('event entry and first occurrence confirmation', () => {
  it('keeps the payment date and manual edits independent from the salary calculation', async () => {
    show()
    fireEvent.click(screen.getByLabelText('Доход'))
    fireEvent.change(screen.getByLabelText('Категория'), { target: { value: eventFixture.id } })
    const date = screen.getByLabelText('Дата начала')
    const paymentDate = '2026-11-10'
    fireEvent.change(date, { target: { value: paymentDate } })
    fireEvent.change(screen.getByLabelText('Сумма, ₽'), { target: { value: '123' } })
    fireEvent.change(screen.getByLabelText('Оклад за полный месяц, ₽'), { target: { value: '100000' } })
    fireEvent.change(screen.getByLabelText('Расчётный месяц'), { target: { value: '2026-04' } })
    fireEvent.change(screen.getByLabelText('Отработано календарных дней'), { target: { value: '15' } })
    expect(screen.getByLabelText('Сумма, ₽')).toHaveValue('123')
    fireEvent.click(screen.getByRole('button', { name: 'Применить к сумме' }))
    expect(screen.getByLabelText('Сумма, ₽')).toHaveValue('50000')
    expect(date).toHaveValue(paymentDate)
    fireEvent.change(screen.getByLabelText('Сумма, ₽'), { target: { value: '48000,55' } })
    expect(screen.getByLabelText('Сумма, ₽')).toHaveValue('48000,55')
    fireEvent.change(screen.getByLabelText('Отработано календарных дней'), { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: 'Применить к сумме' }))
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))
    await waitFor(() => expect(screen.getByText('Введите положительную сумму в допустимом диапазоне')).toBeInTheDocument())
  })

  it('retries only the same PUT after successful create and failed confirmation', async () => {
    let puts = 0
    const created = { ...eventFixture, kind: 'EXPENSE', recurrence: { ...eventFixture.recurrence, start_date: today() } }
    const fetchMock = vi.fn((url: string, init: RequestInit) => {
      if (init.method === 'POST') return Promise.resolve(Response.json(created))
      if (init.method === 'PUT') {
        puts++
        if (puts === 1) return Promise.reject(new TypeError('Failed to fetch'))
        return Promise.resolve(Response.json({ ...occurrenceFixture, kind: 'EXPENSE', occurrence_date: today(), status: 'CONFIRMED', effective_at: new Date().toISOString(), plan_restored: false }))
      }
      if (url.includes('/categories')) return Promise.resolve(Response.json({ categories: [] }))
      return Promise.resolve(Response.json({ events: [created] }))
    })
    vi.stubGlobal('fetch', fetchMock)
    show()
    fireEvent.change(screen.getByLabelText('Сумма, ₽'), { target: { value: '8700' } })
    fireEvent.change(screen.getByLabelText('Состояние первого вхождения'), { target: { value: 'CONFIRMED' } })
    fireEvent.click(screen.getByLabelText('Повседневная трата — из стоимости дня'))
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Повторить подтверждение' })).toBeEnabled())
    expect(screen.getByText(/Операция создана как план; подтверждение не завершено/)).toBeInTheDocument()
    expect(screen.queryByLabelText('Сумма, ₽')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить подтверждение' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    const posts = fetchMock.mock.calls.filter(([, init]) => init.method === 'POST')
    const confirmations = fetchMock.mock.calls.filter(([, init]) => init.method === 'PUT')
    expect(posts).toHaveLength(1)
    expect(confirmations).toHaveLength(2)
    expect(confirmations[0][0]).toEqual(confirmations[1][0])
    expect(confirmations[0][1].body).toEqual(confirmations[1][1].body)
    expect(confirmations[0][0]).toBe(`/api/v1/events/${created.id}/occurrences/${today()}`)
    expect(JSON.parse(confirmations[0][1].body as string)).toEqual({ status: 'CONFIRMED', counts_toward_day_budget: true })
  })

  it('permits only a plan for future operations', () => {
    show()
    fireEvent.change(screen.getByLabelText('Дата начала'), { target: { value: '9999-12-31' } })
    expect(screen.queryByRole('option', { name: 'Факт — уже исполнено' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Фактические дата и время (Москва)')).not.toBeInTheDocument()
  })
})

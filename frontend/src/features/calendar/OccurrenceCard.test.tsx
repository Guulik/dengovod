import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OccurrenceCard } from './OccurrenceCard'
import { occurrenceFixture, deferred, userA } from '../../test/fixtures'
import { useAuthStore } from '../../stores/authStore'
import { useUIStore } from '../../stores/uiStore'
import { today } from '../../lib/dates'

let client: QueryClient
beforeEach(() => { useAuthStore.getState().setAuth('A', userA); client = new QueryClient({ defaultOptions: { mutations: { retry: false } } }) })
afterEach(() => { client.clear(); useAuthStore.getState().logout(); vi.unstubAllGlobals(); vi.useRealTimers() })
describe('actions target a single occurrence', () => {
  it('captures now at confirmation after midnight and retries the same instant for an existing occurrence', async () => {
    vi.useFakeTimers({ toFake: ['Date'] }).setSystemTime(new Date('2026-11-30T20:59:00.000Z'))
    const occurrence = { ...occurrenceFixture, occurrence_date: '2026-11-30' }
    let puts = 0
    const fetchMock = vi.fn((_url: string, init: RequestInit) => {
      puts++
      if (puts === 1) return Promise.reject(new TypeError('Response lost'))
      return Promise.resolve(Response.json({ ...occurrence, status: 'CONFIRMED', effective_at: JSON.parse(init.body as string).effective_at, plan_restored: false }))
    })
    vi.stubGlobal('fetch', fetchMock)
    render(<QueryClientProvider client={client}><OccurrenceCard occurrence={occurrence} /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Подтвердить исполнение' }))
    expect(screen.getByLabelText('Фактические дата и время (Москва)', { exact: false })).toHaveValue('')
    expect(screen.getByText(/Без времени — исполнить сейчас, в момент нажатия/)).toBeInTheDocument()
    vi.setSystemTime(new Date('2026-11-30T21:01:00.000Z'))
    fireEvent.click(screen.getByRole('button', { name: 'Подтвердить факт' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Повторить подтверждение' })).toBeEnabled())
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({ status: 'CONFIRMED', effective_at: '2026-11-30T21:01:00.000Z' })
    vi.setSystemTime(new Date('2026-11-30T22:01:00.000Z'))
    fireEvent.click(screen.getByRole('button', { name: 'Повторить подтверждение' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    expect(fetchMock.mock.calls[1][0]).toBe(`/api/v1/events/${occurrence.event_id}/occurrences/2026-11-30`)
    expect(fetchMock.mock.calls[1][1].body).toBe(fetchMock.mock.calls[0][1].body)
  })

  it('sends the original occurrence key, blocks repeated confirmation, and invalidates financial queries', async () => {
    const response = deferred<Response>()
    const fetchMock = vi.fn((_url: string, _init: RequestInit) => response.promise)
    vi.stubGlobal('fetch', fetchMock)
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const occurrence = { ...occurrenceFixture, occurrence_date: today(), kind: 'EXPENSE' as const }
    render(<QueryClientProvider client={client}><OccurrenceCard occurrence={occurrence} /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Подтвердить исполнение' }))
    fireEvent.click(screen.getByLabelText('Повседневная трата — из стоимости дня'))
    fireEvent.click(screen.getByRole('button', { name: 'Подтвердить факт' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Подтверждаем…' })).toBeDisabled())
    fireEvent.click(screen.getByRole('button', { name: 'Подтверждаем…' }))
    expect(fetchMock).toHaveBeenCalledTimes(1)
    response.resolve(Response.json({ ...occurrence, status: 'CONFIRMED', effective_at: new Date().toISOString(), plan_restored: false }))
    await waitFor(() => expect(invalidate).toHaveBeenCalledTimes(1))
    expect(fetchMock.mock.calls[0]).toMatchObject([`/api/v1/events/${occurrence.event_id}/occurrences/${today()}`, { method: 'PUT' }])
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({ status: 'CONFIRMED', counts_toward_day_budget: true, effective_at: expect.any(String) })
  })
  it('explains when removing confirmation cannot restore a plan after recurrence changes', async () => {
    const occurrence = { ...occurrenceFixture, status: 'CONFIRMED' as const, effective_at: '2026-11-15T09:00:00Z' }
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(Response.json({ ...occurrence, status: 'PLANNED', effective_at: null, plan_restored: false }))))
    render(<QueryClientProvider client={client}><OccurrenceCard occurrence={occurrence} /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Снять подтверждение' }))
    await waitFor(() => expect(useUIStore.getState().notice).toBe('Подтверждение снято. План не восстановлен: правило повторения изменилось'))
  })
  it('shows deleted-series snapshots without editor or state actions', () => {
    render(<QueryClientProvider client={client}><OccurrenceCard occurrence={{ ...occurrenceFixture, status: 'CONFIRMED', series_deleted: true, comment: 'Сохранённая сумма' }} /></QueryClientProvider>)
    expect(screen.getByText('Сохранённая сумма')).toBeInTheDocument()
    expect(screen.getByText(/Серия удалена/)).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})

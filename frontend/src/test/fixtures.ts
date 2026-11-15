import type { Event, Occurrence, User } from '../api/schemas'

export const userA: User = { id: '11111111-1111-4111-8111-111111111111', login: 'A', created_at: '2026-11-01T00:00:00Z' }
export const userB: User = { ...userA, id: '22222222-2222-4222-8222-222222222222', login: 'B' }
export const eventFixture: Event = {
  id: '33333333-3333-4333-8333-333333333333', kind: 'INCOME', necessity: 'REQUIRED',
  amount: { amount: 870000, currency: 'RUB' }, category_id: null, comment: 'Зарплата',
  recurrence: { type: 'NONE', start_date: '2026-11-15', day_of_month: null, interval_days: null, until_date: null },
  created_at: '2026-11-15T00:00:00Z', updated_at: '2026-11-15T00:00:00Z',
}
export const occurrenceFixture: Occurrence = {
  event_id: eventFixture.id, occurrence_date: '2026-11-15', status: 'PLANNED', effective_at: null,
  counts_toward_day_budget: false, series_deleted: false, kind: eventFixture.kind, necessity: 'REQUIRED',
  amount: eventFixture.amount, category_id: null, comment: eventFixture.comment,
}
export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

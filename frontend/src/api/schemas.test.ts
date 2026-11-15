import { describe, expect, it } from 'vitest'
import { calendarSchema, categorySchema, moneySchema, occurrenceSchema, occurrenceStateSchema } from './schemas'
import { occurrenceFixture } from '../test/fixtures'

describe('API schemas', () => {
  it('accepts integer RUB money only', () => {
    expect(moneySchema.parse({ amount: 12_345, currency: 'RUB' })).toEqual({ amount: 12_345, currency: 'RUB' })
    expect(moneySchema.safeParse({ amount: 12.5, currency: 'RUB' }).success).toBe(false)
    expect(moneySchema.safeParse({ amount: 100, currency: 'USD' }).success).toBe(false)
    expect(moneySchema.safeParse({ amount: Number.MAX_SAFE_INTEGER + 1, currency: 'RUB' }).success).toBe(false)
  })
  it('accepts an unknown calendar balance without discarding known operations', () => {
    const money = { amount: 0, currency: 'RUB' }
    const parsed = calendarSchema.parse({ summary: null, days: [{ date: '2026-11-15', expected_balance: null, money_for_life: null, day_cost: money, income_total: money, required_total: money, optional_total: money, events: [occurrenceFixture] }] })
    expect(parsed.summary).toBeNull()
    expect(parsed.days[0].expected_balance).toBeNull()
    expect(parsed.days[0].events[0].status).toBe('PLANNED')
  })
  it('requires occurrence status, original date, and the mutation restoration result', () => {
    expect(occurrenceSchema.safeParse({ ...occurrenceFixture, occurrence_date: undefined }).success).toBe(false)
    expect(occurrenceSchema.safeParse({ ...occurrenceFixture, status: undefined }).success).toBe(false)
    expect(occurrenceStateSchema.safeParse(occurrenceFixture).success).toBe(false)
    expect(occurrenceStateSchema.parse({ ...occurrenceFixture, plan_restored: false }).plan_restored).toBe(false)
  })
  it('preserves optional and nullable system category codes for old API compatibility', () => {
    const category = { id: occurrenceFixture.event_id, name: 'Зарплата', kind: 'INCOME', is_default: true }
    expect(categorySchema.parse(category).system_code).toBeUndefined()
    expect(categorySchema.parse({ ...category, system_code: null }).system_code).toBeNull()
    expect(categorySchema.parse({ ...category, system_code: 'SALARY' }).system_code).toBe('SALARY')
  })

  it('parses a calendar envelope with explicit day array order', () => {
    const money = { amount: 0, currency: 'RUB' }
    const summary = {
      current_balance: money,
      money_for_life: money,
      horizon_date: '2026-07-31',
      horizon_policy: 'END_OF_MONTH',
      required_remaining: money,
      day_cost_remaining: money,
      optional_remaining: money,
      days_until_horizon: 19,
    }
    const parsed = calendarSchema.parse({ days: [], summary })
    expect(parsed.days).toEqual([])
    expect(parsed.summary?.horizon_date).toBe('2026-07-31')
  })
})

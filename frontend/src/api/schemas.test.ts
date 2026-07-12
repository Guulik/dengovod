import { describe, expect, it } from 'vitest'
import { calendarSchema, moneySchema } from './schemas'

describe('API schemas', () => {
  it('accepts integer RUB money only', () => {
    expect(moneySchema.parse({ amount: 12_345, currency: 'RUB' })).toEqual({ amount: 12_345, currency: 'RUB' })
    expect(moneySchema.safeParse({ amount: 12.5, currency: 'RUB' }).success).toBe(false)
    expect(moneySchema.safeParse({ amount: 100, currency: 'USD' }).success).toBe(false)
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
    expect(parsed.summary.horizon_date).toBe('2026-07-31')
  })
})

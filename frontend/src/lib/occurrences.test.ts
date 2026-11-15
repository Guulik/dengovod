import { afterEach, describe, expect, it, vi } from 'vitest'
import { moscowDate, today, toApiDate } from './dates'
import { defaultEffectiveTime, effectiveTimeToApi, firstOccurrenceDate, moscowDateTime } from './occurrences'

afterEach(() => vi.useRealTimers())
describe('Moscow financial dates', () => {
  it('uses the Moscow date at midnight boundaries regardless of browser timezone', () => {
    vi.useFakeTimers().setSystemTime(new Date('2026-11-14T21:30:45Z'))
    expect(today()).toBe('2026-11-15')
    expect(moscowDate(new Date('2026-11-14T20:59:59Z'))).toBe('2026-11-14')
    expect(moscowDateTime()).toBe('2026-11-15T00:30:45')
    expect(defaultEffectiveTime('2026-11-15')).toBe('')
    expect(defaultEffectiveTime('2026-11-14')).toBe('2026-11-14T12:00')
  })
  it('keeps FullCalendar date-only values as calendar dates', () => {
    expect(toApiDate(new Date(2026, 10, 15))).toBe('2026-11-15')
  })
  it('converts explicit Moscow execution time without using the browser UTC offset', () => {
    const now = new Date('2026-11-15T12:00:00Z')
    expect(effectiveTimeToApi('2026-11-14T12:00', now)).toBe('2026-11-14T09:00:00.000Z')
    expect(effectiveTimeToApi('2026-11-15T15:00:00', now)).toBe(now.toISOString())
    expect(effectiveTimeToApi('', now)).toBe(now.toISOString())
    expect(effectiveTimeToApi('0001-01-01T12:00', now)).toBe('0001-01-01T09:00:00.000Z')
    expect(() => effectiveTimeToApi('2026-11-15T15:00:01', now)).toThrow('будущем')
    expect(() => effectiveTimeToApi('2026-02-29T12:00', now)).toThrow()
    expect(() => effectiveTimeToApi('0000-01-01T12:00', now)).toThrow()
    expect(() => effectiveTimeToApi('2026-11-15T24:00', now)).toThrow()
  })
  it('uses the first valid recurrence date when confirming a newly created series', () => {
    const rule = { type: 'MONTHLY_ON_DAY' as const, start_date: '2026-02-15', day_of_month: 31, interval_days: null, until_date: null }
    expect(firstOccurrenceDate(rule)).toBe('2026-02-28')
    expect(firstOccurrenceDate({ ...rule, day_of_month: 10 })).toBe('2026-03-10')
    expect(firstOccurrenceDate({ ...rule, day_of_month: 10, until_date: '2026-02-28' })).toBeUndefined()
    expect(firstOccurrenceDate({ ...rule, start_date: '9999-12-31', day_of_month: 10 })).toBeUndefined()
  })
})

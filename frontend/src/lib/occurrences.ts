import { calendarDaysInMonth } from './salary'
import { today } from './dates'
import type { Event } from '../api/schemas'

export function firstOccurrenceDate(rule: Event['recurrence']): string | undefined {
  if (!/^(?!0000)\d{4}-\d{2}-\d{2}$/.test(rule.start_date)) return undefined
  let result = rule.start_date
  try {
    let [year, month, day] = rule.start_date.split('-').map(Number)
    if (day < 1 || day > calendarDaysInMonth(rule.start_date.slice(0, 7))) return undefined
    if (rule.type === 'MONTHLY_ON_DAY') {
      if (!Number.isInteger(rule.day_of_month) || !rule.day_of_month || rule.day_of_month < 1 || rule.day_of_month > 31) return undefined
      const dateAt = () => `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}`
      day = Math.min(rule.day_of_month, calendarDaysInMonth(dateAt()))
      result = `${dateAt()}-${String(day).padStart(2, '0')}`
      if (result < rule.start_date) {
        if (month === 12) { year++; month = 1 } else month++
        if (year > 9999) return undefined
        day = Math.min(rule.day_of_month, calendarDaysInMonth(dateAt()))
        result = `${dateAt()}-${String(day).padStart(2, '0')}`
      }
    }
  } catch { return undefined }
  return rule.until_date && rule.until_date < result ? undefined : result
}

export function moscowDateTime(date = new Date()): string {
  const parts = new Intl.DateTimeFormat('sv-SE', {
    timeZone: 'Europe/Moscow', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
  }).formatToParts(date)
  const value = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value
  return `${value('year')}-${value('month')}-${value('day')}T${value('hour')}:${value('minute')}:${value('second')}`
}

export function defaultEffectiveTime(occurrenceDate: string): string {
  return occurrenceDate === today() ? '' : `${occurrenceDate}T12:00`
}

export function effectiveTimeToApi(value: string, now = new Date()): string {
  // Empty means the user's current instant, captured before any create/refetch awaits.
  // Sending it explicitly also keeps that intent when the request crosses Moscow midnight.
  if (!value) return now.toISOString()
  const valid = /^((?!0000)\d{4})-(\d{2})-(\d{2})T([01]\d|2[0-3]):([0-5]\d)(?::([0-5]\d))?$/.exec(value)
  if (!valid || Number(valid[1]) < 1) throw new Error('Укажите корректные дату и время исполнения')
  const [, year, month, day] = valid
  // Gregorian month validation, including early years, without UTC date-only parsing.
  const length = calendarDaysInMonth(`${year}-${month}`)
  if (Number(day) < 1 || Number(day) > length) throw new Error('Укажите корректную дату исполнения')
  const result = new Date(`${value.length === 16 ? `${value}:00` : value}+03:00`)
  if (!Number.isFinite(result.getTime())) throw new Error('Укажите корректные дату и время исполнения')
  if (result > now) throw new Error('Фактическое исполнение не может быть в будущем')
  return result.toISOString()
}

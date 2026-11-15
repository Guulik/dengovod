import { describe, expect, it } from 'vitest'
import { calculateSalary, calendarDaysInMonth, countWorkedDates, isSalaryCategory, parseSalaryRubles } from './salary'
import type { Category } from '../api/schemas'

describe('salary calculator: calendar days and exact kopecks', () => {
  it.each([['2026-02', 28], ['2028-02', 29], ['2100-02', 28], ['2000-02', 29], ['2026-04', 30], ['2026-07', 31]])('uses the Gregorian length of %s', (month, days) => {
    expect(calendarDaysInMonth(month)).toBe(days)
    expect(calculateSalary(10_000_000, days, month)).toBe(10_000_000)
  })
  it('calculates 100000 rubles × 15 / 30', () => {
    expect(calculateSalary(parseSalaryRubles('100 000'), 15, '2026-04')).toBe(5_000_000)
  })
  it('rounds only the final amount half up, with no float multiplication', () => {
    expect(calculateSalary(1, 15, '2026-04')).toBe(1)
    expect(calculateSalary(1, 14, '2026-04')).toBe(0)
    expect(calculateSalary(10001, 15, '2026-04')).toBe(5001)
    expect(calculateSalary(Number.MAX_SAFE_INTEGER, 31, '2026-07')).toBe(Number.MAX_SAFE_INTEGER)
    expect(calculateSalary(Number.MAX_SAFE_INTEGER, 15, '2026-04')).toBe(4503599627370496)
  })
  it('counts unique valid dates and allows zero worked days', () => {
    expect(countWorkedDates('2028-02', ['2028-02-01', '2028-02-01', '2028-02-29'])).toBe(2)
    expect(calculateSalary(10000, 0, '2026-02')).toBe(0)
    expect(countWorkedDates('2026-02', [])).toBe(0)
  })
  it.each([['2026-02', ['2026-02-29']], ['2028-02', ['2028-03-01']], ['2026-04', ['2026-04-00']], ['2026-04', ['2026-4-01']]])('rejects dates outside %s', (month, dates) => {
    expect(() => countWorkedDates(month, dates)).toThrow()
  })
  it.each([-1, 0.5, 31, Number.POSITIVE_INFINITY, Number.NaN, Number.MAX_SAFE_INTEGER + 1])('rejects invalid worked day count %s', (days) => {
    expect(() => calculateSalary(10000, days, '2026-04')).toThrow()
  })
  it.each(['', '-1', '0', '1.001', '1e3', 'Infinity', 'NaN', '90071992547409.92', '1,2.3', '\t12'])('rejects invalid or unsafe salary %s', (salary) => {
    expect(() => parseSalaryRubles(salary)).toThrow()
  })
  it('parses decimal kopecks exactly up to the safe integer limit', () => {
    expect(parseSalaryRubles('0,01')).toBe(1)
    expect(parseSalaryRubles('90 071 992 547 409,91')).toBe(Number.MAX_SAFE_INTEGER)
    expect(() => calculateSalary(Number.MAX_SAFE_INTEGER + 1, 1, '2026-04')).toThrow()
  })
  it('requires stable salary identity or a strictly scoped legacy default', () => {
    const base: Category = { id: 'salary', name: 'Зарплата', kind: 'INCOME', is_default: true }
    expect(isSalaryCategory(base)).toBe(true)
    expect(isSalaryCategory({ ...base, system_code: null })).toBe(true)
    expect(isSalaryCategory({ ...base, name: 'Другой заголовок', system_code: 'SALARY' })).toBe(true)
    expect(isSalaryCategory({ ...base, is_default: false })).toBe(false)
    expect(isSalaryCategory({ ...base, kind: 'EXPENSE' })).toBe(false)
    expect(isSalaryCategory({ ...base, name: 'зарплата' })).toBe(false)
    expect(isSalaryCategory({ ...base, system_code: 'OTHER' })).toBe(false)
  })
})

import type { Category } from '../api/schemas'

export function isSalaryCategory(category?: Category): boolean {
  if (!category || category.kind !== 'INCOME') return false
  if (category.system_code != null) return category.system_code === 'SALARY'
  // Compatibility with the old API only; custom categories with the same name do not qualify.
  return category.is_default && category.name === 'Зарплата'
}

export function calendarDaysInMonth(month: string): number {
  if (!/^(?!0000)\d{4}-(0[1-9]|1[0-2])$/.test(month)) throw new Error('Выберите расчётный месяц')
  const [year, number] = month.split('-').map(Number)
  if (number === 2) return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28
  return [4, 6, 9, 11].includes(number) ? 30 : 31
}

export function parseSalaryRubles(value: string): number {
  const normalized = value.replace(/[ \u00a0\u202f]/g, '').replace(',', '.')
  if (!/^\d+(\.\d{1,2})?$/.test(normalized)) throw new Error('Введите положительный оклад, не больше двух знаков после запятой')
  const [whole, fraction = ''] = normalized.split('.')
  const amount = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0'))
  if (amount <= 0n || amount > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('Оклад должен быть положительным и не превышать безопасный диапазон суммы')
  return Number(amount)
}

export function countWorkedDates(month: string, dates: readonly string[]): number {
  const length = calendarDaysInMonth(month)
  const unique = new Set(dates)
  for (const date of unique) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || date.slice(0, 7) !== month || Number(date.slice(8)) < 1 || Number(date.slice(8)) > length) {
      throw new Error('Все отработанные даты должны быть внутри расчётного месяца')
    }
  }
  return unique.size
}

export function calculateSalary(salaryKopecks: number, workedDays: number, month: string): number {
  const days = calendarDaysInMonth(month)
  if (!Number.isSafeInteger(salaryKopecks) || salaryKopecks <= 0) throw new Error('Оклад выходит за допустимый диапазон суммы')
  if (!Number.isInteger(workedDays) || workedDays < 0 || workedDays > days) throw new Error(`Укажите целое число дней от 0 до ${days}`)
  // BigInt preserves the exact numerator even if salary × days exceeds MAX_SAFE_INTEGER.
  const numerator = BigInt(salaryKopecks) * BigInt(workedDays)
  const denominator = BigInt(days)
  const rounded = (2n * numerator + denominator) / (2n * denominator)
  if (rounded > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('Результат выходит за допустимый диапазон суммы')
  return Number(rounded)
}

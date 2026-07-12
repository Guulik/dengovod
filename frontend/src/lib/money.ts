import type { Money } from '../api/schemas'

export function formatMoney(money: Money, compact = false): string {
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: 'RUB',
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
    notation: compact ? 'compact' : 'standard',
  }).format(money.amount / 100)
}

export function parseRubles(value: string): number {
  const normalized = value.replace(/\s/g, '').replace(',', '.')
  if (normalized === '') return 0
  const amount = Number(normalized)
  if (!Number.isFinite(amount)) return Number.NaN
  return Math.round(amount * 100)
}

export function kopecksToInput(amount: number): string {
  return String(amount / 100).replace('.', ',')
}

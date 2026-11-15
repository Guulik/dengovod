import type { Money } from '../api/schemas'

export function formatMoney(money: Money, compact = false): string {
  if (!compact) {
    const [whole, fraction] = kopecksToInput(money.amount).split(',')
    const integer = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 0 }).format(BigInt(whole))
    const sign = money.amount < 0 && whole === '-0' ? '-' : ''
    return `${sign}${integer}${fraction ? `,${fraction}` : ''}\u00a0₽`
  }
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
  if (!/^-?\d+(\.\d{1,2})?$/.test(normalized)) return Number.NaN
  const negative = normalized.startsWith('-')
  const [whole, fraction = ''] = normalized.replace(/^-/, '').split('.')
  const amount = (BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0'))) * (negative ? -1n : 1n)
  if (amount > BigInt(Number.MAX_SAFE_INTEGER) || amount < BigInt(Number.MIN_SAFE_INTEGER)) return Number.NaN
  return Number(amount)
}

export function kopecksToInput(amount: number): string {
  const integer = BigInt(amount)
  const absolute = integer < 0n ? -integer : integer
  const fraction = (absolute % 100n).toString().padStart(2, '0').replace(/0+$/, '')
  return `${integer < 0n ? '-' : ''}${absolute / 100n}${fraction ? `,${fraction}` : ''}`
}

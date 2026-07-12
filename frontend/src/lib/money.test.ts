import { describe, expect, it } from 'vitest'
import { formatMoney, parseRubles } from './money'

describe('money utilities', () => {
  it('converts Russian ruble input to integer kopecks', () => {
    expect(parseRubles('1 400,50')).toBe(140_050)
    expect(parseRubles('99.99')).toBe(9_999)
  })

  it('formats kopecks as rubles', () => {
    expect(formatMoney({ amount: 140_050, currency: 'RUB' })).toContain('1 400,5')
  })
})

import { describe, expect, it } from 'vitest'
import { formatMoney, kopecksToInput, parseRubles } from './money'

describe('money utilities', () => {
  it('converts Russian ruble input to integer kopecks', () => {
    expect(parseRubles('1 400,50')).toBe(140_050)
    expect(parseRubles('99.99')).toBe(9_999)
  })

  it('formats kopecks as rubles', () => {
    expect(formatMoney({ amount: 140_050, currency: 'RUB' })).toContain('1 400,5')
  })
  it('keeps exact kopecks through the whole safe range and rejects overflow', () => {
    expect(kopecksToInput(Number.MAX_SAFE_INTEGER)).toBe('90071992547409,91')
    expect(parseRubles(kopecksToInput(Number.MAX_SAFE_INTEGER))).toBe(Number.MAX_SAFE_INTEGER)
    expect(parseRubles(kopecksToInput(-12345))).toBe(-12345)
    expect(parseRubles('90071992547409,92')).toBeNaN()
    expect(parseRubles('1,001')).toBeNaN()
    expect(parseRubles('1e3')).toBeNaN()
    expect(formatMoney({ amount: Number.MAX_SAFE_INTEGER, currency: 'RUB' })).toContain('90 071 992 547 409,91')
    expect(formatMoney({ amount: -1, currency: 'RUB' })).toBe('-0,01 ₽')
  })
})

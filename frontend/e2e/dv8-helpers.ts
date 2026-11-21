import { expect, type APIRequestContext, type Page } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { calendar, dayCell, dayDetail, moscowToday, uniqueLogin } from './helpers'

export function shiftDate(date: string, days: number) {
  const result = new Date(`${date}T12:00:00Z`)
  result.setUTCDate(result.getUTCDate() + days)
  return result.toISOString().slice(0, 10)
}

export async function seedDayCosts(request: APIRequestContext, withAnchor = true) {
  const today = moscowToday()
  const start = shiftDate(today, -2)
  const yesterday = shiftDate(today, -1)
  const tomorrow = shiftDate(today, 1)
  const login = uniqueLogin()
  const auth = await request.post('/api/v1/auth/login', { data: { login } })
  expect(auth.ok()).toBeTruthy()
  const { token } = await auth.json()
  const headers = { Authorization: `Bearer ${token}` }
  const settings = await request.put('/api/v1/settings', {
    headers,
    data: { base_day_cost: { amount: 0, currency: 'RUB' }, horizon_policy: 'CUSTOM_DATE', horizon_custom_date: today },
  })
  expect(settings.ok()).toBeTruthy()
  let broadOverrideId: string | null = null
  if (withAnchor) {
    const override = await request.post('/api/v1/day-cost-overrides', {
      headers,
      data: { date_from: start, date_to: tomorrow, daily_amount: { amount: 140000, currency: 'RUB' }, comment: 'DV8: исходный многодневный период' },
    })
    expect(override.ok()).toBeTruthy()
    broadOverrideId = (await override.json()).id
    const anchor = await request.post('/api/v1/balance/anchor', {
      headers,
      data: { amount: { amount: 2000000, currency: 'RUB' }, anchored_at: `${start}T09:00:00+03:00` },
    })
    expect(anchor.ok()).toBeTruthy()
  }
  return { login, token, headers, today, start, yesterday, tomorrow, broadOverrideId }
}

export type DayCostProfile = Awaited<ReturnType<typeof seedDayCosts>>

export async function selectDay(page: Page, date: string) {
  if (await dayDetail(page).isVisible()) await dayDetail(page).getByRole('button', { name: 'Закрыть', exact: true }).click()
  await expect(calendar(page)).toBeVisible()
  for (let attempts = 0; attempts < 3; attempts++) {
    const current = await page.locator('.fc-daygrid-day:not(.fc-day-other)[data-date]').first().getAttribute('data-date')
    expect(current).not.toBeNull()
    if (current!.slice(0, 7) === date.slice(0, 7)) break
    await calendar(page).locator(current!.slice(0, 7) < date.slice(0, 7) ? '.fc-next-button' : '.fc-prev-button').click()
  }
  await expect(page.locator(`.fc-daygrid-day[data-date="${date}"]`)).not.toHaveClass(/fc-day-other/)
  await dayCell(page, date).click()
  await expect(dayDetail(page)).toBeVisible()
}

export function dayMetric(page: Page, label: string) {
  return dayDetail(page).getByText(label, { exact: true }).locator('..')
}

export function rublesPattern(amount: number) {
  const digits = String(amount).replace(/\B(?=(\d{3})+(?!\d))/g, '\\s*')
  return new RegExp(`(?<!\\d)${digits}\\s*₽`)
}

export async function openDayCost(page: Page, expectedRubles: string) {
  await expect(dayMetric(page, 'Стоимость дня')).toContainText(rublesPattern(Number(expectedRubles)))
  await dayDetail(page).getByRole('button', { name: 'Изменить стоимость дня', exact: true }).click()
  const editor = dayDetail(page).getByRole('form', { name: 'Стоимость выбранного дня' })
  await expect(editor).toBeVisible()
  const amount = editor.getByLabel('Стоимость этого дня, ₽')
  await expect(amount).toHaveValue(expectedRubles)
  return { editor, amount, save: editor.getByRole('button', { name: /^Сохранить|Сохраня/ }) }
}

export async function saveDayCost(page: Page, date: string, input: string, initial: string, holdResponse = false) {
  const controls = await openDayCost(page, initial)
  await controls.amount.fill(input)
  let release: () => void = () => {}
  let arrived!: () => void
  const received = new Promise<void>((done) => { arrived = done })
  const gate = new Promise<void>((done) => { release = done })
  if (holdResponse) {
    await page.route('**/api/v1/day-cost-overrides**', async (route) => {
      if (!['POST', 'PUT'].includes(route.request().method())) return route.continue()
      const response = await route.fetch()
      arrived()
      await gate
      await route.fulfill({ response })
    })
  }
  try {
    const responsePromise = page.waitForResponse((response) => response.url().includes('/api/v1/day-cost-overrides') && ['POST', 'PUT'].includes(response.request().method()))
    await controls.save.click()
    if (holdResponse) {
      await received
      await expect(controls.save).toBeDisabled()
      await expect(controls.editor).toBeVisible()
      release()
    }
    const response = await responsePromise
    expect(response.ok()).toBeTruthy()
    expect(response.request().method()).toBe('POST')
    const saved = await response.json()
    expect(saved.date_from).toBe(date)
    expect(saved.date_to).toBe(date)
    expect(saved.daily_amount.amount).toBe(Number(input) * 100)
    await expect(controls.editor).toBeHidden()
  } finally {
    release()
    if (holdResponse) await page.unroute('**/api/v1/day-cost-overrides**')
  }
}

export async function captureDayEditor(page: Page, date: string, width: number, amount: string) {
  const directory = resolve('e2e/.artifacts/visual')
  await mkdir(directory, { recursive: true })
  const controls = await openDayCost(page, amount)
  await controls.editor.screenshot({ path: resolve(directory, `dv8-editor-${width}.png`) })
  await controls.editor.getByRole('button', { name: 'Отмена', exact: true }).click()
  await dayDetail(page).getByRole('button', { name: 'Закрыть', exact: true }).click()
  await page.getByRole('button', { name: /Текущий баланс/ }).screenshot({ path: resolve(directory, `dv8-balance-${width}.png`) })
  await selectDay(page, date)
}

export async function verifyFinancials(request: APIRequestContext, profile: DayCostProfile, selected: string, expected: { balance: number; cost: number; evening: number; life: number }) {
  const responses = await Promise.all([
    request.get('/api/v1/balance', { headers: profile.headers }),
    request.get('/api/v1/forecast', { headers: profile.headers }),
    request.get(`/api/v1/calendar?from=${selected}&to=${selected}`, { headers: profile.headers }),
    request.get('/api/v1/events', { headers: profile.headers }),
  ])
  for (const response of responses) expect(response.ok()).toBeTruthy()
  const [balance, forecast, calendarData, events] = await Promise.all(responses.map((response) => response.json()))
  expect(balance.computed_balance.amount).toBe(expected.balance * 100)
  expect(forecast.current_balance.amount).toBe(expected.balance * 100)
  expect(forecast.money_for_life.amount).toBe(expected.life * 100)
  expect(calendarData.summary.current_balance.amount).toBe(expected.balance * 100)
  const day = calendarData.days.find((item: { date: string }) => item.date === selected)
  expect(day.day_cost.amount).toBe(expected.cost * 100)
  expect(day.expected_balance.amount).toBe(expected.evening * 100)
  expect(day.events).toEqual([])
  expect(events.events).toEqual([])
}

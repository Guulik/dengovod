import { expect, type APIRequestContext, type Page } from '@playwright/test'
import { randomUUID } from 'node:crypto'

export const uniqueLogin = () => `e2e-${randomUUID().slice(0, 12)}`
export const moscowToday = () => new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Moscow', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())

export function monthOffset(date: string, offset: number) {
  const [year, month] = date.split('-').map(Number)
  const value = new Date(year, month - 1 + offset, 1, 12)
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-01`
}

export async function loginUI(page: Page, login: string) {
  await page.goto('/')
  await page.getByRole('textbox', { name: 'Имя пользователя' }).fill(login)
  await page.getByRole('button', { name: 'Продолжить', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'План на месяц' })).toBeVisible()
}

export async function seedProfile(request: APIRequestContext, balanceRubles: number) {
  const login = uniqueLogin()
  const response = await request.post('/api/v1/auth/login', { data: { login } })
  expect(response.ok()).toBeTruthy()
  const { token } = await response.json()
  const headers = { Authorization: `Bearer ${token}` }
  const settings = await request.put('/api/v1/settings', {
    headers,
    data: { base_day_cost: { amount: 0, currency: 'RUB' }, horizon_policy: 'END_OF_MONTH', horizon_custom_date: null },
  })
  expect(settings.ok()).toBeTruthy()
  const anchor = await request.post('/api/v1/balance/anchor', { headers, data: { amount: { amount: balanceRubles * 100, currency: 'RUB' }, anchored_at: new Date().toISOString() } })
  expect(anchor.ok()).toBeTruthy()
  return { login, token, headers }
}

export const calendar = (page: Page) => page.locator('.money-calendar')
export const dayCell = (page: Page, date: string) => page.locator(`.fc-daygrid-day[data-date="${date}"] button`)
export const dayDetail = (page: Page) => page.locator('aside').filter({ hasText: 'Детали дня' })

export async function dismissOptionalOnboarding(page: Page) {
  const skip = page.getByRole('button', { name: /позже|пропустить|без баланса|без настройки/i })
  if (await skip.isVisible()) await skip.click()
}

export async function createExpenseUI(page: Page, comment: string, amount = '1500') {
  await dayDetail(page).getByRole('button', { name: 'Добавить', exact: true }).click()
  const modal = page.getByRole('dialog', { name: 'Новое событие' })
  await modal.getByLabel('Сумма, ₽', { exact: true }).fill(amount)
  await modal.getByLabel('Комментарий', { exact: true }).fill(comment)
  await modal.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(modal).toBeHidden()
}

export async function assertNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
}

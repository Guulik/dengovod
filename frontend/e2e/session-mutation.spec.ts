import { expect, test } from '@playwright/test'
import { dayCell, dayDetail, loginUI, moscowToday, seedProfile, uniqueLogin } from './helpers'

test('поздний PUT настроек A не меняет B и не закрывает новый редактор B', async ({ page, request }) => {
  const a = await seedProfile(request, 10000)
  const b = await seedProfile(request, 20000)
  const markerB = `Черновик профиля B ${uniqueLogin()}`
  await loginUI(page, a.login)
  await page.getByRole('button', { name: 'Настройки', exact: true }).click()
  const settings = page.getByRole('dialog', { name: 'Настройки прогноза' })
  await settings.getByLabel('Базовая стоимость дня, ₽').fill('400')
  const editor = page.getByRole('dialog', { name: 'Новое событие' })
  let release!: () => void
  let captured!: () => void
  let delivered!: () => void
  const gate = new Promise<void>((done) => { release = done })
  const received = new Promise<void>((done) => { captured = done })
  const finished = new Promise<void>((done) => { delivered = done })
  await page.route('**/api/v1/settings', async (route) => {
    if (route.request().method() !== 'PUT' || route.request().headers().authorization !== `Bearer ${a.token}`) return route.continue()
    const response = await route.fetch()
    expect(response.ok()).toBeTruthy()
    expect((await response.json()).base_day_cost.amount).toBe(40000)
    captured()
    await gate
    await route.fulfill({ response }).catch(() => {})
    delivered()
  })
  const confirmations: string[] = []
  page.on('request', (req) => { if (req.method() === 'PUT' && req.url().includes('/occurrences/')) confirmations.push(req.url()) })
  try {
    await settings.getByRole('button', { name: 'Сохранить настройки', exact: true }).click()
    await received
    await settings.getByRole('button', { name: 'Закрыть', exact: true }).click()
    await page.getByRole('button', { name: 'Выйти', exact: true }).click()
    await page.getByRole('textbox', { name: 'Имя пользователя' }).fill(b.login)
    await page.getByRole('button', { name: 'Продолжить', exact: true }).click()
    await expect(page.getByRole('button', { name: /Текущий баланс/ })).toContainText(/20\s*000/)
    await dayCell(page, moscowToday()).click()
    await dayDetail(page).getByRole('button', { name: 'Добавить', exact: true }).click()
    await editor.getByLabel('Сумма, ₽', { exact: true }).fill('555')
    await editor.getByLabel('Комментарий', { exact: true }).fill(markerB)
    release()
    await finished
    await expect(editor).toBeVisible()
    await expect(editor.getByLabel('Сумма, ₽', { exact: true })).toHaveValue('555')
    await expect(editor.getByLabel('Комментарий', { exact: true })).toHaveValue(markerB)
    expect(confirmations).toEqual([])
    await editor.getByRole('button', { name: 'Сохранить', exact: true }).click()
    await expect(editor).toBeHidden()
    await expect(dayDetail(page)).toContainText(markerB)
    await expect(page.getByRole('button', { name: /Текущий баланс/ })).toContainText(/20\s*000/)
    const settingsA = await request.get('/api/v1/settings', { headers: a.headers })
    expect((await settingsA.json()).base_day_cost.amount).toBe(40000)
    const settingsB = await request.get('/api/v1/settings', { headers: b.headers })
    expect((await settingsB.json()).base_day_cost.amount).toBe(0)
    await page.getByRole('button', { name: 'Настройки', exact: true }).click()
    await expect(settings.getByLabel('Базовая стоимость дня, ₽')).toHaveValue('0')
    expect(confirmations).toEqual([])
  } finally { release() }
})

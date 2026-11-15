import { expect, test, type Page } from '@playwright/test'
import { assertNoHorizontalOverflow, calendar, dayCell, dayDetail, loginUI, monthOffset, moscowToday, seedProfile, uniqueLogin } from './helpers'

const balanceCard = (page: Page) => page.getByRole('button', { name: /Текущий баланс/ })
const rowFor = (page: Page, marker: string) => dayDetail(page).getByRole('article').filter({ hasText: marker })

test('прошлый факт создаётся без первого якоря и сохраняет неизвестный баланс после reload', async ({ page }) => {
  await loginUI(page, uniqueLogin())
  await expect(page.getByText('Баланс пока неизвестен — операции и прошлые даты доступны.')).toBeVisible()
  await calendar(page).locator('.fc-prev-button').click()
  const date = monthOffset(moscowToday(), -1)
  const marker = `Неизвестный баланс ${uniqueLogin()}`
  const dialog = await createIncome(page, marker, date)
  await dialog.getByLabel('Состояние первого вхождения').selectOption('CONFIRMED')
  await expect(dialog.getByLabel('Фактические дата и время (Москва)')).toHaveValue(`${date}T12:00`)
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(dialog).toBeHidden()
  await expect(rowFor(page, marker)).toContainText('Факт · Доход')
  await expect(dayDetail(page).locator('div.rounded-2xl').filter({ hasText: 'Баланс вечером' })).toContainText('—')
  await page.reload()
  await calendar(page).locator('.fc-prev-button').click()
  await dayCell(page, date).click()
  await expect(rowFor(page, marker)).toContainText('Факт · Доход')
  await expect(dayDetail(page).locator('div.rounded-2xl').filter({ hasText: 'Баланс вечером' })).toContainText('—')
  await expect(page.getByText('Баланс пока неизвестен — операции и прошлые даты доступны.')).toBeVisible()
})

async function createIncome(page: Page, marker: string, date = moscowToday()) {
  await dayCell(page, date).click()
  await dayDetail(page).getByRole('button', { name: 'Добавить', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Новое событие' })
  await dialog.getByText('Доход', { exact: true }).click()
  await dialog.getByLabel('Сумма, ₽', { exact: true }).fill('8700')
  await dialog.getByLabel('Комментарий', { exact: true }).fill(marker)
  return dialog
}

test('план 8700 не меняет факт 10000; подтверждение, повтор, reload и следующий день дают 18700', async ({ page, request }) => {
  const profile = await seedProfile(request, 10000)
  await loginUI(page, profile.login)
  const marker = `План-факт ${uniqueLogin()}`
  const dialog = await createIncome(page, marker)
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(dialog).toBeHidden()
  const row = rowFor(page, marker)
  await expect(row).toContainText('План · Доход')
  await expect(balanceCard(page)).toContainText(/10\s*000/)
  await expect(page.getByRole('article').filter({ hasText: 'Деньги на жизнь' })).toContainText(/18\s*700/)
  await expect(dayDetail(page).locator('div.rounded-2xl').filter({ hasText: 'Баланс вечером' })).toContainText(/18\s*700/)
  await row.getByRole('button', { name: 'Подтвердить исполнение' }).click()
  await row.getByLabel('Фактические дата и время (Москва)').fill('')
  let release!: () => void
  let captured!: (value: { url: string; body: unknown }) => void
  const gate = new Promise<void>((done) => { release = done })
  const received = new Promise<{ url: string; body: unknown }>((done) => { captured = done })
  let confirmations = 0
  await page.route('**/api/v1/events/*/occurrences/*', async (route) => {
    confirmations++
    const response = await route.fetch()
    captured({ url: route.request().url(), body: route.request().postDataJSON() })
    await gate
    await route.fulfill({ response })
  })
  try {
    await row.getByRole('button', { name: 'Подтвердить факт' }).click()
    const confirmation = await received
    await expect(row.getByRole('button', { name: 'Подтверждаем…' })).toBeDisabled()
    expect(confirmations).toBe(1)
    release()
    await expect(row).toContainText('Факт · Доход')
    await expect(balanceCard(page)).toContainText(/18\s*700/)
    await expect(page.getByRole('article').filter({ hasText: 'Деньги на жизнь' })).toContainText(/18\s*700/)
    // Exercise real API idempotency with the exact browser payload, then verify persistence in UI.
    const repeat = await request.put(confirmation.url, { headers: profile.headers, data: confirmation.body })
    expect(repeat.ok()).toBeTruthy()
    await page.reload()
    await expect(balanceCard(page)).toContainText(/18\s*700/)
    await dayCell(page, moscowToday()).click()
    await expect(row).toContainText('Факт · Доход')
    await expect(row.getByRole('button', { name: 'Подтвердить исполнение' })).toHaveCount(0)
    await dayDetail(page).getByRole('button', { name: 'Закрыть', exact: true }).click()
    const tomorrow = new Date(`${moscowToday()}T12:00:00+03:00`)
    tomorrow.setUTCDate(tomorrow.getUTCDate() + 1)
    const nextDate = tomorrow.toISOString().slice(0, 10)
    await dayCell(page, nextDate).click()
    await expect(dayDetail(page).locator('div.rounded-2xl').filter({ hasText: 'Баланс вечером' })).toContainText(/18\s*700/)
    expect(confirmations).toBe(1)
    await assertNoHorizontalOverflow(page)
  } finally { release() }
})

test('подтверждается одно месячное вхождение; следующий месяц остаётся планом', async ({ page, request }) => {
  const profile = await seedProfile(request, 10000)
  const day = moscowToday().slice(-2)
  const marker = `Месячная серия ${uniqueLogin()}`
  // Beginning in the current month ensures valid days 29–31 too.
  const response = await request.post('/api/v1/events', {
    headers: profile.headers,
    data: { kind: 'INCOME', necessity: 'REQUIRED', amount: { amount: 870000, currency: 'RUB' }, category_id: '', comment: marker, recurrence: { type: 'MONTHLY_ON_DAY', start_date: moscowToday(), day_of_month: Number(day), interval_days: null, until_date: null } },
  })
  expect(response.ok()).toBeTruthy()
  await loginUI(page, profile.login)
  await dayCell(page, moscowToday()).click()
  const row = rowFor(page, marker)
  await row.getByRole('button', { name: 'Подтвердить исполнение' }).click()
  await row.getByLabel('Фактические дата и время (Москва)').fill('')
  await row.getByRole('button', { name: 'Подтвердить факт' }).click()
  await expect(row).toContainText('Факт · Доход')
  await expect(balanceCard(page)).toContainText(/18\s*700/)
  await dayDetail(page).getByRole('button', { name: 'Закрыть', exact: true }).click()
  await calendar(page).locator('.fc-next-button').click()
  const [year, month] = monthOffset(moscowToday(), 1).split('-').map(Number)
  const last = new Date(year, month, 0).getDate()
  const next = `${year}-${String(month).padStart(2, '0')}-${String(Math.min(Number(day), last)).padStart(2, '0')}`
  await dayCell(page, next).click()
  await expect(row).toContainText('План · Доход')
  await expect(row.getByRole('button', { name: 'Подтвердить исполнение' })).toHaveCount(0)
  await expect(balanceCard(page)).toContainText(/18\s*700/)
  await page.reload()
  await dayCell(page, moscowToday()).click()
  await expect(row).toContainText('Факт · Доход')
})

test('резерв 400 не меняет факт, а подтверждение расхода и отмена не списывают дважды', async ({ page, request }) => {
  const profile = await seedProfile(request, 10000)
  const settings = await request.put('/api/v1/settings', {
    headers: profile.headers,
    data: { base_day_cost: { amount: 40000, currency: 'RUB' }, horizon_policy: 'END_OF_MONTH', horizon_custom_date: null },
  })
  expect(settings.ok()).toBeTruthy()
  await loginUI(page, profile.login)
  await dayCell(page, moscowToday()).click()
  await dayDetail(page).getByRole('button', { name: 'Добавить', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Новое событие' })
  const marker = `Расход и резерв ${uniqueLogin()}`
  await dialog.getByLabel('Сумма, ₽', { exact: true }).fill('1500')
  await dialog.getByLabel('Комментарий', { exact: true }).fill(marker)
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(dialog).toBeHidden()
  const row = rowFor(page, marker)
  const evening = dayDetail(page).locator('div.rounded-2xl').filter({ hasText: 'Баланс вечером' })
  await expect(balanceCard(page)).toContainText(/10\s*000/)
  await expect(evening).toContainText(/8\s*100/)
  await row.getByRole('button', { name: 'Подтвердить исполнение' }).click()
  await row.getByRole('button', { name: 'Подтвердить факт' }).click()
  await expect(row).toContainText('Факт · Обязательный расход')
  await expect(balanceCard(page)).toContainText(/8\s*500/)
  await expect(evening).toContainText(/8\s*100/)
  await row.getByRole('button', { name: 'Снять подтверждение', exact: true }).click()
  await expect(row).toContainText('План · Обязательный расход')
  await expect(balanceCard(page)).toContainText(/10\s*000/)
  await expect(evening).toContainText(/8\s*100/)
  await row.getByRole('button', { name: 'Отменить план', exact: true }).click()
  await expect(row).toContainText('Отменено · Обязательный расход')
  await expect(evening).toContainText(/9\s*600/)
  await row.getByRole('button', { name: 'Восстановить план', exact: true }).click()
  await expect(row).toContainText('План · Обязательный расход')
  await row.getByRole('button', { name: 'Подтвердить исполнение' }).click()
  await row.getByLabel('Повседневная трата — из стоимости дня').check()
  await row.getByRole('button', { name: 'Подтвердить факт' }).click()
  await expect(row).toContainText('Факт · Обязательный расход')
  await expect(balanceCard(page)).toContainText(/8\s*500/)
  await expect(evening).toContainText(/8\s*500/)
  await page.reload()
  await expect(balanceCard(page)).toContainText(/8\s*500/)
  await dayCell(page, moscowToday()).click()
  await expect(evening).toContainText(/8\s*500/)
})

test('снять факт после изменения recurrence: план не выдумывается, сообщение явно объясняет результат', async ({ page, request }) => {
  const profile = await seedProfile(request, 10000)
  await loginUI(page, profile.login)
  const marker = `Изменённое правило ${uniqueLogin()}`
  const dialog = await createIncome(page, marker)
  await dialog.getByRole('combobox', { name: 'Повторение', exact: true }).selectOption('MONTHLY_ON_DAY')
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(dialog).toBeHidden()
  const row = rowFor(page, marker)
  await row.getByRole('button', { name: 'Подтвердить исполнение' }).click()
  await row.getByLabel('Фактические дата и время (Москва)').fill('')
  await row.getByRole('button', { name: 'Подтвердить факт' }).click()
  await expect(row).toContainText('Факт · Доход')
  await row.getByRole('button', { name: 'Изменить серию' }).click()
  const editor = page.getByRole('dialog', { name: 'Редактировать событие' })
  const differentDay = Number(moscowToday().slice(-2)) === 1 ? '2' : '1'
  await editor.getByLabel('День месяца', { exact: true }).fill(differentDay)
  await editor.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(editor).toBeHidden()
  await expect(row).toContainText('Факт · Доход')
  const removed = page.waitForResponse((response) => response.url().includes('/occurrences/') && response.request().method() === 'PUT')
  await row.getByRole('button', { name: 'Снять подтверждение', exact: true }).click()
  const result = await removed
  expect(result.ok()).toBeTruthy()
  expect((await result.json()).plan_restored).toBe(false)
  await expect(page.getByText('Подтверждение снято. План не восстановлен: правило повторения изменилось', { exact: false })).toBeVisible()
  await expect(row).toHaveCount(0)
  await expect(balanceCard(page)).toContainText(/10\s*000/)
  await page.reload()
  await dayCell(page, moscowToday()).click()
  await expect(row).toHaveCount(0)
  await expect(balanceCard(page)).toContainText(/10\s*000/)
})

test('create-as-fact при потерянном ответе повторяет только тот же PUT и не создаёт второй план', async ({ page, request }) => {
  const profile = await seedProfile(request, 10000)
  await loginUI(page, profile.login)
  const marker = `Повтор подтверждения ${uniqueLogin()}`
  const dialog = await createIncome(page, marker)
  await dialog.getByLabel('Состояние первого вхождения').selectOption('CONFIRMED')
  await dialog.getByLabel('Фактические дата и время (Москва)').fill('')
  const writes: Array<{ method: string; url: string; body: unknown }> = []
  page.on('request', (req) => {
    if (/\/api\/v1\/events/.test(req.url()) && ['POST', 'PUT'].includes(req.method())) {
      writes.push({ method: req.method(), url: req.url(), body: req.postDataJSON() })
    }
  })
  let attempts = 0
  await page.route('**/api/v1/events/*/occurrences/*', async (route) => {
    attempts++
    const response = await route.fetch()
    expect(response.ok()).toBeTruthy()
    // The first PUT really commits in SQLite, but its response is lost in transit.
    if (attempts === 1) return route.abort('failed')
    return route.fulfill({ response })
  })
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(dialog).toContainText('Операция создана как план; подтверждение не завершено')
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: /повторить подтверждение|сохранить/i }).click()
  await expect(dialog).toBeHidden()
  expect(attempts).toBe(2)
  expect(writes.map((value) => value.method)).toEqual(['POST', 'PUT', 'PUT'])
  expect(writes[2]).toEqual(writes[1])
  await expect(balanceCard(page)).toContainText(/18\s*700/)
  const events = await request.get('/api/v1/events', { headers: profile.headers })
  expect((await events.json()).events.filter((event: { comment: string }) => event.comment === marker)).toHaveLength(1)
  await page.reload()
  await dayCell(page, moscowToday()).click()
  await expect(rowFor(page, marker)).toContainText('Факт · Доход')
  await expect(balanceCard(page)).toContainText(/18\s*700/)
})

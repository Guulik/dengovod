import { expect, test } from '@playwright/test'
import { assertNoHorizontalOverflow, calendar, createExpenseUI, dayCell, dayDetail, dismissOptionalOnboarding, loginUI, monthOffset, moscowToday, seedProfile, uniqueLogin } from './helpers'

test('регистрация и сохранение начальной настройки через настоящий API', async ({ page }) => {
  await loginUI(page, uniqueLogin())
  const onboarding = page.getByRole('dialog', { name: 'Настроим ваш прогноз' })
  await page.getByRole('button', { name: 'Указать текущий остаток', exact: true }).click()
  await expect(onboarding).toBeVisible()
  if (await onboarding.isVisible()) {
    await onboarding.getByLabel('Текущий баланс, ₽').fill('10000')
    await onboarding.getByLabel('Базовая стоимость дня, ₽').fill('0')
    await onboarding.getByLabel('Горизонт планирования').selectOption('END_OF_MONTH')
    await onboarding.getByRole('button', { name: 'Начать планировать' }).click()
  } else {
    await page.getByRole('button', { name: /Текущий баланс/ }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel(/баланс.*₽/i).fill('10000')
    await dialog.getByRole('button', { name: /сохранить/i }).click()
  }
  await expect(calendar(page)).toBeVisible()
  await expect(page.getByRole('button', { name: /Текущий баланс/ })).toContainText(/10\s*000/)
  await page.reload()
  await expect(page.getByRole('button', { name: /Текущий баланс/ })).toContainText(/10\s*000/)
  await page.getByRole('button', { name: 'Настройки', exact: true }).click()
  const settings = page.getByRole('dialog', { name: 'Настройки прогноза' })
  await settings.getByLabel('Базовая стоимость дня, ₽').fill('400')
  const saved = page.waitForResponse((response) => response.url().endsWith('/settings') && response.request().method() === 'PUT')
  await settings.getByRole('button', { name: 'Сохранить настройки' }).click()
  expect((await saved).ok()).toBeTruthy()
  await settings.getByRole('button', { name: 'Закрыть' }).click()
  await page.reload()
  await page.getByRole('button', { name: 'Настройки', exact: true }).click()
  await expect(settings.getByLabel('Базовая стоимость дня, ₽')).toHaveValue('400')
  await assertNoHorizontalOverflow(page)
})

test('до первого якоря доступны прошлый месяц и полный CRUD операции', async ({ page }) => {
  await loginUI(page, uniqueLogin())
  await dismissOptionalOnboarding(page)
  await expect(calendar(page), 'Календарь должен быть доступен без обязательного ввода баланса').toBeVisible()
  await expect(page.getByText('Баланс пока неизвестен — операции и прошлые даты доступны.')).toBeVisible()
  await calendar(page).locator('.fc-prev-button').click()
  const date = monthOffset(moscowToday(), -1)
  await dayCell(page, date).click()
  await expect(dayDetail(page)).toBeVisible()
  await expect(dayDetail(page).locator('div.rounded-2xl').filter({ hasText: 'Баланс вечером' })).toContainText('—')
  const comment = `Прошлый расход ${uniqueLogin()}`
  await createExpenseUI(page, comment)
  const row = dayDetail(page).getByRole('article').filter({ hasText: comment })
  await expect(row).toContainText(/1\s*500/)
  await page.reload()
  await calendar(page).locator('.fc-prev-button').click()
  await dayCell(page, date).click()
  await row.getByRole('button', { name: 'Изменить серию' }).click()
  const editor = page.getByRole('dialog', { name: 'Редактировать событие' })
  await editor.getByLabel('Сумма, ₽', { exact: true }).fill('1750')
  await editor.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(editor).toBeHidden()
  await expect(row).toContainText(/1\s*750/)
  await row.getByRole('button', { name: 'Изменить серию' }).click()
  page.once('dialog', (dialog) => dialog.accept())
  await editor.getByRole('button', { name: 'Удалить', exact: true }).click()
  await expect(editor).toBeHidden()
  await expect(row).toHaveCount(0)
  await page.reload()
  await calendar(page).locator('.fc-prev-button').click()
  await dayCell(page, date).click()
  await expect(row).toHaveCount(0)
  await assertNoHorizontalOverflow(page)
})

test('месяц остаётся выбранным при задержанном ответе и сохранении операции', async ({ page, request }) => {
  const profile = await seedProfile(request, 10000)
  await loginUI(page, profile.login)
  await expect(calendar(page)).toBeVisible()
  const next = monthOffset(moscowToday(), 1)
  let release!: () => void
  let received!: () => void
  const gate = new Promise<void>((done) => { release = done })
  const arrived = new Promise<void>((done) => { received = done })
  let held = false
  await page.route('**/api/v1/calendar?*', async (route) => {
    if (held) return route.continue()
    held = true
    const response = await route.fetch()
    received()
    await gate
    await route.fulfill({ response })
  })
  try {
    await calendar(page).locator('.fc-next-button').click()
    const selectedMonth = await calendar(page).locator('.fc-toolbar-title').textContent()
    await arrived
    await expect(dayCell(page, next), 'Выбранный месяц не должен исчезать во время загрузки').toBeVisible()
    await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText(selectedMonth!)
    release()
    await dayCell(page, next).click()
    const comment = `Следующий месяц ${uniqueLogin()}`
    await createExpenseUI(page, comment, '123')
    await expect(dayDetail(page).getByRole('article').filter({ hasText: comment })).toBeVisible()
    await expect(dayCell(page, next)).toBeVisible()
    await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText(selectedMonth!)
    await assertNoHorizontalOverflow(page)
  } finally { release() }
})

test('медленная начальная сводка не блокирует прошлые даты нового профиля', async ({ page }) => {
  let release!: () => void
  let received!: () => void
  const gate = new Promise<void>((done) => { release = done })
  const arrived = new Promise<void>((done) => { received = done })
  await page.route('**/api/v1/forecast', async (route) => {
    const response = await route.fetch()
    received()
    await gate
    await route.fulfill({ response }).catch(() => {})
  })
  try {
    await loginUI(page, uniqueLogin())
    await arrived
    await expect(calendar(page)).toBeVisible()
    await calendar(page).locator('.fc-prev-button').click()
    await dayCell(page, monthOffset(moscowToday(), -1)).click()
    await expect(dayDetail(page)).toBeVisible()
    await dayDetail(page).getByRole('button', { name: 'Добавить', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Новое событие' })).toBeVisible()
    release()
  } finally { release() }
})

test('выход A и вход B очищают суммы, выбранный день и поздний ответ A', async ({ page, request }) => {
  const a = await seedProfile(request, 87654)
  const b = await seedProfile(request, 12345)
  const marker = `Секрет профиля A ${uniqueLogin()}`
  const event = await request.post('/api/v1/events', {
    headers: a.headers,
    data: { kind: 'EXPENSE', necessity: 'REQUIRED', amount: { amount: 98765, currency: 'RUB' }, category_id: '', comment: marker, recurrence: { type: 'NONE', start_date: moscowToday(), day_of_month: null, interval_days: null, until_date: null } },
  })
  expect(event.ok()).toBeTruthy()
  await loginUI(page, a.login)
  await expect(calendar(page)).toBeVisible()
  await dayCell(page, moscowToday()).click()
  await expect(dayDetail(page)).toContainText(marker)
  await dayDetail(page).getByRole('button', { name: 'Закрыть', exact: true }).click()
  let release!: () => void
  let received!: () => void
  const gate = new Promise<void>((done) => { release = done })
  const arrived = new Promise<void>((done) => { received = done })
  let held = false
  await page.route('**/api/v1/calendar?*', async (route) => {
    if (held || route.request().headers().authorization !== `Bearer ${a.token}`) return route.continue()
    held = true
    const response = await route.fetch()
    received()
    await gate
    // Aborted old-session requests are the expected safe implementation.
    await route.fulfill({ response }).catch(() => {})
  })
  const leaks: string[] = []
  try {
    await calendar(page).locator('.fc-next-button').click()
    await arrived
    await dayCell(page, monthOffset(moscowToday(), 1)).click()
    await expect(dayDetail(page)).toBeVisible()
    await page.getByRole('button', { name: 'Выйти', exact: true }).click()
    await page.getByRole('textbox', { name: 'Имя пользователя' }).fill(b.login)
    // Observe every DOM mutation after B starts login, including transient cached A data.
    await page.exposeFunction('recordPrivateLeak', (value: string) => { leaks.push(value) })
    await page.evaluate((privateMarker) => {
      const inspect = () => {
        const body = document.body.innerText
        if (body.includes(privateMarker) || /87\s*654|86\s*666/.test(body)) {
          void (window as unknown as { recordPrivateLeak: (value: string) => Promise<void> }).recordPrivateLeak(body)
        }
      }
      new MutationObserver(inspect).observe(document.body, { childList: true, subtree: true, characterData: true })
    }, marker)
    await page.getByRole('button', { name: 'Продолжить', exact: true }).click()
    await expect(page.getByRole('button', { name: /Текущий баланс/ })).toContainText(/12\s*345/)
    release()
    await expect(calendar(page)).toBeVisible()
    await expect(dayDetail(page)).toHaveCount(0)
    await expect(page.getByText(marker, { exact: false })).toHaveCount(0)
    await dayCell(page, moscowToday()).click()
    await expect(dayDetail(page)).not.toContainText(marker)
    expect(leaks, 'Даже кратковременное отображение данных A в сессии B недопустимо').toEqual([])
    const bCategories = await request.get('/api/v1/categories', { headers: b.headers })
    const salaryB = (await bCategories.json()).categories.find((category: { system_code: string | null }) => category.system_code === 'SALARY')
    expect(salaryB).toBeTruthy()
    await dayDetail(page).getByRole('button', { name: 'Добавить', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Новое событие' })
    await dialog.getByText('Доход', { exact: true }).click()
    await dialog.getByRole('combobox', { name: 'Категория', exact: true }).selectOption({ label: 'Зарплата' })
    await dialog.getByLabel('Сумма, ₽', { exact: true }).fill('500')
    const saved = page.waitForResponse((response) => response.url().endsWith('/events') && response.request().method() === 'POST')
    await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click()
    const result = await saved
    expect(result.ok()).toBeTruthy()
    expect((await result.json()).category_id).toBe(salaryB.id)
    await expect(dialog).toBeHidden()
    await assertNoHorizontalOverflow(page)
  } finally { release() }
})

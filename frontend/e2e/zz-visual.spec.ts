import { expect, test } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { assertNoHorizontalOverflow, dayCell, dayDetail, loginUI, moscowToday, seedProfile } from './helpers'

test('визуальная приёмка: план/факт и калькулятор на 390px', async ({ page, request }, testInfo) => {
  const profile = await seedProfile(request, 10000)
  for (const [kind, amount, comment, confirmed] of [
    ['INCOME', 870000, 'Зарплата получена', true],
    ['EXPENSE', 150000, 'План: коммунальные услуги', false],
  ] as const) {
    const created = await request.post('/api/v1/events', {
      headers: profile.headers,
      data: { kind, necessity: 'REQUIRED', amount: { amount, currency: 'RUB' }, category_id: '', comment, recurrence: { type: 'NONE', start_date: moscowToday(), day_of_month: null, interval_days: null, until_date: null } },
    })
    expect(created.ok()).toBeTruthy()
    if (confirmed) {
      const event = await created.json()
      const result = await request.put(`/api/v1/events/${event.id}/occurrences/${moscowToday()}`, { headers: profile.headers, data: { status: 'CONFIRMED' } })
      expect(result.ok()).toBeTruthy()
    }
  }
  await loginUI(page, profile.login)
  await dayCell(page, moscowToday()).click()
  await expect(dayDetail(page)).toContainText('Факт · Доход')
  await expect(dayDetail(page)).toContainText('План · Обязательный расход')
  await expect(page.getByRole('button', { name: /Текущий баланс/ })).toContainText(/18\s*700/)
  await assertNoHorizontalOverflow(page)
  const directory = resolve('e2e/.artifacts/visual')
  if (testInfo.project.name === 'mobile-390') {
    await mkdir(directory, { recursive: true })
    await page.screenshot({ path: resolve(directory, 'mobile-calendar-plan-fact.png'), fullPage: true })
  }
  await dayDetail(page).getByRole('button', { name: 'Добавить', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Новое событие' })
  await dialog.getByText('Доход', { exact: true }).click()
  await dialog.getByRole('combobox', { name: 'Категория', exact: true }).selectOption({ label: 'Зарплата' })
  const calculator = dialog.getByRole('group', { name: 'Калькулятор зарплаты', exact: true })
  await calculator.getByLabel('Оклад за полный месяц, ₽').fill('100000')
  await calculator.getByLabel('Расчётный месяц').fill('2026-04')
  await calculator.getByLabel('Отработано календарных дней').fill('15')
  await expect(calculator).toContainText(/Итого: 50\s*000/)
  await calculator.scrollIntoViewIfNeeded()
  await assertNoHorizontalOverflow(page)
  if (testInfo.project.name === 'mobile-390') {
    await page.screenshot({ path: resolve(directory, 'mobile-salary-calculator.png'), fullPage: false })
  }
})

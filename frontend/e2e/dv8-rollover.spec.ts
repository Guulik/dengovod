import { expect, test, type Page } from '@playwright/test'
import { assertNoHorizontalOverflow, calendar, dayCell, dayDetail, loginUI, seedProfile } from './helpers'

// These tests prove browser rollover -> fresh real API reads and stable selection.
// The backend keeps real time: they do NOT prove a financial midnight transition.
function observeFinancialReads(page: Page) {
  const counts = { balance: 0, forecast: 0, calendar: 0 }
  page.on('response', (response) => {
    if (response.status() !== 200 || response.request().method() !== 'GET') return
    const path = new URL(response.url()).pathname
    if (path === '/api/v1/balance') counts.balance++
    if (path === '/api/v1/forecast') counts.forecast++
    if (path === '/api/v1/calendar') counts.calendar++
  })
  return counts
}

async function chooseOtherMonth(page: Page, counts: ReturnType<typeof observeFinancialReads>) {
  await expect.poll(() => counts.balance > 0 && counts.forecast > 0 && counts.calendar > 0).toBe(true)
  const before = counts.calendar
  await calendar(page).locator('.fc-prev-button').click()
  await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText('октябрь 2026')
  await expect.poll(() => counts.calendar).toBeGreaterThan(before)
  await dayCell(page, '2026-10-15').click()
  await expect(dayDetail(page)).toBeVisible()
  return { month: await calendar(page).locator('.fc-toolbar-title').textContent(), heading: await dayDetail(page).getByRole('heading', { level: 2 }).textContent() }
}

async function expectRefreshed(page: Page, counts: ReturnType<typeof observeFinancialReads>, before: ReturnType<typeof observeFinancialReads>, selected: { month: string | null; heading: string | null }) {
  await expect.poll(() => counts.balance).toBeGreaterThan(before.balance)
  await expect.poll(() => counts.forecast).toBeGreaterThan(before.forecast)
  await expect.poll(() => counts.calendar).toBeGreaterThan(before.calendar)
  await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText(selected.month!)
  await expect(dayDetail(page).getByRole('heading', { level: 2 })).toHaveText(selected.heading!)
  await assertNoHorizontalOverflow(page)
}

test('DV8: таймер московской полуночи обновляет реальные API-запросы и сохраняет выбранные месяц/дату', async ({ page, request }) => {
  const profile = await seedProfile(request, 20000) // Real Node/backend clock.
  await page.clock.install({ time: '2026-11-30T20:59:00Z' })
  const counts = observeFinancialReads(page)
  await loginUI(page, profile.login)
  const selected = await chooseOtherMonth(page, counts)
  const before = { ...counts }
  await page.clock.fastForward(61_000)
  await expectRefreshed(page, counts, before, selected)
  await dayDetail(page).getByRole('button', { name: 'Закрыть', exact: true }).click()
  await calendar(page).getByRole('button', { name: 'Сегодня', exact: true }).click()
  await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText('декабрь 2026')
  await expect(page.locator('.fc-daygrid-day.fc-day-today')).toHaveAttribute('data-date', '2026-12-01')
})

test('DV8: сигнал focus после смены browser Date обновляет реальные API и сохраняет выбранные месяц/дату', async ({ page, request }) => {
  const profile = await seedProfile(request, 20000) // Real Node/backend clock.
  await page.clock.setFixedTime('2026-11-30T20:50:00Z')
  const counts = observeFinancialReads(page)
  await loginUI(page, profile.login)
  const selected = await chooseOtherMonth(page, counts)
  const before = { ...counts }
  await page.clock.setFixedTime('2026-11-30T21:01:00Z')
  // A DOM focus signal exercises the recovery handler, not an OS background session.
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expectRefreshed(page, counts, before, selected)
})

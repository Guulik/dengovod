import { expect, test } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { assertNoHorizontalOverflow, calendar, dayCell, loginUI, seedProfile } from './helpers'

test('Сегодня после московской полуночи открывает новый месяц без перезагрузки', async ({ page, request }, testInfo) => {
  // Seed with the real Node/backend clock before changing browser Date only.
  // No future facts, fake API responses, or accelerated timer queues are involved.
  const profile = await seedProfile(request, 10000)
  await page.clock.setFixedTime('2026-11-30T20:59:00Z') // 30 November, 23:59 Moscow.
  await loginUI(page, profile.login)
  expect(await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)).toBe('Europe/Moscow')
  await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText('ноябрь 2026')
  await calendar(page).locator('.fc-prev-button').click()
  await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText('октябрь 2026')

  // The same mounted page crosses Moscow midnight; UTC still says November 30.
  // setFixedTime leaves normal browser timers running, so this takes no real wait.
  await page.clock.setFixedTime('2026-11-30T21:01:00Z') // 1 December, 00:01 Moscow.
  const todayButton = calendar(page).getByRole('button', { name: 'Сегодня', exact: true })
  await expect(todayButton, 'Пользователь должен видеть кнопку «Сегодня», в том числе на 390px').toBeVisible()
  await todayButton.click()
  await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText('декабрь 2026')
  await expect(dayCell(page, '2026-12-01')).toBeVisible()
  await expect(page.locator('.fc-daygrid-day[data-date="2026-12-01"]')).not.toHaveClass(/fc-day-other/)
  await assertNoHorizontalOverflow(page)

  if (testInfo.project.name === 'mobile-390') {
    const visualDirectory = resolve('e2e/.artifacts/visual')
    await mkdir(visualDirectory, { recursive: true })
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 844 })
      await todayButton.scrollIntoViewIfNeeded()
      await expect(todayButton, `«Сегодня» должна оставаться видимой при ширине ${width}px`).toBeVisible()
      await expect(todayButton).toBeInViewport()
      await expect(calendar(page).locator('.fc-toolbar-title')).toHaveText('декабрь 2026')
      await assertNoHorizontalOverflow(page)
      const bounds = await todayButton.boundingBox()
      expect(bounds).not.toBeNull()
      expect(bounds!.x).toBeGreaterThanOrEqual(0)
      expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width)
      expect(await calendar(page).locator('.fc-toolbar').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
      await calendar(page).screenshot({ path: resolve(visualDirectory, `calendar-today-${width}.png`) })
    }
  }
})

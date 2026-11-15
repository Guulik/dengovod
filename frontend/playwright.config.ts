import { defineConfig } from '@playwright/test'
import { randomUUID } from 'node:crypto'

const port = Number(process.env.E2E_PORT ?? 4179)
const runToken = randomUUID()

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.spec.ts',
  globalTeardown: './e2e/teardown.ts',
  metadata: { e2eRunToken: runToken, e2ePort: port },
  outputDir: './e2e/.artifacts/results',
  fullyParallel: false,
  workers: 1,
  timeout: 30_000,
  expect: { timeout: 7_000 },
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  reporter: [['list'], ['html', { outputFolder: './e2e/.artifacts/report', open: 'never' }]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    channel: process.env.E2E_BROWSER_CHANNEL,
    timezoneId: 'Europe/Moscow',
    locale: 'ru-RU',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    serviceWorkers: 'block',
  },
  projects: [
    { name: 'desktop', use: { browserName: 'chromium', viewport: { width: 1440, height: 1000 } } },
    { name: 'mobile-390', use: { browserName: 'chromium', viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } },
  ],
  webServer: {
    command: 'node e2e/server.mjs',
    url: `http://127.0.0.1:${port}/__e2e__/ready`,
    env: { E2E_RUN_TOKEN: runToken },
    timeout: 180_000,
    reuseExistingServer: false,
    stdout: 'pipe',
    stderr: 'pipe',
  },
})

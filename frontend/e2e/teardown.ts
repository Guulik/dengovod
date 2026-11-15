import type { FullConfig } from '@playwright/test'

export default async function teardown(config: FullConfig) {
  const base = `http://127.0.0.1:${config.metadata.e2ePort}`
  try {
    const response = await fetch(`${base}/__e2e__/shutdown`, {
      method: 'POST',
      headers: { 'x-e2e-run-token': String(config.metadata.e2eRunToken) },
      signal: AbortSignal.timeout(2000),
    })
    if (!response.ok) throw new Error(`E2E shutdown returned ${response.status}`)
    for (let attempt = 0; attempt < 50; attempt++) {
      await new Promise((done) => setTimeout(done, 100))
      try { await fetch(`${base}/__e2e__/ready`, { signal: AbortSignal.timeout(500) }) }
      catch { return }
    }
    throw new Error('Isolated E2E server did not stop')
  } catch (error) {
    // Make cleanup failures visible; Playwright also stops its own process tree.
    console.warn('[e2e] teardown:', error)
  }
}

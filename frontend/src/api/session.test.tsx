import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiNoContent, apiRequest } from './client'
import { categoriesSchema } from './schemas'
import { queryKeys, useCategories, useEvents, useSaveSettings } from './hooks'
import { currentSession, installSessionIsolation } from './session'
import { useAuthStore } from '../stores/authStore'
import { useUIStore } from '../stores/uiStore'
import { deferred, eventFixture, userA, userB } from '../test/fixtures'

const clients: QueryClient[] = []
const unsubscribe: Array<() => void> = []
function harness() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } })
  clients.push(client)
  unsubscribe.push(installSessionIsolation(client))
  return { client, wrapper: ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider> }
}
afterEach(() => {
  unsubscribe.splice(0).forEach((stop) => stop())
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().logout()
  vi.unstubAllGlobals()
})

describe('private financial session isolation', () => {
  it('clears cached financial data and chosen entities on direct profile switch', async () => {
    useAuthStore.getState().setAuth('token-A', userA)
    const { client, wrapper } = harness()
    const sessionA = currentSession()
    const categoryA = { id: eventFixture.id, name: 'Только A', kind: 'INCOME', is_default: false }
    const b = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn((_url, options: RequestInit) => options.headers && (options.headers as Record<string, string>).Authorization === 'Bearer token-A'
      ? Promise.resolve(Response.json({ categories: [categoryA] })) : b.promise))
    const hook = renderHook(() => useCategories(), { wrapper })
    await waitFor(() => expect(hook.result.current.data?.categories[0]?.name).toBe('Только A'))
    useUIStore.getState().setSelectedDate('2026-01-01')
    useUIStore.getState().openEvent(eventFixture.id)
    act(() => useAuthStore.getState().setAuth('token-B', userB))
    expect(hook.result.current.data).toBeUndefined()
    expect(client.getQueryData(queryKeys.categories(sessionA.sessionId))).toBeUndefined()
    expect(useUIStore.getState()).toMatchObject({ selectedDate: null, editingEventId: null, eventOpen: false })
    b.resolve(Response.json({ categories: [] }))
    await waitFor(() => expect(hook.result.current.data).toEqual({ categories: [] }))
  })

  it('aborts old queries and ignores late successful responses even if fetch ignores AbortSignal', async () => {
    useAuthStore.getState().setAuth('token-A', userA)
    const { client, wrapper } = harness()
    const sessionA = currentSession()
    const a = deferred<Response>()
    let signal: AbortSignal | null | undefined
    vi.stubGlobal('fetch', vi.fn((_url, options: RequestInit) => {
      if ((options.headers as Record<string, string>).Authorization === 'Bearer token-A') { signal = options.signal; return a.promise }
      return Promise.resolve(Response.json({ events: [] }))
    }))
    const hook = renderHook(() => useEvents(), { wrapper })
    await waitFor(() => expect(signal).toBeDefined())
    act(() => { useAuthStore.getState().logout(); useAuthStore.getState().setAuth('token-B', userB) })
    expect(signal?.aborted).toBe(true)
    await waitFor(() => expect(hook.result.current.data).toEqual({ events: [] }))
    a.resolve(Response.json({ events: [eventFixture] }))
    await act(async () => { await a.promise; await Promise.resolve() })
    expect(hook.result.current.data).toEqual({ events: [] })
    expect(client.getQueryData(queryKeys.events(sessionA.sessionId))).toBeUndefined()
  })

  it.each(['json', 'no-content'])('a delayed 401 from %s cannot log out the next session', async (kind) => {
    useAuthStore.getState().setAuth('token-A', userA)
    const a = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn(() => a.promise))
    const pending = kind === 'json' ? apiRequest('/categories', categoriesSchema) : apiNoContent('/events/id', { method: 'DELETE' })
    const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    useAuthStore.getState().setAuth('token-B', userB)
    a.resolve(Response.json({ error: { code: 'UNAUTHORIZED', message: 'Old session' } }, { status: 401 }))
    await rejected
    expect(useAuthStore.getState().token).toBe('token-B')
  })

  it('does not send a retained mutation using the next profile credentials', async () => {
    useAuthStore.getState().setAuth('token-A', userA)
    const { wrapper } = harness()
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const hook = renderHook(() => useSaveSettings(), { wrapper })
    const oldMutation = hook.result.current
    hook.unmount()
    useAuthStore.getState().setAuth('token-B', userB)
    await expect(oldMutation.mutateAsync({ base_day_cost: { amount: 0, currency: 'RUB' }, horizon_policy: 'END_OF_MONTH', horizon_custom_date: null })).rejects.toMatchObject({ name: 'AbortError' })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not invalidate or alter the next profile after a delayed mutation succeeds', async () => {
    useAuthStore.getState().setAuth('token-A', userA)
    const { client, wrapper } = harness()
    const a = deferred<Response>()
    const fetchMock = vi.fn(() => a.promise)
    vi.stubGlobal('fetch', fetchMock)
    const hook = renderHook(() => useSaveSettings(), { wrapper })
    const payload = { base_day_cost: { amount: 0, currency: 'RUB' as const }, horizon_policy: 'END_OF_MONTH' as const, horizon_custom_date: null }
    let pending!: Promise<unknown>
    act(() => { pending = hook.result.current.mutateAsync(payload) })
    const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    hook.unmount()
    useAuthStore.getState().setAuth('token-B', userB)
    const keyB = queryKeys.balance(currentSession().sessionId)
    client.setQueryData(keyB, { privateB: 123 })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    a.resolve(Response.json({ ...payload, currency: 'RUB', updated_at: 'now' }))
    await rejected
    expect(client.getQueryData(keyB)).toEqual({ privateB: 123 })
    expect(invalidate).not.toHaveBeenCalled()
  })
})

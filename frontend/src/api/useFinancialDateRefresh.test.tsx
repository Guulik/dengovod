import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { act, cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { millisecondsUntilMoscowMidnight, useFinancialDateRefresh } from './useFinancialDateRefresh'
import { currentSession, installSessionIsolation } from './session'
import { queryKeys } from './hooks'
import { useAuthStore } from '../stores/authStore'
import { useUIStore } from '../stores/uiStore'
import { deferred, userA, userB } from '../test/fixtures'

let client: QueryClient
let stop: () => void
const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
const tick = async (milliseconds = 0) => { await act(async () => { await vi.advanceTimersByTimeAsync(milliseconds) }) }

beforeEach(() => {
  vi.useFakeTimers().setSystemTime(new Date('2026-11-30T20:59:59.999Z'))
  useAuthStore.getState().setAuth('A', userA)
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  stop = installSessionIsolation(client)
})
afterEach(() => {
  cleanup()
  stop()
  client.clear()
  useAuthStore.getState().logout()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('Moscow daily financial refresh', () => {
  it.each([
    ['2026-11-30T20:59:59.999Z', '2026-12-01'],
    ['2026-12-31T20:59:59.999Z', '2027-01-01'],
    ['2028-02-28T20:59:59.999Z', '2028-02-29'],
    ['2028-02-29T20:59:59.999Z', '2028-03-01'],
  ])('refreshes only the current session at exact midnight after %s', async (before, after) => {
    vi.setSystemTime(new Date(before))
    expect(millisecondsUntilMoscowMidnight()).toBe(1)
    const session = currentSession()
    const keyA = queryKeys.balance(session.sessionId)
    const keyB = queryKeys.balance(session.sessionId + 1)
    client.setQueryData(keyA, { amount: 20000 })
    client.setQueryData(keyB, { amount: 777 })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    useUIStore.getState().setSelectedDate('2026-08-10')
    const hook = renderHook(useFinancialDateRefresh, { wrapper })
    expect(invalidate).not.toHaveBeenCalled()
    await tick(1)
    expect(hook.result.current).toBe(after)
    expect(invalidate).toHaveBeenCalledExactlyOnceWith({ queryKey: queryKeys.session(session.sessionId) })
    expect(client.getQueryState(keyA)?.isInvalidated).toBe(true)
    expect(client.getQueryState(keyB)?.isInvalidated).toBe(false)
    expect(useUIStore.getState().selectedDate).toBe('2026-08-10')
    expect(millisecondsUntilMoscowMidnight()).toBe(86_400_000)
  })

  it('catches up after a suspended week and coalesces foreground lifecycle events', async () => {
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const visible = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    const hook = renderHook(useFinancialDateRefresh, { wrapper })
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    await tick()
    expect(invalidate).not.toHaveBeenCalled()
    // Advancing the clock without running timers simulates a suspended background tab.
    vi.setSystemTime(new Date('2026-12-07T07:00:00Z'))
    visible.mockReturnValue('visible')
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'))
      window.dispatchEvent(new Event('focus'))
      window.dispatchEvent(new Event('pageshow'))
    })
    await tick()
    expect(hook.result.current).toBe('2026-12-07')
    expect(invalidate).toHaveBeenCalledTimes(1)
    await tick(14 * 60 * 60 * 1000)
    expect(hook.result.current).toBe('2026-12-08')
    expect(invalidate).toHaveBeenCalledTimes(2)
  })

  it.each(['focus', 'pageshow', 'visibilitychange'])('refreshes stale financial data on %s even within the same Moscow date', async (event) => {
    vi.setSystemTime(new Date('2026-11-21T09:00:00Z'))
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    renderHook(useFinancialDateRefresh, { wrapper })
    act(() => (event === 'visibilitychange' ? document : window).dispatchEvent(new Event(event)))
    await tick()
    expect(invalidate).toHaveBeenCalledTimes(1)
  })

  it('cancels a pre-midnight fetch and ignores its late result after refetch', async () => {
    const old = deferred<{ amount: number }>()
    const signals: AbortSignal[] = []
    let count = 0
    const key = queryKeys.balance(currentSession().sessionId)
    const hook = renderHook(() => {
      const date = useFinancialDateRefresh()
      const balance = useQuery({ queryKey: key, queryFn: ({ signal }) => {
        signals.push(signal)
        count++
        return count === 1 ? old.promise : Promise.resolve({ amount: 18600 })
      } })
      return { date, balance }
    }, { wrapper })
    expect(count).toBe(1)
    await tick(1)
    await tick(1)
    expect(signals[0].aborted).toBe(true)
    expect(count).toBe(2)
    expect(hook.result.current.balance.data).toEqual({ amount: 18600 })
    await act(async () => { old.resolve({ amount: 20000 }); await old.promise })
    await tick()
    expect(hook.result.current.balance.data).toEqual({ amount: 18600 })
  })

  it('does not let an awaited old-session refresh invalidate the next login', async () => {
    const gate = deferred<void>()
    const cancel = vi.spyOn(client, 'cancelQueries').mockImplementationOnce(() => gate.promise)
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const hook = renderHook(useFinancialDateRefresh, { wrapper })
    act(() => window.dispatchEvent(new Event('focus')))
    await tick()
    expect(cancel).toHaveBeenCalledTimes(1)
    act(() => useAuthStore.getState().setAuth('B', userB))
    const keyB = queryKeys.balance(currentSession().sessionId)
    client.setQueryData(keyB, { amount: 777 })
    await act(async () => { gate.resolve(); await gate.promise })
    expect(invalidate).not.toHaveBeenCalled()
    expect(client.getQueryState(keyB)?.isInvalidated).toBe(false)
    await tick(1)
    expect(invalidate).toHaveBeenCalledExactlyOnceWith({ queryKey: queryKeys.session(currentSession().sessionId) })
    expect(hook.result.current).toBe('2026-12-01')
    hook.unmount()
    invalidate.mockClear()
    act(() => window.dispatchEvent(new Event('focus')))
    await tick(86_400_000)
    expect(invalidate).not.toHaveBeenCalled()
  })

  it('removes the old timer on logout and starts from the current date on a later login', async () => {
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const hook = renderHook(useFinancialDateRefresh, { wrapper })
    act(() => useAuthStore.getState().logout())
    await tick(7 * 86_400_000)
    expect(invalidate).not.toHaveBeenCalled()
    act(() => useAuthStore.getState().setAuth('new-token-A', userA))
    expect(hook.result.current).toBe('2026-12-07')
    await tick(1)
    expect(hook.result.current).toBe('2026-12-08')
    expect(invalidate).toHaveBeenCalledExactlyOnceWith({ queryKey: queryKeys.session(currentSession().sessionId) })
  })
})

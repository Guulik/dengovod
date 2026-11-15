import type { QueryClient } from '@tanstack/react-query'
import { useAuthStore } from '../stores/authStore'

export type Session = Pick<ReturnType<typeof useAuthStore.getState>, 'token' | 'sessionId'>
export const currentSession = (): Session => {
  const { token, sessionId } = useAuthStore.getState()
  return { token, sessionId }
}
export const isCurrentSession = (session: Session) => {
  const current = currentSession()
  return current.sessionId === session.sessionId && current.token === session.token
}
export function assertCurrentSession(session: Session) {
  if (!isCurrentSession(session)) throw new DOMException('Сессия изменилась', 'AbortError')
}

// Requests also carry their original credentials: an old mutation can never use a new login.
const pending = new Set<AbortController>()
export function sessionRequestSignal(signal?: AbortSignal | null) {
  const controller = new AbortController()
  const abort = () => controller.abort()
  if (signal?.aborted) abort()
  signal?.addEventListener('abort', abort, { once: true })
  pending.add(controller)
  return {
    signal: controller.signal,
    dispose: () => {
      pending.delete(controller)
      signal?.removeEventListener('abort', abort)
    },
  }
}
useAuthStore.subscribe((state, previous) => {
  if (state.sessionId === previous.sessionId && state.token === previous.token) return
  for (const controller of pending) controller.abort()
  pending.clear()
})

export function installSessionIsolation(client: QueryClient) {
  return useAuthStore.subscribe((state, previous) => {
    if (state.sessionId === previous.sessionId && state.token === previous.token) return
    void client.cancelQueries()
    client.clear()
  })
}

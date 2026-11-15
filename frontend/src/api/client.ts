import type { ZodType } from 'zod'
import { apiErrorSchema } from './schemas'
import { useAuthStore } from '../stores/authStore'
import { assertCurrentSession, currentSession, isCurrentSession, sessionRequestSignal, type Session } from './session'

const BASE_URL = '/api/v1'

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly details: Array<{ field: string; message: string }>

  constructor(
    status: number,
    code: string,
    message: string,
    details: Array<{ field: string; message: string }> = [],
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
  }
}

type RequestOptions = Omit<RequestInit, 'body'> & { body?: unknown; session?: Session }

async function request(path: string, options: RequestOptions, noContent = false): Promise<unknown> {
  const { session = currentSession(), body, signal, ...init } = options
  assertCurrentSession(session)
  const active = sessionRequestSignal(signal)
  try {
    const response = await fetch(`${BASE_URL}${path}`, {
      ...init,
      signal: active.signal,
      headers: {
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
        ...(session.token ? { Authorization: `Bearer ${session.token}` } : {}),
        ...init.headers,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    assertCurrentSession(session)
    if (!response.ok) {
      const raw: unknown = await response.json().catch(() => null)
      assertCurrentSession(session)
      const parsed = apiErrorSchema.safeParse(raw)
      if (response.status === 401 && session.token && isCurrentSession(session)) useAuthStore.getState().logout()
      throw parsed.success
        ? new ApiError(response.status, parsed.data.error.code, parsed.data.error.message, parsed.data.error.details)
        : new ApiError(response.status, 'UNKNOWN_ERROR', 'Не удалось обработать ответ сервера')
    }
    const raw: unknown = noContent ? undefined : await response.json()
    assertCurrentSession(session)
    return raw
  } finally {
    active.dispose()
  }
}

export async function apiRequest<T>(
  path: string,
  schema: ZodType<T>,
  options: RequestOptions = {},
): Promise<T> {
  const raw = await request(path, options)
  const parsed = schema.safeParse(raw)
  if (!parsed.success) {
    console.error('Invalid API response', path, parsed.error)
    throw new ApiError(200, 'INVALID_RESPONSE', 'Сервер вернул данные неизвестного формата')
  }
  return parsed.data
}

export async function apiNoContent(path: string, options: RequestOptions): Promise<void> {
  await request(path, options, true)
}

export function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : 'Что-то пошло не так. Попробуйте ещё раз.'
}

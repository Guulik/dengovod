import type { ZodType } from 'zod'
import { apiErrorSchema } from './schemas'
import { useAuthStore } from '../stores/authStore'

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

type RequestOptions = Omit<RequestInit, 'body'> & { body?: unknown }

export async function apiRequest<T>(
  path: string,
  schema: ZodType<T>,
  options: RequestOptions = {},
): Promise<T> {
  const token = useAuthStore.getState().token
  const response = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      ...(options.body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  })

  if (!response.ok) {
    const raw: unknown = await response.json().catch(() => null)
    const parsed = apiErrorSchema.safeParse(raw)
    if (response.status === 401) useAuthStore.getState().logout()
    throw parsed.success
      ? new ApiError(response.status, parsed.data.error.code, parsed.data.error.message, parsed.data.error.details)
      : new ApiError(response.status, 'UNKNOWN_ERROR', 'Не удалось обработать ответ сервера')
  }

  const raw: unknown = await response.json()
  const parsed = schema.safeParse(raw)
  if (!parsed.success) {
    console.error('Invalid API response', path, parsed.error)
    throw new ApiError(response.status, 'INVALID_RESPONSE', 'Сервер вернул данные неизвестного формата')
  }
  return parsed.data
}

export async function apiNoContent(path: string, options: RequestOptions): Promise<void> {
  const token = useAuthStore.getState().token
  const { body: _body, ...requestOptions } = options
  const response = await fetch(`${BASE_URL}${path}`, {
    ...requestOptions,
    headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}), ...requestOptions.headers },
  })
  if (!response.ok) {
    const raw: unknown = await response.json().catch(() => null)
    const parsed = apiErrorSchema.safeParse(raw)
    throw parsed.success
      ? new ApiError(response.status, parsed.data.error.code, parsed.data.error.message, parsed.data.error.details)
      : new ApiError(response.status, 'UNKNOWN_ERROR', 'Не удалось выполнить запрос')
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : 'Что-то пошло не так. Попробуйте ещё раз.'
}

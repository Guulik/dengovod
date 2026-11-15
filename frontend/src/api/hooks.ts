import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  balanceSchema, calendarSchema, categoriesSchema, eventSchema, eventsSchema,
  forecastSchema, loginSchema, overrideSchema, overridesSchema, settingsSchema, type Event,
  occurrenceStateSchema,
} from './schemas'
import { apiNoContent, apiRequest } from './client'
import { useAuthStore } from '../stores/authStore'
import { assertCurrentSession, isCurrentSession, type Session } from './session'

export const queryKeys = {
  session: (sessionId: number) => ['private', sessionId] as const,
  forecast: (sessionId: number) => ['private', sessionId, 'forecast'] as const,
  balance: (sessionId: number) => ['private', sessionId, 'balance'] as const,
  events: (sessionId: number) => ['private', sessionId, 'events'] as const,
  categories: (sessionId: number) => ['private', sessionId, 'categories'] as const,
  settings: (sessionId: number) => ['private', sessionId, 'settings'] as const,
  overrides: (sessionId: number) => ['private', sessionId, 'day-cost-overrides'] as const,
  calendar: (sessionId: number, from: string, to: string) => ['private', sessionId, 'calendar', from, to] as const,
}
function useSession(): Session {
  const token = useAuthStore((state) => state.token)
  const sessionId = useAuthStore((state) => state.sessionId)
  return { token, sessionId }
}
const rub = (amount: number) => ({ amount, currency: 'RUB' as const })

export function useLogin() {
  const session = useSession()
  return useMutation({
    mutationFn: (login: string) => apiRequest('/auth/login', loginSchema, { method: 'POST', body: { login }, session }),
    onSuccess: ({ token, user }) => {
      if (isCurrentSession(session)) useAuthStore.getState().setAuth(token, user)
    },
  })
}
export function useForecast() {
  const session = useSession()
  return useQuery({ queryKey: queryKeys.forecast(session.sessionId), queryFn: ({ signal }) => apiRequest('/forecast', forecastSchema, { session, signal }), enabled: !!session.token })
}
export function useBalance() {
  const session = useSession()
  return useQuery({ queryKey: queryKeys.balance(session.sessionId), queryFn: ({ signal }) => apiRequest('/balance', balanceSchema, { session, signal }), enabled: !!session.token })
}
export function useCalendar(from: string, to: string) {
  const session = useSession()
  return useQuery({
    queryKey: queryKeys.calendar(session.sessionId, from, to),
    queryFn: ({ signal }) => apiRequest(`/calendar?${new URLSearchParams({ from, to })}`, calendarSchema, { session, signal }),
    enabled: !!session.token && Boolean(from && to),
  })
}
export function useEvents() {
  const session = useSession()
  return useQuery({ queryKey: queryKeys.events(session.sessionId), queryFn: ({ signal }) => apiRequest('/events', eventsSchema, { session, signal }), enabled: !!session.token })
}
export function useCategories() {
  const session = useSession()
  return useQuery({ queryKey: queryKeys.categories(session.sessionId), queryFn: ({ signal }) => apiRequest('/categories', categoriesSchema, { session, signal }), enabled: !!session.token })
}
export function useSettings() {
  const session = useSession()
  return useQuery({ queryKey: queryKeys.settings(session.sessionId), queryFn: ({ signal }) => apiRequest('/settings', settingsSchema, { session, signal }), enabled: !!session.token })
}
export function useOverrides() {
  const session = useSession()
  return useQuery({ queryKey: queryKeys.overrides(session.sessionId), queryFn: ({ signal }) => apiRequest('/day-cost-overrides', overridesSchema, { session, signal }), enabled: !!session.token })
}

export type EventPayload = {
  kind: 'INCOME' | 'EXPENSE'
  necessity: 'REQUIRED' | 'OPTIONAL'
  amount: { amount: number; currency: 'RUB' }
  category_id: string
  comment: string
  recurrence: {
    type: 'NONE' | 'MONTHLY_ON_DAY' | 'EVERY_N_DAYS'
    start_date: string
    day_of_month: number | null
    interval_days: number | null
    until_date: string | null
  }
}

function useInvalidateFinancials(session: Session) {
  const client = useQueryClient()
  return async () => {
    assertCurrentSession(session)
    await client.invalidateQueries({ queryKey: queryKeys.session(session.sessionId) })
    // Stop chained mutations and modal callbacks if the login changed while refetching.
    assertCurrentSession(session)
  }
}

export function useSaveEvent(existing?: Event) {
  const session = useSession()
  const invalidate = useInvalidateFinancials(session)
  return useMutation({
    mutationFn: (payload: EventPayload) => apiRequest(existing ? `/events/${existing.id}` : '/events', eventSchema, {
      method: existing ? 'PUT' : 'POST', body: payload, session,
    }),
    onSuccess: invalidate,
  })
}
export function useDeleteEvent() {
  const session = useSession()
  const invalidate = useInvalidateFinancials(session)
  return useMutation({
    mutationFn: (id: string) => apiNoContent(`/events/${id}`, { method: 'DELETE', session }),
    onSuccess: invalidate,
  })
}
export type OccurrencePayload = {
  eventId: string
  occurrenceDate: string
  status: 'PLANNED' | 'CONFIRMED' | 'CANCELLED'
  effective_at?: string
  counts_toward_day_budget?: boolean
}
export function useSetOccurrenceState() {
  const session = useSession()
  const invalidate = useInvalidateFinancials(session)
  return useMutation({
    mutationFn: ({ eventId, occurrenceDate, ...body }: OccurrencePayload) => apiRequest(
      `/events/${eventId}/occurrences/${occurrenceDate}`, occurrenceStateSchema, { method: 'PUT', body, session },
    ),
    onSuccess: invalidate,
  })
}
export type SettingsPayload = {
  base_day_cost: { amount: number; currency: 'RUB' }
  horizon_policy: 'END_OF_MONTH' | 'NEXT_INCOME' | 'CUSTOM_DATE'
  horizon_custom_date: string | null
}
export function useSaveSettings() {
  const session = useSession()
  const invalidate = useInvalidateFinancials(session)
  return useMutation({
    mutationFn: (payload: SettingsPayload) => apiRequest('/settings', settingsSchema, { method: 'PUT', body: payload, session }),
    onSuccess: invalidate,
  })
}
export function useSetAnchor() {
  const session = useSession()
  const invalidate = useInvalidateFinancials(session)
  return useMutation({
    mutationFn: (amount: number) => apiRequest('/balance/anchor', balanceSchema, {
      method: 'POST', body: { amount: rub(amount), anchored_at: new Date().toISOString() }, session,
    }),
    onSuccess: invalidate,
  })
}
export type OverridePayload = {
  date_from: string
  date_to: string
  daily_amount: { amount: number; currency: 'RUB' }
  comment: string
}
export function useSaveOverride(id?: string) {
  const session = useSession()
  const invalidate = useInvalidateFinancials(session)
  return useMutation({
    mutationFn: (payload: OverridePayload) => apiRequest(id ? `/day-cost-overrides/${id}` : '/day-cost-overrides', overrideSchema, {
      method: id ? 'PUT' : 'POST', body: payload, session,
    }),
    onSuccess: invalidate,
  })
}
export function useDeleteOverride() {
  const session = useSession()
  const invalidate = useInvalidateFinancials(session)
  return useMutation({
    mutationFn: (id: string) => apiNoContent(`/day-cost-overrides/${id}`, { method: 'DELETE', session }),
    onSuccess: invalidate,
  })
}

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  balanceSchema,
  calendarSchema,
  categoriesSchema,
  eventSchema,
  eventsSchema,
  forecastSchema,
  loginSchema,
  overrideSchema,
  overridesSchema,
  settingsSchema,
  type Event,
} from './schemas'
import { apiNoContent, apiRequest } from './client'
import { useAuthStore } from '../stores/authStore'

export const queryKeys = {
  forecast: ['forecast'] as const,
  balance: ['balance'] as const,
  events: ['events'] as const,
  categories: ['categories'] as const,
  settings: ['settings'] as const,
  overrides: ['day-cost-overrides'] as const,
  calendar: (from: string, to: string) => ['calendar', from, to] as const,
}

const rub = (amount: number) => ({ amount, currency: 'RUB' as const })

export function useLogin() {
  const setAuth = useAuthStore((state) => state.setAuth)
  return useMutation({
    mutationFn: (login: string) => apiRequest('/auth/login', loginSchema, { method: 'POST', body: { login } }),
    onSuccess: ({ token, user }) => setAuth(token, user),
  })
}

export function useForecast() {
  return useQuery({ queryKey: queryKeys.forecast, queryFn: () => apiRequest('/forecast', forecastSchema) })
}
export function useBalance() {
  return useQuery({ queryKey: queryKeys.balance, queryFn: () => apiRequest('/balance', balanceSchema) })
}
export function useCalendar(from: string, to: string) {
  return useQuery({
    queryKey: queryKeys.calendar(from, to),
    queryFn: () => apiRequest(`/calendar?${new URLSearchParams({ from, to })}`, calendarSchema),
    enabled: Boolean(from && to),
  })
}
export function useEvents() {
  return useQuery({ queryKey: queryKeys.events, queryFn: () => apiRequest('/events', eventsSchema) })
}
export function useCategories() {
  return useQuery({ queryKey: queryKeys.categories, queryFn: () => apiRequest('/categories', categoriesSchema) })
}
export function useSettings() {
  return useQuery({ queryKey: queryKeys.settings, queryFn: () => apiRequest('/settings', settingsSchema) })
}
export function useOverrides() {
  return useQuery({ queryKey: queryKeys.overrides, queryFn: () => apiRequest('/day-cost-overrides', overridesSchema) })
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

function useInvalidateFinancials() {
  const client = useQueryClient()
  return () => Promise.all([
    client.invalidateQueries({ queryKey: queryKeys.events }),
    client.invalidateQueries({ queryKey: ['calendar'] }),
    client.invalidateQueries({ queryKey: queryKeys.forecast }),
    client.invalidateQueries({ queryKey: queryKeys.balance }),
  ])
}

export function useSaveEvent(existing?: Event) {
  const invalidate = useInvalidateFinancials()
  return useMutation({
    mutationFn: (payload: EventPayload) =>
      apiRequest(existing ? `/events/${existing.id}` : '/events', eventSchema, {
        method: existing ? 'PUT' : 'POST',
        body: payload,
      }),
    onSuccess: invalidate,
  })
}
export function useDeleteEvent() {
  const invalidate = useInvalidateFinancials()
  return useMutation({
    mutationFn: (id: string) => apiNoContent(`/events/${id}`, { method: 'DELETE' }),
    onSuccess: invalidate,
  })
}

export type SettingsPayload = {
  base_day_cost: { amount: number; currency: 'RUB' }
  horizon_policy: 'END_OF_MONTH' | 'NEXT_INCOME' | 'CUSTOM_DATE'
  horizon_custom_date: string | null
}

export function useSaveSettings() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (payload: SettingsPayload) => apiRequest('/settings', settingsSchema, { method: 'PUT', body: payload }),
    onSuccess: () => client.invalidateQueries(),
  })
}
export function useSetAnchor() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (amount: number) => apiRequest('/balance/anchor', balanceSchema, {
      method: 'POST',
      body: { amount: rub(amount), anchored_at: new Date().toISOString() },
    }),
    onSuccess: () => client.invalidateQueries(),
  })
}

export type OverridePayload = {
  date_from: string
  date_to: string
  daily_amount: { amount: number; currency: 'RUB' }
  comment: string
}
export function useSaveOverride(id?: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (payload: OverridePayload) => apiRequest(id ? `/day-cost-overrides/${id}` : '/day-cost-overrides', overrideSchema, {
      method: id ? 'PUT' : 'POST',
      body: payload,
    }),
    onSuccess: () => client.invalidateQueries(),
  })
}
export function useDeleteOverride() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiNoContent(`/day-cost-overrides/${id}`, { method: 'DELETE' }),
    onSuccess: () => client.invalidateQueries(),
  })
}

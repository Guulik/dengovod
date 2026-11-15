import { z } from 'zod'

export const moneySchema = z.object({
  amount: z.number().int().safe(),
  currency: z.literal('RUB'),
})

const isoDate = z.string().regex(/^\d{4}-\d{2}-\d{2}$/)
const nullableInt = z.number().int().nullable()

export const userSchema = z.object({
  id: z.string().uuid(),
  login: z.string(),
  created_at: z.string(),
})

export const loginSchema = z.object({ user: userSchema, token: z.string().uuid() })

export const recurrenceSchema = z.object({
  type: z.enum(['NONE', 'MONTHLY_ON_DAY', 'EVERY_N_DAYS']),
  start_date: isoDate,
  day_of_month: nullableInt,
  interval_days: nullableInt,
  until_date: isoDate.nullable(),
})

export const eventSchema = z.object({
  id: z.string().uuid(),
  kind: z.enum(['INCOME', 'EXPENSE']),
  necessity: z.enum(['REQUIRED', 'OPTIONAL']),
  amount: moneySchema,
  category_id: z.string().uuid().nullable(),
  comment: z.string(),
  recurrence: recurrenceSchema,
  created_at: z.string(),
  updated_at: z.string(),
})

export const eventsSchema = z.object({ events: z.array(eventSchema) })

export const categorySchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  kind: z.enum(['INCOME', 'EXPENSE']),
  is_default: z.boolean(),
  system_code: z.string().nullable().optional(),
})
export const categoriesSchema = z.object({ categories: z.array(categorySchema) })

export const occurrenceSchema = z.object({
  event_id: z.string().uuid(),
  occurrence_date: isoDate,
  status: z.enum(['PLANNED', 'CONFIRMED', 'CANCELLED']),
  effective_at: z.string().nullable(),
  counts_toward_day_budget: z.boolean(),
  series_deleted: z.boolean(),
  kind: z.enum(['INCOME', 'EXPENSE']),
  necessity: z.enum(['REQUIRED', 'OPTIONAL']),
  amount: moneySchema,
  category_id: z.string().uuid().nullable(),
  comment: z.string(),
})

export const forecastSchema = z.object({
  current_balance: moneySchema,
  money_for_life: moneySchema,
  horizon_date: isoDate,
  horizon_policy: z.enum(['END_OF_MONTH', 'NEXT_INCOME', 'CUSTOM_DATE']),
  required_remaining: moneySchema,
  day_cost_remaining: moneySchema,
  optional_remaining: moneySchema,
  days_until_horizon: z.number().int(),
})

export const occurrenceStateSchema = occurrenceSchema.extend({ plan_restored: z.boolean() })

export const calendarDaySchema = z.object({
  date: isoDate,
  expected_balance: moneySchema.nullable(),
  day_cost: moneySchema,
  money_for_life: moneySchema.nullable(),
  income_total: moneySchema,
  required_total: moneySchema,
  optional_total: moneySchema,
  events: z.array(occurrenceSchema),
})
export const calendarSchema = z.object({
  days: z.array(calendarDaySchema),
  summary: forecastSchema.nullable(),
})

export const settingsSchema = z.object({
  base_day_cost: moneySchema,
  horizon_policy: z.enum(['END_OF_MONTH', 'NEXT_INCOME', 'CUSTOM_DATE']),
  horizon_custom_date: isoDate.nullable(),
  currency: z.literal('RUB'),
  updated_at: z.string(),
})

export const balanceSchema = z.object({
  needs_balance_review: z.boolean(),
  anchor: z.object({
    id: z.string().uuid(),
    amount: moneySchema,
    anchored_at: z.string(),
  }),
  computed_balance: moneySchema,
  as_of: z.string(),
})

export const overrideSchema = z.object({
  id: z.string().uuid(),
  date_from: isoDate,
  date_to: isoDate,
  daily_amount: moneySchema,
  comment: z.string(),
  created_at: z.string(),
})
export const overridesSchema = z.object({ overrides: z.array(overrideSchema) })

export const apiErrorSchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    details: z.array(z.object({ field: z.string(), message: z.string() })).default([]),
  }),
})

export type Money = z.infer<typeof moneySchema>
export type User = z.infer<typeof userSchema>
export type Event = z.infer<typeof eventSchema>
export type Category = z.infer<typeof categorySchema>
export type Forecast = z.infer<typeof forecastSchema>
export type CalendarDay = z.infer<typeof calendarDaySchema>
export type Occurrence = z.infer<typeof occurrenceSchema>
export type Settings = z.infer<typeof settingsSchema>
export type DayCostOverride = z.infer<typeof overrideSchema>

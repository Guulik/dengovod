import { zodResolver } from '@hookform/resolvers/zod'
import { Save, Trash2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useCategories, useDeleteEvent, useEvents, useSaveEvent, useSetOccurrenceState, type EventPayload, type OccurrencePayload } from '../../api/hooks'
import { errorMessage } from '../../api/client'
import { Button, ErrorNotice, Field, Input, Modal, Select } from '../../components/ui'
import { today } from '../../lib/dates'
import { kopecksToInput, parseRubles } from '../../lib/money'
import { useUIStore } from '../../stores/uiStore'
import { isSalaryCategory } from '../../lib/salary'
import { SalaryCalculator } from './SalaryCalculator'
import { currentSession, isCurrentSession } from '../../api/session'
import { defaultEffectiveTime, effectiveTimeToApi, firstOccurrenceDate } from '../../lib/occurrences'
import { FactFields } from './FactFields'

const schema = z.object({
  kind: z.enum(['INCOME', 'EXPENSE']),
  necessity: z.enum(['REQUIRED', 'OPTIONAL']),
  amount: z.string().refine((value) => Number.isSafeInteger(parseRubles(value)) && parseRubles(value) > 0, 'Введите положительную сумму в допустимом диапазоне'),
  categoryId: z.string(),
  comment: z.string().max(240, 'Не больше 240 символов'),
  recurrenceType: z.enum(['NONE', 'MONTHLY_ON_DAY', 'EVERY_N_DAYS']),
  startDate: z.string().min(1, 'Выберите дату'),
  dayOfMonth: z.string(),
  intervalDays: z.string(),
  untilDate: z.string(),
}).superRefine((value, ctx) => {
  if (value.recurrenceType === 'MONTHLY_ON_DAY' && !(Number(value.dayOfMonth) >= 1 && Number(value.dayOfMonth) <= 31)) {
    ctx.addIssue({ code: 'custom', path: ['dayOfMonth'], message: 'Укажите день от 1 до 31' })
  }
  if (value.recurrenceType === 'EVERY_N_DAYS' && !(Number(value.intervalDays) >= 1)) {
    ctx.addIssue({ code: 'custom', path: ['intervalDays'], message: 'Минимум 1 день' })
  }
})
type FormData = z.infer<typeof schema>

const initialValues = (startDate = today()): FormData => ({
  kind: 'EXPENSE',
  necessity: 'REQUIRED',
  amount: '',
  categoryId: '',
  comment: '',
  recurrenceType: 'NONE',
  startDate,
  dayOfMonth: String(Number(startDate.slice(-2))),
  intervalDays: '7',
  untilDate: '',
})

export function EventModal() {
  const { eventOpen, editingEventId, selectedDate, closeEvent } = useUIStore()
  const { data: eventData } = useEvents()
  const { data: categoryData } = useCategories()
  const existing = eventData?.events.find((item) => item.id === editingEventId)
  const [pendingFact, setPendingFact] = useState<OccurrencePayload>()
  const [asFact, setAsFact] = useState(false)
  const [effectiveTime, setEffectiveTime] = useState('')
  const [budget, setBudget] = useState(false)
  const [validationError, setValidationError] = useState('')
  const locked = useRef(false)
  const save = useSaveEvent(existing)
  const confirm = useSetOccurrenceState()
  const remove = useDeleteEvent()
  const { register, handleSubmit, reset, watch, setValue, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: initialValues(),
  })
  const kind = watch('kind')
  const recurrenceType = watch('recurrenceType')
  const categoryId = watch('categoryId')
  const startDate = watch('startDate')
  const dayOfMonth = watch('dayOfMonth')
  const untilDate = watch('untilDate')
  const firstDate = firstOccurrenceDate({ type: recurrenceType, start_date: startDate, day_of_month: Number(dayOfMonth), interval_days: null, until_date: untilDate || null })
  const future = !firstDate || firstDate > today()
  const pending = save.isPending || remove.isPending || confirm.isPending

  useEffect(() => {
    if (!eventOpen) return
    setPendingFact(undefined)
    setAsFact(false)
    setBudget(false)
    setValidationError('')
    setEffectiveTime(defaultEffectiveTime(selectedDate ?? today()))
    if (existing) {
      reset({
        kind: existing.kind,
        necessity: existing.necessity,
        amount: kopecksToInput(existing.amount.amount),
        categoryId: existing.category_id ?? '',
        comment: existing.comment,
        recurrenceType: existing.recurrence.type,
        startDate: existing.recurrence.start_date,
        dayOfMonth: existing.recurrence.day_of_month?.toString() ?? '',
        intervalDays: existing.recurrence.interval_days?.toString() ?? '',
        untilDate: existing.recurrence.until_date ?? '',
      })
    } else {
      reset(initialValues(selectedDate ?? today()))
    }
  }, [eventOpen, existing, reset, selectedDate])

  useEffect(() => {
    setAsFact(false)
    setEffectiveTime(defaultEffectiveTime(firstDate ?? startDate))
  }, [firstDate, startDate])

  const submit = async (data: FormData) => {
    if (locked.current || (editingEventId && !existing)) return
    const session = currentSession()
    setValidationError('')
    let time: string | undefined
    try {
      if (!existing && asFact && !future) time = effectiveTimeToApi(effectiveTime)
    } catch (error) {
      setValidationError(error instanceof Error ? error.message : 'Проверьте время исполнения')
      return
    }
    const payload: EventPayload = {
      kind: data.kind,
      necessity: data.kind === 'INCOME' ? 'REQUIRED' : data.necessity,
      amount: { amount: parseRubles(data.amount), currency: 'RUB' },
      category_id: data.categoryId,
      comment: data.comment.trim(),
      recurrence: {
        type: data.recurrenceType,
        start_date: data.startDate,
        day_of_month: data.recurrenceType === 'MONTHLY_ON_DAY' ? Number(data.dayOfMonth) : null,
        interval_days: data.recurrenceType === 'EVERY_N_DAYS' ? Number(data.intervalDays) : null,
        until_date: data.untilDate || null,
      },
    }
    locked.current = true
    try {
      const saved = await save.mutateAsync(payload)
      if (!isCurrentSession(session)) return
      if (!existing && asFact && !future) {
        const confirmation: OccurrencePayload = { eventId: saved.id, occurrenceDate: firstOccurrenceDate(payload.recurrence)!, status: 'CONFIRMED', ...(time ? { effective_at: time } : {}), ...(data.kind === 'EXPENSE' ? { counts_toward_day_budget: budget } : {}) }
        setPendingFact(confirmation)
        await confirm.mutateAsync(confirmation)
      }
      if (isCurrentSession(session)) closeEvent()
    } finally {
      locked.current = false
    }
  }

  const deleteEvent = async () => {
    if (!existing || !window.confirm('Удалить серию и будущие планы? Подтверждённые факты сохранятся.')) return
    const session = currentSession()
    await remove.mutateAsync(existing.id)
    if (isCurrentSession(session)) closeEvent()
  }

  const retryConfirmation = async () => {
    if (!pendingFact || locked.current) return
    const session = currentSession()
    locked.current = true
    try {
      await confirm.mutateAsync(pendingFact)
      if (isCurrentSession(session)) closeEvent()
    } finally {
      locked.current = false
    }
  }

  const categories = categoryData?.categories.filter((category) => category.kind === kind) ?? []
  const error = save.error ?? remove.error ?? confirm.error

  return (
    <Modal open={eventOpen} title={existing ? 'Редактировать событие' : 'Новое событие'} onClose={pending ? undefined : closeEvent}>
      {pendingFact ? <div className="grid gap-4">
        <p className="text-sm text-stone-700">Операция создана как план; подтверждение не завершено. Повтор будет применён к той же операции.</p>
        {confirm.error && <ErrorNotice>{errorMessage(confirm.error)}</ErrorNotice>}
        <Button disabled={pending} onClick={() => void retryConfirmation().catch(() => undefined)}>{pending ? 'Подтверждаем…' : 'Повторить подтверждение'}</Button>
        <Button variant="ghost" disabled={pending} onClick={closeEvent}>Закрыть — проверить в календаре</Button>
      </div> : (
      <form className="grid gap-5" onSubmit={handleSubmit((data) => submit(data).catch(() => undefined))}>
        {existing && <p className="rounded-xl bg-stone-100 p-3 text-sm text-stone-600">Вы изменяете серию. Суммы и даты уже подтверждённых фактов сохранятся. Подтверждение отдельного вхождения доступно в деталях дня.</p>}
        <div className="grid grid-cols-2 gap-2 rounded-xl bg-stone-100 p-1">
          {(['EXPENSE', 'INCOME'] as const).map((value) => (
            <label key={value} className="cursor-pointer">
              <input type="radio" value={value} className="peer sr-only" {...register('kind')} />
              <span className="block rounded-lg px-3 py-2 text-center text-sm font-semibold text-stone-500 peer-checked:bg-white peer-checked:text-stone-900 peer-checked:shadow-sm">
                {value === 'EXPENSE' ? 'Расход' : 'Доход'}
              </span>
            </label>
          ))}
        </div>
        {kind === 'EXPENSE' && (
          <Field label="Важность">
            <Select {...register('necessity')}>
              <option value="REQUIRED">Обязательный</option>
              <option value="OPTIONAL">Необязательный</option>
            </Select>
          </Field>
        )}
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Сумма, ₽" error={errors.amount?.message}>
            <Input inputMode="decimal" placeholder="2 500" autoFocus {...register('amount')} />
          </Field>
          <Field label="Категория">
            <Select {...register('categoryId')}>
              <option value="">Без категории</option>
              {categories.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}
            </Select>
          </Field>
        </div>
        {eventOpen && kind === 'INCOME' && isSalaryCategory(categories.find((category) => category.id === categoryId)) && (
          <SalaryCalculator key={`${editingEventId ?? 'new'}-${categoryId}`} onApply={(amount) => setValue('amount', kopecksToInput(amount), { shouldDirty: true, shouldValidate: true })} />
        )}
        <Field label="Комментарий" error={errors.comment?.message}>
          <textarea className="min-h-20 w-full resize-y rounded-xl border border-stone-200 bg-white p-3 text-sm outline-none focus:border-teal-700 focus:ring-3 focus:ring-teal-700/10" placeholder="Например, аренда квартиры" {...register('comment')} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Дата начала" error={errors.startDate?.message}><Input type="date" {...register('startDate')} /></Field>
          <Field label="Повторение">
            <Select {...register('recurrenceType')}>
              <option value="NONE">Не повторять</option>
              <option value="MONTHLY_ON_DAY">Каждый месяц</option>
              <option value="EVERY_N_DAYS">Каждые N дней</option>
            </Select>
          </Field>
        </div>
        {recurrenceType === 'MONTHLY_ON_DAY' && <Field label="День месяца" error={errors.dayOfMonth?.message}><Input type="number" min="1" max="31" {...register('dayOfMonth')} /></Field>}
        {recurrenceType === 'EVERY_N_DAYS' && <Field label="Интервал, дней" error={errors.intervalDays?.message}><Input type="number" min="1" {...register('intervalDays')} /></Field>}
        {recurrenceType !== 'NONE' && <Field label="Повторять до (необязательно)"><Input type="date" {...register('untilDate')} /></Field>}
        {!existing && <div className="grid gap-3">
          <Field label="Состояние первого вхождения">
            <Select value={!future && asFact ? 'CONFIRMED' : 'PLANNED'} disabled={pending} onChange={(event) => setAsFact(event.target.value === 'CONFIRMED')}>
              <option value="PLANNED">План</option>
              {!future && <option value="CONFIRMED">Факт — уже исполнено</option>}
            </Select>
          </Field>
          {future && <p className="text-xs text-stone-500">Будущая операция сохраняется как план.</p>}
          {!future && asFact && <FactFields occurrenceDate={firstDate!} effectiveTime={effectiveTime} onTimeChange={setEffectiveTime} expense={kind === 'EXPENSE'} countsTowardBudget={budget} onBudgetChange={setBudget} />}
          {recurrenceType !== 'NONE' && <p className="text-xs text-stone-500">Первое вхождение: {firstDate ?? 'нет в выбранном периоде'}. Следующие повторения будут планами. Факт относится только к первому вхождению.</p>}
        </div>}
        {validationError && <ErrorNotice>{validationError}</ErrorNotice>}
        {error && <ErrorNotice>{errorMessage(error)}</ErrorNotice>}
        <div className="flex gap-3">
          {existing && <Button type="button" variant="danger" className="px-3" onClick={() => void deleteEvent().catch(() => undefined)} disabled={pending}><Trash2 size={17} /><span className="sr-only sm:not-sr-only">Удалить</span></Button>}
          <Button type="submit" className="ml-auto flex-1 sm:flex-none" disabled={pending || Boolean(editingEventId && !existing)}><Save size={17} />{pending ? 'Сохраняем…' : 'Сохранить'}</Button>
        </div>
      </form>
      )}
    </Modal>
  )
}

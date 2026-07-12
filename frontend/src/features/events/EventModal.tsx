import { zodResolver } from '@hookform/resolvers/zod'
import { Save, Trash2 } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useCategories, useDeleteEvent, useEvents, useSaveEvent, type EventPayload } from '../../api/hooks'
import { errorMessage } from '../../api/client'
import { Button, ErrorNotice, Field, Input, Modal, Select } from '../../components/ui'
import { today } from '../../lib/dates'
import { kopecksToInput, parseRubles } from '../../lib/money'
import { useUIStore } from '../../stores/uiStore'

const schema = z.object({
  kind: z.enum(['INCOME', 'EXPENSE']),
  necessity: z.enum(['REQUIRED', 'OPTIONAL']),
  amount: z.string().refine((value) => parseRubles(value) > 0, 'Сумма должна быть больше нуля'),
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
  const save = useSaveEvent(existing)
  const remove = useDeleteEvent()
  const { register, handleSubmit, reset, watch, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: initialValues(),
  })
  const kind = watch('kind')
  const recurrenceType = watch('recurrenceType')

  useEffect(() => {
    if (!eventOpen) return
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

  const submit = async (data: FormData) => {
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
    await save.mutateAsync(payload)
    closeEvent()
  }

  const deleteEvent = async () => {
    if (!existing || !window.confirm('Удалить событие и все его повторения?')) return
    await remove.mutateAsync(existing.id)
    closeEvent()
  }

  const categories = categoryData?.categories.filter((category) => category.kind === kind) ?? []
  const error = save.error ?? remove.error

  return (
    <Modal open={eventOpen} title={existing ? 'Редактировать событие' : 'Новое событие'} onClose={closeEvent}>
      <form className="grid gap-5" onSubmit={handleSubmit(submit)}>
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
        {error && <ErrorNotice>{errorMessage(error)}</ErrorNotice>}
        <div className="flex gap-3">
          {existing && <Button type="button" variant="danger" className="px-3" onClick={deleteEvent} disabled={remove.isPending}><Trash2 size={17} /><span className="sr-only sm:not-sr-only">Удалить</span></Button>}
          <Button type="submit" className="ml-auto flex-1 sm:flex-none" disabled={save.isPending || remove.isPending}><Save size={17} />{save.isPending ? 'Сохраняем…' : 'Сохранить'}</Button>
        </div>
      </form>
    </Modal>
  )
}

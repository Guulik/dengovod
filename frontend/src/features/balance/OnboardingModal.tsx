import { zodResolver } from '@hookform/resolvers/zod'
import { CheckCircle2, Sparkles } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useSaveSettings, useSetAnchor } from '../../api/hooks'
import type { HorizonPolicy } from '../../lib/horizon'
import { errorMessage } from '../../api/client'
import { Button, ErrorNotice, Field, Input, Modal, Select } from '../../components/ui'
import { horizonPolicyLabels } from '../../lib/horizon'
import { parseRubles } from '../../lib/money'

const moneyInput = z.string().refine((value) => {
  const parsed = parseRubles(value)
  return Number.isInteger(parsed) && parsed >= 0
}, 'Введите сумму в рублях')
const schema = z
  .object({
    balance: moneyInput,
    dayCost: moneyInput,
    horizonPolicy: z.enum(['END_OF_MONTH', 'NEXT_INCOME', 'CUSTOM_DATE']),
    horizonCustomDate: z.string(),
  })
  .refine((data) => data.horizonPolicy !== 'CUSTOM_DATE' || /^\d{4}-\d{2}-\d{2}$/.test(data.horizonCustomDate), {
    message: 'Укажите дату горизонта',
    path: ['horizonCustomDate'],
  })
type FormData = z.infer<typeof schema>

export function OnboardingModal({ open, onClose }: { open: boolean; onClose?: () => void }) {
  const saveSettings = useSaveSettings()
  const setAnchor = useSetAnchor()
  const { register, handleSubmit, watch, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: { balance: '', dayCost: '', horizonPolicy: 'NEXT_INCOME', horizonCustomDate: '' },
  })
  const horizonPolicy = watch('horizonPolicy')
  const pending = saveSettings.isPending || setAnchor.isPending
  const error = saveSettings.error ?? setAnchor.error

  const submit = async (data: FormData) => {
    await saveSettings.mutateAsync({
      base_day_cost: { amount: parseRubles(data.dayCost), currency: 'RUB' },
      horizon_policy: data.horizonPolicy,
      horizon_custom_date: data.horizonPolicy === 'CUSTOM_DATE' ? data.horizonCustomDate : null,
    })
    await setAnchor.mutateAsync(parseRubles(data.balance))
  }

  return (
    <Modal open={open} title="Настроим ваш прогноз" onClose={onClose}>
      <div className="mb-6 rounded-2xl bg-teal-800 p-5 text-white">
        <Sparkles className="mb-3 text-amber-300" size={24} />
        <p className="font-bold">Три параметра — и всё готово</p>
        <p className="mt-1 text-sm leading-5 text-teal-100">Баланс, стоимость дня и горизонт определяют «деньги на жизнь» и цифры в календаре.</p>
      </div>
      <form className="grid gap-5" onSubmit={handleSubmit((data) => submit(data).catch(() => undefined))}>
        <Field label="Текущий баланс, ₽" error={errors.balance?.message} hint="Сколько денег доступно прямо сейчас">
          <Input inputMode="decimal" placeholder="85 000" autoFocus {...register('balance')} />
        </Field>
        <Field label="Базовая стоимость дня, ₽" error={errors.dayCost?.message} hint="Обычные ежедневные расходы: еда, транспорт, мелочи">
          <Input inputMode="decimal" placeholder="1 500" {...register('dayCost')} />
        </Field>
        <Field label="Горизонт планирования" error={errors.horizonPolicy?.message} hint="До какой даты считать обязательные расходы и дневные траты">
          <Select {...register('horizonPolicy')}>
            {(Object.entries(horizonPolicyLabels) as [HorizonPolicy, string][]).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
        </Field>
        {horizonPolicy === 'CUSTOM_DATE' && (
          <Field label="Дата горизонта" error={errors.horizonCustomDate?.message}>
            <Input type="date" {...register('horizonCustomDate')} />
          </Field>
        )}
        {error && <ErrorNotice>{errorMessage(error)}</ErrorNotice>}
        <Button type="submit" disabled={pending} className="w-full">
          <CheckCircle2 size={17} /> {pending ? 'Сохраняем…' : 'Начать планировать'}
        </Button>
        {onClose && <Button type="button" variant="ghost" onClick={onClose}>Позже — перейти к календарю</Button>}
      </form>
    </Modal>
  )
}

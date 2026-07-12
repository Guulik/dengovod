import { zodResolver } from '@hookform/resolvers/zod'
import { CheckCircle2, Sparkles } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useSaveSettings, useSetAnchor } from '../../api/hooks'
import { errorMessage } from '../../api/client'
import { Button, ErrorNotice, Field, Input, Modal } from '../../components/ui'
import { parseRubles } from '../../lib/money'

const moneyInput = z.string().refine((value) => {
  const parsed = parseRubles(value)
  return Number.isInteger(parsed) && parsed >= 0
}, 'Введите сумму в рублях')
const schema = z.object({ balance: moneyInput, dayCost: moneyInput })
type FormData = z.infer<typeof schema>

export function OnboardingModal({ open }: { open: boolean }) {
  const saveSettings = useSaveSettings()
  const setAnchor = useSetAnchor()
  const { register, handleSubmit, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: { balance: '', dayCost: '' },
  })
  const pending = saveSettings.isPending || setAnchor.isPending
  const error = saveSettings.error ?? setAnchor.error

  const submit = async (data: FormData) => {
    await saveSettings.mutateAsync({
      base_day_cost: { amount: parseRubles(data.dayCost), currency: 'RUB' },
      horizon_policy: 'END_OF_MONTH',
      horizon_custom_date: null,
    })
    await setAnchor.mutateAsync(parseRubles(data.balance))
  }

  return (
    <Modal open={open} title="Настроим ваш прогноз">
      <div className="mb-6 rounded-2xl bg-teal-800 p-5 text-white">
        <Sparkles className="mb-3 text-amber-300" size={24} />
        <p className="font-bold">Два числа — и всё готово</p>
        <p className="mt-1 text-sm leading-5 text-teal-100">Они станут отправной точкой расчёта денег до конца месяца.</p>
      </div>
      <form className="grid gap-5" onSubmit={handleSubmit(submit)}>
        <Field label="Текущий баланс, ₽" error={errors.balance?.message} hint="Сколько денег доступно прямо сейчас">
          <Input inputMode="decimal" placeholder="85 000" autoFocus {...register('balance')} />
        </Field>
        <Field label="Базовая стоимость дня, ₽" error={errors.dayCost?.message} hint="Обычные ежедневные расходы: еда, транспорт, мелочи">
          <Input inputMode="decimal" placeholder="1 500" {...register('dayCost')} />
        </Field>
        {error && <ErrorNotice>{errorMessage(error)}</ErrorNotice>}
        <Button type="submit" disabled={pending} className="w-full">
          <CheckCircle2 size={17} /> {pending ? 'Сохраняем…' : 'Начать планировать'}
        </Button>
      </form>
    </Modal>
  )
}

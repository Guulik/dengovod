import { useRef, useState, type FormEvent } from 'react'
import { useSaveOverride } from '../../api/hooks'
import { errorMessage } from '../../api/client'
import type { Money } from '../../api/schemas'
import { currentSession, isCurrentSession } from '../../api/session'
import { Button, ErrorNotice, Field, Input } from '../../components/ui'
import { formatDate } from '../../lib/dates'
import { formatMoney, kopecksToInput, parseRubles } from '../../lib/money'

export function DayCostEditor({ date, currentCost }: { date: string; currentCost?: Money }) {
  const [editing, setEditing] = useState(false)
  const [amount, setAmount] = useState('')
  const locked = useRef(false)
  // Append a single-day rule on every save. Updating an old rule would retain its
  // original priority and could lose to a period created since that rule.
  const save = useSaveOverride()
  const parsed = parseRubles(amount)
  const invalid = !amount.trim() || !Number.isSafeInteger(parsed) || parsed < 0

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (invalid || locked.current) return
    const session = currentSession()
    locked.current = true
    try {
      await save.mutateAsync({
        date_from: date, date_to: date,
        daily_amount: { amount: parsed, currency: 'RUB' },
        comment: `Стоимость дня ${date}`,
      })
      if (isCurrentSession(session)) setEditing(false)
    } finally {
      locked.current = false
    }
  }

  if (!editing) return (
    <Button type="button" variant="secondary" className="mt-3 w-full" disabled={!currentCost} onClick={() => {
      if (!currentCost) return
      save.reset()
      setAmount(kopecksToInput(currentCost.amount))
      setEditing(true)
    }}>Изменить стоимость дня</Button>
  )

  return (
    <form className="mt-3 grid min-w-0 gap-3 rounded-2xl border border-teal-200 bg-teal-50 p-4" aria-label="Стоимость выбранного дня" onSubmit={(event) => void submit(event).catch(() => undefined)}>
      <p className="text-sm font-bold">Стоимость дня · {formatDate(date)}</p>
      <p className="text-xs text-stone-600">Сейчас: {currentCost ? formatMoney(currentCost) : '—'}. Изменение относится только к этой дате. Ноль — день без повседневных расходов.</p>
      <Field label="Стоимость этого дня, ₽">
        <Input inputMode="decimal" autoFocus value={amount} disabled={save.isPending} onChange={(event) => setAmount(event.target.value)} />
      </Field>
      {invalid && <p className="text-xs text-red-600">Введите сумму от нуля с точностью до копейки.</p>}
      {save.error && <ErrorNotice>{errorMessage(save.error)}</ErrorNotice>}
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="ghost" className="flex-1" disabled={save.isPending} onClick={() => setEditing(false)}>Отмена</Button>
        <Button type="submit" className="flex-1" disabled={invalid || save.isPending}>{save.isPending ? 'Сохраняем…' : 'Сохранить стоимость'}</Button>
      </div>
    </form>
  )
}

import { useEffect, useState } from 'react'
import { errorMessage } from '../../api/client'
import { useBalance, useSetAnchor } from '../../api/hooks'
import { Button, ErrorNotice, Field, Input, Modal } from '../../components/ui'
import { formatDate, toApiDate } from '../../lib/dates'
import { formatMoney, kopecksToInput, parseRubles } from '../../lib/money'
import { useUIStore } from '../../stores/uiStore'

export function BalanceModal() {
  const open = useUIStore((state) => state.balanceOpen)
  const setBalanceOpen = useUIStore((state) => state.setBalanceOpen)
  const { data: balance } = useBalance()
  const setAnchor = useSetAnchor()
  const [amount, setAmount] = useState('')

  useEffect(() => {
    if (!balance || !open) return
    setAmount(kopecksToInput(balance.computed_balance.amount))
  }, [balance, open])

  const parsed = parseRubles(amount)
  const invalid = !Number.isInteger(parsed) || parsed < 0

  const save = async () => {
    if (invalid) return
    await setAnchor.mutateAsync(parsed)
    setBalanceOpen(false)
  }

  const anchorDate = balance ? toApiDate(new Date(balance.anchor.anchored_at)) : null

  return (
    <Modal open={open} onClose={() => setBalanceOpen(false)} title="Текущий баланс">
      <div className="space-y-4">
        <p className="text-sm text-stone-600">
          Укажите, сколько денег у вас сейчас. Прогноз и «деньги на жизнь» пересчитаются от этой точки.
        </p>
        {balance && (
          <div className="rounded-2xl bg-stone-100 p-4 text-sm">
            <p>
              Сейчас в расчёте: <strong>{formatMoney(balance.computed_balance)}</strong>
            </p>
            {anchorDate && (
              <p className="mt-1 text-stone-500">
                Последняя точка: {formatMoney(balance.anchor.amount)} · {formatDate(anchorDate)}
              </p>
            )}
          </div>
        )}
        <Field label="Новый баланс, ₽" hint="Сумма на счетах прямо сейчас">
          <Input inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} autoFocus />
        </Field>
        {setAnchor.error && <ErrorNotice>{errorMessage(setAnchor.error)}</ErrorNotice>}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setBalanceOpen(false)}>
            Отмена
          </Button>
          <Button onClick={() => void save()} disabled={setAnchor.isPending || invalid}>
            {setAnchor.isPending ? 'Сохраняем…' : 'Сохранить'}
          </Button>
        </div>
      </div>
    </Modal>
  )
}

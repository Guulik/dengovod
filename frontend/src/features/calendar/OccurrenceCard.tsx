import { useRef, useState } from 'react'
import { useSetOccurrenceState, type OccurrencePayload } from '../../api/hooks'
import type { Occurrence } from '../../api/schemas'
import { errorMessage } from '../../api/client'
import { Button, ErrorNotice } from '../../components/ui'
import { today } from '../../lib/dates'
import { formatMoney } from '../../lib/money'
import { defaultEffectiveTime, effectiveTimeToApi, moscowDateTime } from '../../lib/occurrences'
import { useUIStore } from '../../stores/uiStore'
import { FactFields } from '../events/FactFields'
import { currentSession, isCurrentSession } from '../../api/session'

const statusLabels = { PLANNED: 'План', CONFIRMED: 'Факт', CANCELLED: 'Отменено' }

export function OccurrenceCard({ occurrence }: { occurrence: Occurrence }) {
  const update = useSetOccurrenceState()
  const openEvent = useUIStore((state) => state.openEvent)
  const [confirming, setConfirming] = useState(false)
  const [effectiveTime, setEffectiveTime] = useState(() => defaultEffectiveTime(occurrence.occurrence_date))
  const [budget, setBudget] = useState(false)
  const [validationError, setValidationError] = useState('')
  const [confirmation, setConfirmation] = useState<OccurrencePayload>()
  const locked = useRef(false)
  const future = occurrence.occurrence_date > today()
  const change = async (status: Occurrence['status']) => {
    if (locked.current) return
    const session = currentSession()
    setValidationError('')
    try {
      const payload: OccurrencePayload = status === 'CONFIRMED' && confirmation ? confirmation : {
        eventId: occurrence.event_id, occurrenceDate: occurrence.occurrence_date, status,
        ...(status === 'CONFIRMED' ? { effective_at: effectiveTimeToApi(effectiveTime), ...(occurrence.kind === 'EXPENSE' ? { counts_toward_day_budget: budget } : {}) } : {}),
      }
      if (status === 'CONFIRMED') setConfirmation(payload)
      locked.current = true
      const result = await update.mutateAsync(payload)
      if (isCurrentSession(session) && status === 'PLANNED' && !result.plan_restored) useUIStore.getState().setNotice('Подтверждение снято. План не восстановлен: правило повторения изменилось')
      setConfirming(false)
      setConfirmation(undefined)
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError')) setValidationError(error instanceof Error ? error.message : errorMessage(error))
    } finally {
      locked.current = false
    }
  }

  return (
    <article className="grid gap-3 rounded-2xl border border-stone-200 bg-white p-3">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="break-words text-sm font-bold">{occurrence.comment || 'Без комментария'}</p>
          <p className="mt-1 text-xs text-stone-500">{statusLabels[occurrence.status]} · {occurrence.kind === 'INCOME' ? 'Доход' : occurrence.necessity === 'OPTIONAL' ? 'Необязательный расход' : 'Обязательный расход'}</p>
        </div>
        <strong className={`shrink-0 ${occurrence.kind === 'INCOME' ? 'text-emerald-700' : 'text-stone-900'}`}>{occurrence.kind === 'INCOME' ? '+' : '−'}{formatMoney(occurrence.amount)}</strong>
      </div>
      {occurrence.effective_at && <p className="text-xs text-stone-500">Исполнено: {moscowDateTime(new Date(occurrence.effective_at)).replace('T', ' ')} (Москва). Исходная дата: {occurrence.occurrence_date}.{occurrence.counts_toward_day_budget ? ' Из стоимости дня.' : ''}</p>}
      {occurrence.series_deleted ? <p className="text-xs text-stone-500">Серия удалена. Сохранённый факт доступен для просмотра.</p> : (
        <>
          {confirming && <FactFields effectiveTime={effectiveTime} onTimeChange={(value) => { setEffectiveTime(value); setConfirmation(undefined) }} expense={occurrence.kind === 'EXPENSE'} countsTowardBudget={budget} onBudgetChange={(value) => { setBudget(value); setConfirmation(undefined) }} />}
          {confirmation && update.error && <p className="text-xs text-stone-500">Повтор сохраняет время и параметры первого нажатия. Чтобы изменить время, укажите его явно.</p>}
          <div className="flex flex-wrap gap-2">
            {occurrence.status === 'PLANNED' && !future && (confirming ? (
              <Button disabled={update.isPending} onClick={() => void change('CONFIRMED')}>{update.isPending ? 'Подтверждаем…' : confirmation ? 'Повторить подтверждение' : 'Подтвердить факт'}</Button>
            ) : <Button variant="secondary" onClick={() => setConfirming(true)}>Подтвердить исполнение</Button>)}
            {occurrence.status === 'PLANNED' && <Button variant="ghost" disabled={update.isPending} onClick={() => void change('CANCELLED')}>Отменить план</Button>}
            {occurrence.status !== 'PLANNED' && <Button variant="secondary" disabled={update.isPending} onClick={() => void change('PLANNED')}>{update.isPending ? 'Обновляем…' : occurrence.status === 'CONFIRMED' ? 'Снять подтверждение' : 'Восстановить план'}</Button>}
            <Button variant="ghost" disabled={update.isPending} onClick={() => openEvent(occurrence.event_id)}>Изменить серию</Button>
          </div>
          {future && occurrence.status === 'PLANNED' && <p className="text-xs text-stone-500">Факт можно подтвердить в день операции или позже.</p>}
        </>
      )}
      {(validationError || update.error) && <ErrorNotice>{validationError || errorMessage(update.error)}</ErrorNotice>}
    </article>
  )
}

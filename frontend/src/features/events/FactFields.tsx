import { Field, Input } from '../../components/ui'
import { moscowDateTime } from '../../lib/occurrences'
import { today } from '../../lib/dates'

export function FactFields({ effectiveTime, onTimeChange, expense, countsTowardBudget, onBudgetChange, occurrenceDate }: {
  occurrenceDate: string
  effectiveTime: string
  onTimeChange: (value: string) => void
  expense: boolean
  countsTowardBudget: boolean
  onBudgetChange: (value: boolean) => void
}) {
  return (
    <div className="grid gap-3 rounded-xl bg-stone-100 p-3">
      <Field label="Фактические дата и время (Москва)" hint={`${occurrenceDate === today() ? 'Без времени — исполнить сейчас, после последней сверки.' : `Без времени — 12:00 ${occurrenceDate} (Москва).`} Если операция уже включена в остаток, укажите время до сверки.`}>
        <Input type="datetime-local" min="0001-01-01T00:00" max={moscowDateTime()} step="1" value={effectiveTime} onChange={(event) => onTimeChange(event.target.value)} />
      </Field>
      {expense && (
        <label className="flex items-start gap-2 text-sm text-stone-700">
          <input type="checkbox" className="mt-1 size-4 shrink-0 accent-teal-800" checked={countsTowardBudget} onChange={(event) => onBudgetChange(event.target.checked)} />
          Повседневная трата — из стоимости дня
        </label>
      )}
    </div>
  )
}

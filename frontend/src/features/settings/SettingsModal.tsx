import { Pencil, Plus, Save, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { useDeleteOverride, useOverrides, useSaveOverride, useSaveSettings, useSettings } from '../../api/hooks'
import type { DayCostOverride } from '../../api/schemas'
import { errorMessage } from '../../api/client'
import { Button, ErrorNotice, Field, Input, LoadingBlock, Modal, Select } from '../../components/ui'
import { formatDate, today } from '../../lib/dates'
import { formatMoney, kopecksToInput, parseRubles } from '../../lib/money'
import { useUIStore } from '../../stores/uiStore'

type Policy = 'END_OF_MONTH' | 'NEXT_INCOME' | 'CUSTOM_DATE'

function OverrideEditor({ value, onDone }: { value?: DayCostOverride; onDone: () => void }) {
  const save = useSaveOverride(value?.id)
  const [from, setFrom] = useState(value?.date_from ?? today())
  const [to, setTo] = useState(value?.date_to ?? today())
  const [amount, setAmount] = useState(value ? kopecksToInput(value.daily_amount.amount) : '')
  const [comment, setComment] = useState(value?.comment ?? '')
  const invalid = !from || !to || to < from || !(parseRubles(amount) >= 0)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    await save.mutateAsync({
      date_from: from,
      date_to: to,
      daily_amount: { amount: parseRubles(amount), currency: 'RUB' },
      comment: comment.trim(),
    })
    onDone()
  }

  return (
    <form className="mt-4 grid gap-3 rounded-2xl border border-stone-200 bg-white p-4" onSubmit={(event) => void submit(event).catch(() => undefined)}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="С даты"><Input type="date" value={from} onChange={(event) => setFrom(event.target.value)} /></Field>
        <Field label="По дату"><Input type="date" value={to} onChange={(event) => setTo(event.target.value)} /></Field>
      </div>
      <Field label="Стоимость дня, ₽"><Input inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} placeholder="0 — день без расходов" /></Field>
      <Field label="Комментарий"><Input value={comment} onChange={(event) => setComment(event.target.value)} placeholder="Отпуск, командировка…" /></Field>
      {save.error && <ErrorNotice>{errorMessage(save.error)}</ErrorNotice>}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onDone}>Отмена</Button>
        <Button type="submit" disabled={invalid || save.isPending}>{save.isPending ? 'Сохраняем…' : 'Сохранить'}</Button>
      </div>
    </form>
  )
}

export function SettingsModal() {
  const open = useUIStore((state) => state.settingsOpen)
  const setOpen = useUIStore((state) => state.setSettingsOpen)
  const { data: settings, isLoading } = useSettings()
  const { data: overrides } = useOverrides()
  const saveSettings = useSaveSettings()
  const removeOverride = useDeleteOverride()
  const [dayCost, setDayCost] = useState('')
  const [policy, setPolicy] = useState<Policy>('END_OF_MONTH')
  const [customDate, setCustomDate] = useState('')
  const [editing, setEditing] = useState<DayCostOverride | 'new' | null>(null)

  useEffect(() => {
    if (!settings) return
    setDayCost(kopecksToInput(settings.base_day_cost.amount))
    setPolicy(settings.horizon_policy)
    setCustomDate(settings.horizon_custom_date ?? '')
  }, [settings])

  const submitSettings = async (event: FormEvent) => {
    event.preventDefault()
    await saveSettings.mutateAsync({
      base_day_cost: { amount: parseRubles(dayCost), currency: 'RUB' },
      horizon_policy: policy,
      horizon_custom_date: policy === 'CUSTOM_DATE' ? customDate : null,
    })
  }

  const sortedOverrides = [...(overrides?.overrides ?? [])].sort((a, b) => a.date_from.localeCompare(b.date_from))

  return (
    <Modal open={open} title="Настройки прогноза" onClose={() => setOpen(false)} wide>
      {isLoading ? <LoadingBlock /> : (
        <div className="grid gap-8 md:grid-cols-[1fr_1.1fr]">
          <form className="grid content-start gap-5" onSubmit={(event) => void submitSettings(event).catch(() => undefined)}>
            <div>
              <h3 className="font-bold">Основные параметры</h3>
              <p className="mt-1 text-sm text-stone-500">Используются для ежедневного прогноза.</p>
            </div>
            <Field label="Базовая стоимость дня, ₽">
              <Input inputMode="decimal" value={dayCost} onChange={(event) => setDayCost(event.target.value)} />
            </Field>
            <Field label="Горизонт планирования">
              <Select value={policy} onChange={(event) => setPolicy(event.target.value as Policy)}>
                <option value="END_OF_MONTH">До конца месяца</option>
                <option value="NEXT_INCOME">До следующего дохода</option>
                <option value="CUSTOM_DATE">До выбранной даты</option>
              </Select>
            </Field>
            {policy === 'CUSTOM_DATE' && <Field label="Дата горизонта"><Input type="date" value={customDate} onChange={(event) => setCustomDate(event.target.value)} required /></Field>}
            {saveSettings.error && <ErrorNotice>{errorMessage(saveSettings.error)}</ErrorNotice>}
            <Button type="submit" disabled={saveSettings.isPending || !Number.isFinite(parseRubles(dayCost))}><Save size={17} />{saveSettings.isPending ? 'Сохраняем…' : 'Сохранить настройки'}</Button>
          </form>

          <section>
            <div className="flex items-start justify-between gap-4">
              <div>
                <h3 className="font-bold">Особые дни</h3>
                <p className="mt-1 text-sm text-stone-500">Заменяют базовую стоимость в выбранном периоде.</p>
              </div>
              <Button type="button" variant="secondary" className="shrink-0 px-3" onClick={() => setEditing('new')}><Plus size={16} />Добавить</Button>
            </div>
            {editing === 'new' && <OverrideEditor onDone={() => setEditing(null)} />}
            <div className="mt-4 grid gap-2">
              {sortedOverrides.length === 0 && !editing && <div className="rounded-2xl border border-dashed border-stone-300 p-6 text-center text-sm text-stone-500">Особых дней пока нет</div>}
              {sortedOverrides.map((item) => editing !== item ? (
                <article key={item.id} className="flex items-center gap-3 rounded-2xl border border-stone-200 bg-white p-3">
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-bold">{formatMoney(item.daily_amount)} · {item.comment || 'Без комментария'}</p>
                    <p className="mt-0.5 text-xs text-stone-500">{formatDate(item.date_from)} — {formatDate(item.date_to)}</p>
                  </div>
                  <Button type="button" variant="ghost" className="size-9 p-0" aria-label="Изменить" onClick={() => setEditing(item)}><Pencil size={15} /></Button>
                  <Button type="button" variant="ghost" className="size-9 p-0 text-red-600" aria-label="Удалить" onClick={() => removeOverride.mutate(item.id)} disabled={removeOverride.isPending}><Trash2 size={15} /></Button>
                </article>
              ) : <OverrideEditor key={item.id} value={item} onDone={() => setEditing(null)} />)}
            </div>
            {removeOverride.error && <div className="mt-3"><ErrorNotice>{errorMessage(removeOverride.error)}</ErrorNotice></div>}
          </section>
        </div>
      )}
    </Modal>
  )
}

import { useEffect, useState } from 'react'
import { useForecast, useSaveSettings, useSettings } from '../../api/hooks'
import type { HorizonPolicy } from '../../lib/horizon'
import { Button, Field, Input, Modal, Select } from '../../components/ui'
import { useUIStore } from '../../stores/uiStore'

const policyOptions: { value: HorizonPolicy; label: string }[] = [
  { value: 'END_OF_MONTH', label: 'До конца месяца' },
  { value: 'NEXT_INCOME', label: 'До следующего дохода' },
  { value: 'CUSTOM_DATE', label: 'До выбранной даты' },
]

export function HorizonModal() {
  const open = useUIStore((state) => state.horizonOpen)
  const setHorizonOpen = useUIStore((state) => state.setHorizonOpen)
  const { data: settings } = useSettings()
  const { data: forecast } = useForecast()
  const saveSettings = useSaveSettings()

  const [policy, setPolicy] = useState<HorizonPolicy>('END_OF_MONTH')
  const [customDate, setCustomDate] = useState('')

  useEffect(() => {
    if (!settings) return
    setPolicy(settings.horizon_policy)
    setCustomDate(settings.horizon_custom_date ?? '')
  }, [settings, open])

  const save = async () => {
    if (!settings) return
    await saveSettings.mutateAsync({
      horizon_policy: policy,
      horizon_custom_date: policy === 'CUSTOM_DATE' ? customDate : null,
      base_day_cost: settings.base_day_cost,
    })
    setHorizonOpen(false)
  }

  return (
    <Modal open={open} onClose={() => setHorizonOpen(false)} title="Горизонт расчёта">
      <div className="space-y-4">
        <p className="text-sm text-stone-600">
          От горизонта зависят «деньги на жизнь» и прогноз в календаре. Сейчас:{' '}
          <strong>{forecast?.days_until_horizon ?? '—'} дн.</strong>
          {forecast?.horizon_date ? ` (до ${forecast.horizon_date})` : ''}
        </p>
        <Field label="Политика">
          <Select value={policy} onChange={(event) => setPolicy(event.target.value as HorizonPolicy)}>
            {policyOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </Field>
        {policy === 'CUSTOM_DATE' ? (
          <Field label="Дата горизонта">
            <Input type="date" value={customDate} onChange={(event) => setCustomDate(event.target.value)} />
          </Field>
        ) : null}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setHorizonOpen(false)}>
            Отмена
          </Button>
          <Button onClick={() => void save()} disabled={saveSettings.isPending || (policy === 'CUSTOM_DATE' && !customDate)}>
            Сохранить
          </Button>
        </div>
      </div>
    </Modal>
  )
}

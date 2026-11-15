import { useState } from 'react'
import { Button, Field, Input, Select } from '../../components/ui'
import { today } from '../../lib/dates'
import { formatMoney } from '../../lib/money'
import { calculateSalary, calendarDaysInMonth, countWorkedDates, parseSalaryRubles } from '../../lib/salary'

export function SalaryCalculator({ onApply }: { onApply: (kopecks: number) => void }) {
  const [salary, setSalary] = useState('')
  const [month, setMonth] = useState(() => today().slice(0, 7))
  const [mode, setMode] = useState<'count' | 'dates'>('count')
  const [count, setCount] = useState('0')
  const [dates, setDates] = useState<string[]>([])
  let result: number | null = null
  let error = ''
  let denominator = 0
  let workedDays = 0
  try {
    denominator = calendarDaysInMonth(month)
    if (mode === 'count' && !/^\d+$/.test(count)) throw new Error(`Укажите целое число дней от 0 до ${denominator}`)
    workedDays = mode === 'count' ? Number(count) : countWorkedDates(month, dates)
    result = calculateSalary(parseSalaryRubles(salary), workedDays, month)
  } catch (cause) {
    error = cause instanceof Error ? cause.message : 'Проверьте параметры расчёта'
  }
  const clearDays = () => { setCount('0'); setDates([]) }

  return (
    <fieldset className="min-w-0 rounded-2xl border border-teal-200 bg-teal-50/60 p-4">
      <legend className="px-1 text-sm font-bold text-teal-900">Калькулятор зарплаты</legend>
      <div className="grid gap-4">
        <Field label="Оклад за полный месяц, ₽"><Input inputMode="decimal" value={salary} onChange={(event) => setSalary(event.target.value)} placeholder="100 000" /></Field>
        <Field label="Расчётный месяц"><Input type="month" value={month} onChange={(event) => { setMonth(event.target.value); clearDays() }} /></Field>
        <Field label="Как указать отработанные дни">
          <Select value={mode} onChange={(event) => { setMode(event.target.value as 'count' | 'dates'); clearDays() }}>
            <option value="count">Числом дней</option><option value="dates">Выбрать даты</option>
          </Select>
        </Field>
        {mode === 'count' ? (
          <Field label="Отработано календарных дней"><Input type="number" min="0" max={denominator || 31} step="1" value={count} onChange={(event) => setCount(event.target.value)} /></Field>
        ) : (
          <div role="group" aria-label="Отработанные даты" className="grid grid-cols-7 gap-1">
            {Array.from({ length: denominator }, (_, index) => {
              const date = `${month}-${String(index + 1).padStart(2, '0')}`
              const checked = dates.includes(date)
              return (
                <label key={date} className={`flex min-h-11 min-w-0 cursor-pointer items-center justify-center rounded-lg text-sm ${checked ? 'bg-teal-800 text-white' : 'border border-teal-100 bg-white text-stone-700'}`}>
                  <input type="checkbox" className="peer sr-only" aria-label={date} checked={checked} onChange={() => setDates((previous) => checked ? previous.filter((value) => value !== date) : [...previous, date])} />
                  <span className="peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4">{index + 1}</span>
                </label>
              )
            })}
          </div>
        )}
        <p className="text-xs leading-5 text-stone-600">Оклад × отработанные дни / календарные дни месяца ({denominator || '—'}). Это расчёт по календарным дням, без производственного календаря.</p>
        <p aria-live="polite" className="text-sm font-semibold">{result === null ? error : `Итого: ${formatMoney({ amount: result, currency: 'RUB' })} (${workedDays} из ${denominator} дн.)`}</p>
        <Button type="button" variant="secondary" disabled={result === null} onClick={() => result !== null && onApply(result)}>Применить к сумме</Button>
        {result === 0 && <p className="text-xs text-stone-600">Расчёт равен нулю. Сохранить операцию можно только с положительной суммой.</p>}
        <p className="text-xs leading-5 text-stone-500">Сохраняется только итоговая сумма, формула не сохраняется. Сумму можно изменить вручную; дата выплаты остаётся датой операции.</p>
      </div>
    </fieldset>
  )
}

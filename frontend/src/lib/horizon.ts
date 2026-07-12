export type HorizonPolicy = 'END_OF_MONTH' | 'NEXT_INCOME' | 'CUSTOM_DATE'

export const horizonPolicyLabels: Record<HorizonPolicy, string> = {
  END_OF_MONTH: 'До конца месяца',
  NEXT_INCOME: 'До следующего дохода',
  CUSTOM_DATE: 'До выбранной даты',
}

export function formatHorizonDate(isoDate: string): string {
  const [year, month, day] = isoDate.split('-').map(Number)
  return new Intl.DateTimeFormat('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  }).format(new Date(year, month - 1, day))
}

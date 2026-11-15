import { format, parseISO } from 'date-fns'
import { ru } from 'date-fns/locale'

export const toApiDate = (date: Date) => format(date, 'yyyy-MM-dd')
export const moscowDate = (date: Date) => new Intl.DateTimeFormat('sv-SE', {
  timeZone: 'Europe/Moscow', year: 'numeric', month: '2-digit', day: '2-digit',
}).format(date)
export const today = () => moscowDate(new Date())
export const formatDate = (value: string) => format(parseISO(value), 'd MMMM yyyy', { locale: ru })

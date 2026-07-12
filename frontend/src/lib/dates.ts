import { format, parseISO } from 'date-fns'
import { ru } from 'date-fns/locale'

export const toApiDate = (date: Date) => format(date, 'yyyy-MM-dd')
export const today = () => toApiDate(new Date())
export const formatDate = (value: string) => format(parseISO(value), 'd MMMM yyyy', { locale: ru })

import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '../stores/authStore'
import { moscowDate, today } from '../lib/dates'
import { queryKeys } from './hooks'
import { isCurrentSession } from './session'

export function millisecondsUntilMoscowMidnight(now = new Date()): number {
  const midnight = new Date(`${moscowDate(now)}T00:00:00+03:00`).getTime()
  return Math.max(1, midnight + 86_400_000 - now.getTime())
}

export function useFinancialDateRefresh(): string {
  const client = useQueryClient()
  const token = useAuthStore((state) => state.token)
  const sessionId = useAuthStore((state) => state.sessionId)
  const [date, setDate] = useState(today)

  useEffect(() => {
    if (!token) return
    const session = { token, sessionId }
    let stopped = false
    let lastDate = today()
    let midnightTimer: ReturnType<typeof setTimeout>
    let resumeTimer: ReturnType<typeof setTimeout> | undefined
    setDate(lastDate)

    const refresh = async () => {
      if (stopped || !isCurrentSession(session)) return
      // A pre-midnight request must not finish after the new day's refresh.
      await client.cancelQueries({ queryKey: queryKeys.session(sessionId) })
      if (stopped || !isCurrentSession(session)) return
      await client.invalidateQueries({ queryKey: queryKeys.session(sessionId) })
    }
    const syncDate = () => {
      const nextDate = today()
      const changed = nextDate !== lastDate
      lastDate = nextDate
      setDate(nextDate)
      return changed
    }
    const scheduleMidnight = () => {
      clearTimeout(midnightTimer)
      midnightTimer = setTimeout(() => {
        if (stopped || !isCurrentSession(session)) return
        if (syncDate()) void refresh()
        scheduleMidnight()
      }, millisecondsUntilMoscowMidnight())
    }
    const resume = () => {
      if (stopped || !isCurrentSession(session)) return
      syncDate()
      scheduleMidnight()
      // Browsers can emit visibility, focus and pageshow together on one resume.
      if (resumeTimer !== undefined) return
      resumeTimer = setTimeout(() => {
        resumeTimer = undefined
        void refresh()
      }, 0)
    }
    const visibilityChange = () => {
      if (document.visibilityState === 'visible') resume()
    }

    scheduleMidnight()
    window.addEventListener('focus', resume)
    window.addEventListener('pageshow', resume)
    document.addEventListener('visibilitychange', visibilityChange)
    return () => {
      stopped = true
      clearTimeout(midnightTimer)
      clearTimeout(resumeTimer)
      window.removeEventListener('focus', resume)
      window.removeEventListener('pageshow', resume)
      document.removeEventListener('visibilitychange', visibilityChange)
    }
  }, [client, token, sessionId])

  return date
}

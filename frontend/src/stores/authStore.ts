import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User } from '../api/schemas'
import { useUIStore } from './uiStore'

type AuthState = {
  token: string | null
  user: User | null
  sessionId: number
  setAuth: (token: string, user: User) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      sessionId: 0,
      setAuth: (token, user) => {
        useUIStore.getState().reset()
        set({ token, user, sessionId: get().sessionId + 1 })
      },
      logout: () => {
        useUIStore.getState().reset()
        set({ token: null, user: null, sessionId: get().sessionId + 1 })
      },
    }),
    { name: 'dengovod-auth', partialize: ({ token, user }) => ({ token, user }) },
  ),
)

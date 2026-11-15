import { create } from 'zustand'

type UIState = {
  selectedDate: string | null
  editingEventId: string | null
  eventOpen: boolean
  settingsOpen: boolean
  horizonOpen: boolean
  balanceOpen: boolean
  notice: string | null
  setNotice: (notice: string | null) => void
  setSelectedDate: (date: string | null) => void
  openEvent: (eventId?: string | null) => void
  closeEvent: () => void
  setSettingsOpen: (open: boolean) => void
  setHorizonOpen: (open: boolean) => void
  setBalanceOpen: (open: boolean) => void
  reset: () => void
}

const privateDefaults = { selectedDate: null, editingEventId: null, eventOpen: false, settingsOpen: false, horizonOpen: false, balanceOpen: false, notice: null }

export const useUIStore = create<UIState>((set) => ({
  ...privateDefaults,
  reset: () => set(privateDefaults),
  setNotice: (notice) => set({ notice }),
  setSelectedDate: (selectedDate) => set({ selectedDate }),
  openEvent: (editingEventId = null) => set({ eventOpen: true, editingEventId }),
  closeEvent: () => set({ eventOpen: false, editingEventId: null }),
  setSettingsOpen: (settingsOpen) => set({ settingsOpen }),
  setHorizonOpen: (horizonOpen) => set({ horizonOpen }),
  setBalanceOpen: (balanceOpen) => set({ balanceOpen }),
}))

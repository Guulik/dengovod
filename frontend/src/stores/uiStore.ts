import { create } from 'zustand'

type UIState = {
  selectedDate: string | null
  editingEventId: string | null
  eventOpen: boolean
  settingsOpen: boolean
  horizonOpen: boolean
  balanceOpen: boolean
  setSelectedDate: (date: string | null) => void
  openEvent: (eventId?: string | null) => void
  closeEvent: () => void
  setSettingsOpen: (open: boolean) => void
  setHorizonOpen: (open: boolean) => void
  setBalanceOpen: (open: boolean) => void
}

export const useUIStore = create<UIState>((set) => ({
  selectedDate: null,
  editingEventId: null,
  eventOpen: false,
  settingsOpen: false,
  horizonOpen: false,
  balanceOpen: false,
  setSelectedDate: (selectedDate) => set({ selectedDate }),
  openEvent: (editingEventId = null) => set({ eventOpen: true, editingEventId }),
  closeEvent: () => set({ eventOpen: false, editingEventId: null }),
  setSettingsOpen: (settingsOpen) => set({ settingsOpen }),
  setHorizonOpen: (horizonOpen) => set({ horizonOpen }),
  setBalanceOpen: (balanceOpen) => set({ balanceOpen }),
}))

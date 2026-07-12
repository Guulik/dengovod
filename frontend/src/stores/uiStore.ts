import { create } from 'zustand'

type UIState = {
  selectedDate: string | null
  editingEventId: string | null
  eventOpen: boolean
  settingsOpen: boolean
  setSelectedDate: (date: string | null) => void
  openEvent: (eventId?: string | null) => void
  closeEvent: () => void
  setSettingsOpen: (open: boolean) => void
}

export const useUIStore = create<UIState>((set) => ({
  selectedDate: null,
  editingEventId: null,
  eventOpen: false,
  settingsOpen: false,
  setSelectedDate: (selectedDate) => set({ selectedDate }),
  openEvent: (editingEventId = null) => set({ eventOpen: true, editingEventId }),
  closeEvent: () => set({ eventOpen: false, editingEventId: null }),
  setSettingsOpen: (settingsOpen) => set({ settingsOpen }),
}))

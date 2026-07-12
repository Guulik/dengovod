import { LogOut, Plus, Settings, WalletCards } from 'lucide-react'
import { Outlet } from 'react-router-dom'
import { Button } from './ui'
import { useAuthStore } from '../stores/authStore'
import { useUIStore } from '../stores/uiStore'

export function AppLayout() {
  const user = useAuthStore((state) => state.user)
  const logout = useAuthStore((state) => state.logout)
  const openEvent = useUIStore((state) => state.openEvent)
  const setSettingsOpen = useUIStore((state) => state.setSettingsOpen)

  return (
    <div className="min-h-svh bg-[#f4f1e9] text-stone-900">
      <header className="sticky top-0 z-30 border-b border-stone-200/70 bg-[#f4f1e9]/90 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-[1480px] items-center gap-3 px-4 sm:px-6">
          <div className="flex items-center gap-2.5">
            <div className="grid size-9 place-items-center rounded-xl bg-teal-800 text-white"><WalletCards size={18} /></div>
            <span className="hidden font-black tracking-tight sm:block">Деньговод</span>
          </div>
          <div className="ml-auto flex items-center gap-1 sm:gap-2">
            <span className="mr-2 hidden text-sm text-stone-500 md:block">{user?.login}</span>
            <Button className="px-3 sm:px-4" onClick={() => openEvent()}><Plus size={17} /><span className="hidden sm:inline">Событие</span></Button>
            <Button variant="ghost" className="size-10 p-0" aria-label="Настройки" onClick={() => setSettingsOpen(true)}><Settings size={18} /></Button>
            <Button variant="ghost" className="size-10 p-0" aria-label="Выйти" onClick={logout}><LogOut size={18} /></Button>
          </div>
        </div>
      </header>
      <Outlet />
    </div>
  )
}

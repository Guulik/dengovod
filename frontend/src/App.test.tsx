import { useState } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { useAuthStore } from './stores/authStore'
import { userA, userB } from './test/fixtures'

vi.mock('./features/calendar/Dashboard', () => ({
  Dashboard: function Draft() {
    const [draft, setDraft] = useState('')
    return <label>Приватный черновик<input value={draft} onChange={(event) => setDraft(event.target.value)} /></label>
  },
}))
afterEach(() => useAuthStore.getState().logout())
describe('protected financial screen lifecycle', () => {
  it('remounts all private local form state when switching directly between profiles', () => {
    useAuthStore.getState().setAuth('A', userA)
    render(<MemoryRouter><App /></MemoryRouter>)
    fireEvent.change(screen.getByLabelText('Приватный черновик'), { target: { value: 'Финансовые данные A' } })
    expect(screen.getByLabelText('Приватный черновик')).toHaveValue('Финансовые данные A')
    act(() => useAuthStore.getState().setAuth('B', userB))
    expect(screen.getByLabelText('Приватный черновик')).toHaveValue('')
  })
})

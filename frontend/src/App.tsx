import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { AppLayout } from './components/AppLayout'
import { LoginPage } from './features/auth/LoginPage'
import { Dashboard } from './features/calendar/Dashboard'
import { useAuthStore } from './stores/authStore'

function ProtectedRoute() {
  const token = useAuthStore((state) => state.token)
  return token ? <Outlet /> : <Navigate to="/login" replace />
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<AppLayout />}>
          <Route index element={<Dashboard />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default App

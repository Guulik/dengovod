import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowRight, Landmark } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { Navigate } from 'react-router-dom'
import { z } from 'zod'
import { useLogin } from '../../api/hooks'
import { errorMessage } from '../../api/client'
import { Button, ErrorNotice, Field, Input } from '../../components/ui'
import { useAuthStore } from '../../stores/authStore'

const schema = z.object({ login: z.string().trim().min(2, 'Введите хотя бы 2 символа').max(64) })
type FormData = z.infer<typeof schema>

export function LoginPage() {
  const token = useAuthStore((state) => state.token)
  const login = useLogin()
  const { register, handleSubmit, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: { login: '' },
  })

  if (token) return <Navigate to="/" replace />

  return (
    <main className="grid min-h-svh place-items-center bg-[#f4f1e9] p-4">
      <section className="w-full max-w-md rounded-[2rem] border border-white/70 bg-[#fcfbf8] p-7 shadow-[0_24px_70px_-30px_rgba(28,25,23,.35)] sm:p-9">
        <div className="mb-10 flex items-center gap-3">
          <div className="grid size-11 place-items-center rounded-2xl bg-teal-800 text-white"><Landmark size={22} /></div>
          <div>
            <p className="text-lg font-black tracking-tight text-stone-900">Деньговод</p>
            <p className="text-xs text-stone-500">Спокойный контроль денег</p>
          </div>
        </div>
        <h1 className="text-3xl font-black tracking-tight text-stone-900">Добро пожаловать</h1>
        <p className="mt-2 text-sm leading-6 text-stone-500">Войдите по имени — пароль для локального профиля не нужен.</p>
        <form className="mt-8 grid gap-5" onSubmit={handleSubmit((data) => login.mutate(data.login))}>
          <Field label="Имя пользователя" error={errors.login?.message}>
            <Input autoFocus autoComplete="username" placeholder="Например, алексей" {...register('login')} />
          </Field>
          {login.error && <ErrorNotice>{errorMessage(login.error)}</ErrorNotice>}
          <Button type="submit" disabled={login.isPending} className="w-full">
            {login.isPending ? 'Входим…' : 'Продолжить'} <ArrowRight size={17} />
          </Button>
        </form>
      </section>
    </main>
  )
}

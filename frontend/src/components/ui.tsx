import { X } from 'lucide-react'
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from 'react'
import { twMerge } from 'tailwind-merge'
import clsx from 'clsx'

const cn = (...values: Parameters<typeof clsx>) => twMerge(clsx(...values))

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
}
export function Button({ className, variant = 'primary', ...props }: ButtonProps) {
  return (
    <button
      className={cn(
        'inline-flex min-h-10 items-center justify-center gap-2 rounded-xl px-4 text-sm font-semibold transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:pointer-events-none disabled:opacity-50',
        variant === 'primary' && 'bg-teal-800 text-white hover:bg-teal-900',
        variant === 'secondary' && 'border border-stone-200 bg-white text-stone-800 hover:bg-stone-50',
        variant === 'danger' && 'bg-red-50 text-red-700 hover:bg-red-100',
        variant === 'ghost' && 'text-stone-600 hover:bg-stone-100',
        className,
      )}
      {...props}
    />
  )
}

type FieldProps = {
  label: string
  error?: string
  children: ReactNode
  hint?: string
}
export function Field({ label, error, children, hint }: FieldProps) {
  return (
    <label className="grid gap-1.5 text-sm font-medium text-stone-700">
      {label}
      {children}
      {error && <span className="text-xs text-red-600">{error}</span>}
      {!error && hint && <span className="text-xs font-normal text-stone-500">{hint}</span>}
    </label>
  )
}

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn('h-11 w-full rounded-xl border border-stone-200 bg-white px-3 text-stone-900 outline-none transition placeholder:text-stone-400 focus:border-teal-700 focus:ring-3 focus:ring-teal-700/10', className)} {...props} />
}

export function Select({ className, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn('h-11 w-full rounded-xl border border-stone-200 bg-white px-3 text-stone-900 outline-none focus:border-teal-700 focus:ring-3 focus:ring-teal-700/10', className)} {...props} />
}

type ModalProps = {
  open: boolean
  title: string
  children: ReactNode
  onClose?: () => void
  wide?: boolean
}
export function Modal({ open, title, children, onClose, wide }: ModalProps) {
  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-stone-950/35 p-0 backdrop-blur-[2px] sm:items-center sm:p-4" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose?.()}>
      <section role="dialog" aria-modal="true" aria-labelledby="modal-title" className={cn('max-h-[92svh] w-full overflow-y-auto rounded-t-3xl bg-[#fcfbf8] shadow-2xl sm:rounded-3xl', wide ? 'max-w-3xl' : 'max-w-lg')}>
        <header className="sticky top-0 z-10 flex items-center justify-between border-b border-stone-200/80 bg-[#fcfbf8]/95 px-5 py-4 backdrop-blur">
          <h2 id="modal-title" className="text-lg font-bold text-stone-900">{title}</h2>
          {onClose && <Button type="button" variant="ghost" className="size-10 p-0" onClick={onClose} aria-label="Закрыть"><X size={19} /></Button>}
        </header>
        <div className="p-5">{children}</div>
      </section>
    </div>
  )
}

export function ErrorNotice({ children }: { children: ReactNode }) {
  return <div role="alert" className="rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{children}</div>
}

export function LoadingBlock() {
  return <div className="flex min-h-32 items-center justify-center text-sm text-stone-500"><span className="mr-2 size-4 animate-spin rounded-full border-2 border-teal-700 border-t-transparent" />Загружаем…</div>
}

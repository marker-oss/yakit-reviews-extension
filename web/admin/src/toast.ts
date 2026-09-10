export type ToastKind = 'success' | 'error' | 'info'
export type ToastAction = { label: string; onClick: () => void }
export type Toast = {
  id: number
  kind: ToastKind
  message: string
  action?: ToastAction
  ttl: number
}

let seq = 0
let toasts: Toast[] = []
const listeners = new Set<(t: Toast[]) => void>()

function emit() {
  for (const l of listeners) l(toasts)
}

export function subscribe(listener: (t: Toast[]) => void): () => void {
  listeners.add(listener)
  listener(toasts)
  return () => listeners.delete(listener)
}

export function dismiss(id: number) {
  toasts = toasts.filter((t) => t.id !== id)
  emit()
}

function push(kind: ToastKind, message: string, action?: ToastAction) {
  const id = ++seq
  // Errors linger (8s) and success/info auto-clear faster (4s).
  const ttl = action ? 10000 : kind === 'error' ? 8000 : 4000
  toasts = [...toasts, { id, kind, message, action, ttl }]
  emit()
  window.setTimeout(() => dismiss(id), ttl)
  return id
}

export const toast = {
  success: (message: string, action?: ToastAction) => push('success', message, action),
  error: (message: string, action?: ToastAction) => push('error', message, action),
  info: (message: string, action?: ToastAction) => push('info', message, action),
}

import { writable } from 'svelte/store'

let nextToastId = 1

export const toasts = writable([])

/** kind: 'success' | 'error' | 'info' */
export function notify(message, kind = 'info', timeout = 5500) {
  const id = nextToastId++
  toasts.update((items) => [...items.slice(-4), { id, message: String(message), kind }])
  if (timeout > 0 && typeof setTimeout === 'function') {
    setTimeout(() => dismissToast(id), timeout)
  }
  return id
}

export function dismissToast(id) {
  toasts.update((items) => items.filter((item) => item.id !== id))
}

/** Estado global de la sesión para la capa visual. */
export const sessionState = writable({ loading: true, authenticated: false, email: '', setup: false, error: '' })

/** Colección que otra pantalla (p. ej. Resumen) pide abrir. Se consume al usarse. */
export const openCollection = writable('')

/** Señal para abrir el diálogo de nueva colección desde otra pantalla. */
export const createCollectionRequest = writable(0)

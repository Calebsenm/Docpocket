/**
 * Sesión del panel de administración.
 *
 * Los tokens se guardan en `sessionStorage` (no en `localStorage`) y se espejan
 * en memoria. Razones de seguridad:
 *  - `sessionStorage` está limitado a la pestaña y se descarta al cerrarla,
 *    mientras que `localStorage` persistentemente expone los tokens a cualquier
 *    script que se ejecute en el origen, incluso en visitas futuras.
 *  - El espejo en memoria evita leer el almacenamiento en cada petición y permite
 *    que el cliente de API funcione aunque el almacenamiento esté bloqueado.
 * Ningún token se escribe en la URL ni en `localStorage`; el tema de la interfaz
 * sí se guarda en `localStorage` porque no es información sensible.
 */

const TOKEN_KEY = 'dp.session.token'
const REFRESH_KEY = 'dp.session.refresh'
const EMAIL_KEY = 'dp.session.email'

let memory = { token: '', refresh: '', email: '' }
let hydrated = false

function storage() {
  try {
    return globalThis.sessionStorage ?? null
  } catch {
    // Algunos navegadores lanzan al acceder en modo privado o con cookies bloqueadas.
    return null
  }
}

function hydrate() {
  if (hydrated) return
  hydrated = true
  const s = storage()
  if (!s) return
  try {
    memory = {
      token: s.getItem(TOKEN_KEY) || '',
      refresh: s.getItem(REFRESH_KEY) || '',
      email: s.getItem(EMAIL_KEY) || '',
    }
  } catch {
    memory = { token: '', refresh: '', email: '' }
  }
}

export function getSession() {
  hydrate()
  return { ...memory }
}

export function hasSession() {
  hydrate()
  return Boolean(memory.token || memory.refresh)
}

export function saveSession(session = {}) {
  hydrate()
  const email = session.email || session?.record?.email || session?.identity?.email || memory.email
  memory = {
    token: session.token || '',
    refresh: session.refreshToken || session.refresh || '',
    email: email || '',
  }
  const s = storage()
  if (!s) return
  try {
    if (memory.token) s.setItem(TOKEN_KEY, memory.token)
    else s.removeItem(TOKEN_KEY)
    if (memory.refresh) s.setItem(REFRESH_KEY, memory.refresh)
    else s.removeItem(REFRESH_KEY)
    if (memory.email) s.setItem(EMAIL_KEY, memory.email)
    else s.removeItem(EMAIL_KEY)
  } catch {
    // El espejo en memoria sigue siendo válido durante esta pestaña.
  }
}

export function setSessionEmail(email) {
  saveSession({ ...getSession(), email })
}

export function clearSession() {
  memory = { token: '', refresh: '', email: '' }
  const s = storage()
  if (!s) return
  try {
    s.removeItem(TOKEN_KEY)
    s.removeItem(REFRESH_KEY)
    s.removeItem(EMAIL_KEY)
  } catch {
    // Nada más que hacer: la memoria ya está limpia.
  }
}

/** Reinicia el estado en pruebas (no se usa en la aplicación). */
export function __resetSessionForTests() {
  memory = { token: '', refresh: '', email: '' }
  hydrated = false
}

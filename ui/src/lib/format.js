/** Utilidades de formato y análisis de respuestas de texto. */

export function formatBytes(bytes) {
  if (bytes === null || bytes === undefined || bytes === '') return '—'
  const n = Number(bytes)
  if (!Number.isFinite(n)) return '—'
  if (n === 0) return '0 B'
  const negative = n < 0
  const abs = Math.abs(n)
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(units.length - 1, Math.floor(Math.log(abs) / Math.log(1024)))
  const value = abs / Math.pow(1024, i)
  const text = `${value.toFixed(value >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
  return negative ? `-${text}` : text
}

export function formatUptime(seconds) {
  if (seconds === null || seconds === undefined || seconds === '') return '—'
  const total = Math.max(0, Math.floor(Number(seconds) || 0))
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60
  if (days) return `${days} d ${hours} h`
  if (hours) return `${hours} h ${minutes} min`
  if (minutes) return `${minutes} min ${secs} s`
  return `${secs} s`
}

export function formatNumber(value) {
  if (value === null || value === undefined || value === '') return '—'
  const n = Number(value)
  if (!Number.isFinite(n)) return '—'
  return new Intl.NumberFormat('es-ES', { maximumFractionDigits: 2, useGrouping: 'always' }).format(n)
}

export function formatDate(value, { withTime = true } = {}) {
  if (!value) return '—'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('es-ES', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    ...(withTime ? { hour: '2-digit', minute: '2-digit', second: '2-digit' } : {}),
  }).format(date)
}

export function formatDuration(ms) {
  if (ms === null || ms === undefined || ms === '') return '—'
  const n = Number(ms)
  if (!Number.isFinite(n)) return '—'
  return n < 1000 ? `${Math.round(n)} ms` : `${(n / 1000).toFixed(2)} s`
}

export function truncate(value, max = 64) {
  const text = value === null || value === undefined ? '' : String(value)
  return text.length > max ? `${text.slice(0, Math.max(0, max - 1))}…` : text
}

/** Analiza métricas Prometheus simples (`nombre{etiqueta="..."} valor`). */
export function parsePrometheus(text) {
  const result = {}
  if (typeof text !== 'string') return result
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const parts = trimmed.split(/\s+/)
    if (parts.length < 2) continue
    const name = parts[0].replace(/\{.*\}$/, '')
    const value = Number(parts[parts.length - 1])
    if (name && Number.isFinite(value)) result[name] = value
  }
  return result
}

/** Resultado seguro de JSON.parse, sin lanzar excepciones. */
export function parseJson(text, fallback = undefined) {
  try {
    return { ok: true, value: JSON.parse(text) }
  } catch (error) {
    return { ok: false, value: fallback, error: error instanceof Error ? error.message : String(error) }
  }
}

export function isRecord(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

/**
 * Tokenizador para el resaltado seguro de CEL: devuelve fragmentos que la vista
 * renderiza como nodos de texto (nunca como HTML).
 */
export function tokenizeRule(source = '') {
  const tokens = []
  const re = /(\s+)|(&&|\|\||==|!=|>=|<=|[()[\]{},.:])|('[^']*'|"[^"]*")|(\b(?:true|false|null)\b)|(\b\d+(?:\.\d+)?\b)|([A-Za-z_][\w]*)/g
  let last = 0
  for (const match of String(source).matchAll(re)) {
    if (match.index > last) tokens.push({ text: source.slice(last, match.index), kind: 'plain' })
    const kind = match[1] ? 'space' : match[2] ? 'operator' : match[3] ? 'string' : match[4] ? 'literal' : match[5] ? 'number' : 'word'
    tokens.push({ text: match[0], kind })
    last = match.index + match[0].length
  }
  if (last < source.length) tokens.push({ text: source.slice(last), kind: 'plain' })
  return tokens
}

/** Lista de líneas para la numeración del editor de reglas. */
export function lineNumbers(source = '') {
  return Array.from({ length: Math.max(1, String(source).split('\n').length) }, (_, i) => i + 1)
}

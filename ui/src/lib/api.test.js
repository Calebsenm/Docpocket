import { beforeEach, describe, expect, it } from 'bun:test'
import {
  ApiError,
  __resetApiState,
  api,
  buildURL,
  setFetchImplementation,
  withCursor,
} from './api.js'
import { __resetSessionForTests, getSession, saveSession } from './session.js'

function jsonResponse(status, data, headers = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 401 ? 'Unauthorized' : 'OK',
    headers: {
      get(name) {
        return headers[name] || headers[name.toLowerCase()] || null
      },
    },
    async text() {
      return JSON.stringify(data)
    },
  }
}

beforeEach(() => {
  __resetApiState()
  __resetSessionForTests()
  setFetchImplementation(() => {
    throw new Error('fetch no configurado en esta prueba')
  })
})

describe('buildURL / withCursor', () => {
  it('conserva parámetros existentes y omite valores vacíos', () => {
    expect(buildURL('/api/records?view=all', { filter: 'a=1', empty: '', skip: null, n: 0 }))
      .toBe('/api/records?view=all&filter=a%3D1&n=0')
  })

  it('serializa arrays como parámetros repetidos', () => {
    expect(buildURL('/api/records', { fields: ['a', 'b'], sort: '-created' }))
      .toBe('/api/records?fields=a&fields=b&sort=-created')
  })

  it('añade el cursor solo cuando existe', () => {
    expect(withCursor('/api/records', { limit: 10 }, 'abc')).toBe('/api/records?limit=10&cursor=abc')
    expect(withCursor('/api/records', { limit: 10 }, '')).toBe('/api/records?limit=10')
  })
})

describe('api', () => {
  it('adjunta el Bearer del access token', async () => {
    saveSession({ token: 'access-1', refreshToken: 'refresh-1', email: 'admin@example.com' })
    let seen = null
    setFetchImplementation(async (url, init) => {
      seen = { url, init }
      return jsonResponse(200, { ok: true })
    })
    expect(await api('/api/admin/me')).toEqual({ ok: true })
    expect(seen.url).toBe('/api/admin/me')
    expect(seen.init.headers.Authorization).toBe('Bearer access-1')
  })

  it('mapea errores uniformes del servidor a ApiError', async () => {
    setFetchImplementation(async () => jsonResponse(422, {
      error: { code: 'validation_failed', message: 'Datos no válidos', details: { field: 'email' } },
    }, { 'X-Request-ID': 'req-42' }))

    try {
      await api('/api/admin/admins', { method: 'POST', body: { email: 'bad' } })
      throw new Error('la petición debería haber fallado')
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError)
      expect(error.status).toBe(422)
      expect(error.code).toBe('validation_failed')
      expect(error.message).toBe('Datos no válidos')
      expect(error.details).toEqual({ field: 'email' })
      expect(error.requestId).toBe('req-42')
    }
  })

  it(' USA una sola renovación para varias respuestas 401 concurrentes', async () => {
    saveSession({ token: 'old-token', refreshToken: 'refresh-token', email: 'admin@example.com' })
    const calls = []
    let refreshCalls = 0
    let protectedCallsCount = 0

    setFetchImplementation(async (url, init) => {
      calls.push({ url, token: init.headers.Authorization })
      if (url === '/api/admin/auth-refresh') {
        refreshCalls++
        await new Promise((resolve) => setTimeout(resolve, 15))
        return jsonResponse(200, { token: 'new-token', refreshToken: 'refresh-token-2' })
      }
      protectedCallsCount++
      if (protectedCallsCount <= 3) {
        return jsonResponse(401, { error: { code: 'unauthorized', message: 'Token caducado' } })
      }
      return jsonResponse(200, { ok: true })
    })

    const results = await Promise.all([
      api('/api/protected'),
      api('/api/protected'),
      api('/api/protected'),
    ])

    expect(refreshCalls).toBe(1)
    expect(results.every((r) => r.ok === true)).toBe(true)
    const protectedCalls = calls.filter((c) => c.url === '/api/protected')
    expect(protectedCalls).toHaveLength(6)
    expect(protectedCalls.slice(0, 3).every((c) => c.token === 'Bearer old-token')).toBe(true)
    expect(protectedCalls.slice(3).every((c) => c.token === 'Bearer new-token')).toBe(true)
    expect(getSession().token).toBe('new-token')
  })

  it('aborta la petición anterior con la misma clave', async () => {
    setFetchImplementation(async (_url, init) => {
      if (init.signal.aborted) throw new DOMException('Abortada', 'AbortError')
      await new Promise((resolve, reject) => {
        const timer = setTimeout(resolve, 30)
        init.signal.addEventListener('abort', () => {
          clearTimeout(timer)
          reject(new DOMException('Abortada', 'AbortError'))
        }, { once: true })
      })
      return jsonResponse(200, { ok: true })
    })

    const first = api('/api/search?q=uno', { key: 'search' })
    const second = api('/api/search?q=dos', { key: 'search' })
    await expect(first).rejects.toThrow()
    expect(await second).toEqual({ ok: true })
  })
})

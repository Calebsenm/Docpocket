<script>
  import { onMount } from 'svelte'
  import { api, apiFetch, parseApiResponse, serverLogout } from './lib/api.js'
  import { getSession, hasSession, saveSession, setSessionEmail } from './lib/session.js'
  import { notify, sessionState } from './lib/stores.js'
  import Icon from './lib/components/Icon.svelte'
  import StateBlock from './lib/components/StateBlock.svelte'
  import Toasts from './lib/components/Toasts.svelte'
  import Login from './screens/Login.svelte'
  import Overview from './screens/Overview.svelte'
  import Collections from './screens/Collections.svelte'
  import Users from './screens/Users.svelte'
  import Files from './screens/Files.svelte'
  import Rules from './screens/Rules.svelte'
  import Realtime from './screens/Realtime.svelte'
  import Logs from './screens/Logs.svelte'
  import Maintenance from './screens/Maintenance.svelte'
  import Settings from './screens/Settings.svelte'
  import Admins from './screens/Admins.svelte'
  import Sessions from './screens/Sessions.svelte'

  const sections = [
    { id: 'overview', label: 'Resumen', icon: 'grid', group: 'Panel' },
    { id: 'collections', label: 'Colecciones', icon: 'database', group: 'Panel' },
    { id: 'users', label: 'Usuarios', icon: 'users', group: 'Panel' },
    { id: 'files', label: 'Archivos', icon: 'folder', group: 'Panel' },
    { id: 'rules', label: 'Reglas CEL', icon: 'shield', group: 'Panel' },
    { id: 'realtime', label: 'Tiempo real', icon: 'bolt', group: 'Panel' },
    { id: 'logs', label: 'Registro', icon: 'list', group: 'Sistema' },
    { id: 'maintenance', label: 'Mantenimiento', icon: 'wrench', group: 'Sistema' },
    { id: 'settings', label: 'Ajustes', icon: 'settings', group: 'Sistema' },
    { id: 'admins', label: 'Superusuarios', icon: 'key', group: 'Sistema' },
    { id: 'sessions', label: 'Sesiones', icon: 'activity', group: 'Sistema' },
  ]
  const groups = ['Panel', 'Sistema']

  let booting = $state(true)
  let fatalError = $state('')
  let configured = $state(false)
  let authenticated = $state(false)
  let email = $state('')
  let active = $state('overview')
  let theme = $state('auto')
  let health = $state(null)
  let healthError = $state(false)
  let loggingOut = $state(false)
  let healthTimer = null

  const statusLabel = $derived(
    healthError ? 'Sin conexión' : health?.status === 'ok' ? 'En línea' : 'Comprobando…'
  )
  const statusClass = $derived(healthError ? 'is-error' : health?.status === 'ok' ? 'is-ok' : 'is-warn')

  function delay(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms))
  }

  async function retryNetwork(fn, attempts = 3) {
    let last
    for (let i = 0; i < attempts; i += 1) {
      try {
        return await fn()
      } catch (error) {
        last = error
        const transient = !error?.status || error.status >= 500 || error.code === 'network_error' || error.code === 'timeout'
        if (!transient || i === attempts - 1) throw error
        await delay(400 * (i + 1))
      }
    }
    throw last
  }

  async function checkSetup() {
    const res = await retryNetwork(() => apiFetch('/api/admin/setup', { auth: false, timeoutMs: 8000 }))
    const data = await parseApiResponse(res)
    return Boolean(data?.configured)
  }

  async function boot() {
    booting = true
    fatalError = ''
    try {
      configured = await checkSetup()
      if (configured && hasSession()) {
        try {
          const me = await retryNetwork(() => api('/api/admin/me', { key: 'boot-me' }), 2)
          email = me?.email || getSession().email || 'superusuario'
          setSessionEmail(email)
          authenticated = true
        } catch (error) {
          if (error?.status === 401) {
            // Sesión definitively rechazada: sí se puede descartar.
            configured = await checkSetup().catch(() => false)
          } else {
            // Fallo de red: NUNCA borramos la sesión; mostramos estado de error.
            fatalError = error instanceof Error ? error.message : String(error)
          }
        }
      }
      await refreshHealth()
    } catch (error) {
      // No se pudo ni siquiera consultar /api/admin/setup: conservar la sesión.
      fatalError = error instanceof Error ? error.message : String(error)
    } finally {
      booting = false
      sessionState.set({ loading: false, authenticated, email, setup: !configured, error: fatalError })
    }
  }

  async function refreshHealth() {
    try {
      health = await apiFetch('/api/health', { auth: false, timeoutMs: 6000, key: 'header-health' })
        .then(parseApiResponse)
      healthError = false
    } catch {
      healthError = true
    }
  }

  async function onAuthenticated(session) {
    saveSession(session)
    const me = await api('/api/admin/me', { key: 'boot-me' })
    email = me?.email || session?.record?.email || 'superusuario'
    setSessionEmail(email)
    authenticated = true
    configured = true
    fatalError = ''
    notify('Sesión iniciada correctamente', 'success')
    await refreshHealth()
  }

  async function logout() {
    loggingOut = true
    // El servidor se invalida ANTES de limpiar la sesión local.
    const result = await serverLogout()
    authenticated = false
    email = ''
    active = 'overview'
    loggingOut = false
    notify(result.remote ? 'Sesión cerrada' : 'Sesión cerrada localmente; no se pudo confirmar en el servidor', result.remote ? 'success' : 'error')
  }

  function applyTheme(value) {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.theme = value
  }

  function cycleTheme() {
    const order = ['auto', 'light', 'dark']
    theme = order[(order.indexOf(theme) + 1) % order.length]
    try {
      localStorage.setItem('dp.theme', theme)
    } catch {
      // Sin persistencia, el tema sigue aplicado durante la sesión.
    }
    applyTheme(theme)
  }

  function handleSessionExpired() {
    authenticated = false
    email = ''
    notify('La sesión ha caducado. Vuelve a iniciar sesión.', 'error')
  }

  onMount(() => {
    try {
      theme = localStorage.getItem('dp.theme') || 'auto'
    } catch {
      theme = 'auto'
    }
    applyTheme(theme)
    globalThis.addEventListener?.('dp:session-expired', handleSessionExpired)
    boot()
    healthTimer = setInterval(refreshHealth, 30000)
    return () => {
      if (healthTimer) clearInterval(healthTimer)
      globalThis.removeEventListener?.('dp:session-expired', handleSessionExpired)
    }
  })
</script>

<div class="app-shell">
  <header class="app-header">
    <div class="brand">
      <span class="brand-mark" aria-hidden="true">DP</span>
      <span class="brand-name">DocPocket</span>
      <span class="brand-tag">ADMIN</span>
    </div>
    <div class="header-actions">
      {#if authenticated}
        <div class="header-status" title="Estado real del servidor">
          <span class="status-dot {statusClass}" aria-hidden="true"></span>
          <span>{statusLabel}{health?.version ? ` · v${health.version}` : ''}</span>
        </div>
      {/if}
      <button type="button" class="icon-button" onclick={cycleTheme} aria-label="Cambiar tema (auto, claro, oscuro)" title="Tema: {theme}">
        <Icon name={theme === 'dark' ? 'moon' : theme === 'light' ? 'sun' : 'activity'} size={16} />
      </button>
      {#if authenticated}
        <div class="user-chip" title={email}>
          <Icon name="users" size={13} />
          <span class="truncate">{email}</span>
        </div>
        <button type="button" class="btn btn-sm btn-ghost" onclick={logout} disabled={loggingOut}>
          {#if loggingOut}<span class="spinner" aria-hidden="true"></span>{/if}
          <Icon name="logout" size={15} />
          <span>Salir</span>
        </button>
      {/if}
    </div>
  </header>

  {#if !booting && fatalError}
    <main class="app-main">
      <div class="page card">
        <StateBlock
          kind="error"
          title="No se pudo comprobar el estado del servidor"
          detail={`${fatalError}. La sesión local se ha conservado; vuelve a intentarlo.`}
          onRetry={boot}
        />
      </div>
    </main>
  {:else if booting}
    <main class="app-main">
      <div class="page card">
        <StateBlock kind="loading" title="Conectando con DocPocket…" />
      </div>
    </main>
  {:else if !authenticated}
    <Login setup={!configured} onSuccess={onAuthenticated} />
  {:else}
    <nav class="mobile-tabs" aria-label="Secciones del panel">
      {#each sections as section (section.id)}
        <button
          type="button"
          class="tab"
          class:active={active === section.id}
          aria-current={active === section.id ? 'page' : undefined}
          onclick={() => (active = section.id)}
        >
          {section.label}
        </button>
      {/each}
    </nav>
    <div class="app-body">
      <aside class="app-sidebar">
        {#each groups as group (group)}
          <p class="side-label">{group}</p>
          {#each sections.filter((s) => s.group === group) as section (section.id)}
            <button
              type="button"
              class="nav-item"
              class:active={active === section.id}
              aria-current={active === section.id ? 'page' : undefined}
              onclick={() => (active = section.id)}
            >
              <Icon name={section.icon} size={16} />
              <span>{section.label}</span>
            </button>
          {/each}
        {/each}
        <div class="side-footer">Panel de administración<br />DocPocket · un único binario</div>
      </aside>

      <main class="app-main">
        <div class="page">
          {#if active === 'overview'}<Overview />
          {:else if active === 'collections'}<Collections />
          {:else if active === 'users'}<Users />
          {:else if active === 'files'}<Files />
          {:else if active === 'rules'}<Rules />
          {:else if active === 'realtime'}<Realtime />
          {:else if active === 'logs'}<Logs />
          {:else if active === 'maintenance'}<Maintenance />
          {:else if active === 'settings'}<Settings />
          {:else if active === 'admins'}<Admins />
          {:else if active === 'sessions'}<Sessions />
          {/if}
        </div>
      </main>
    </div>
  {/if}
</div>

<Toasts />

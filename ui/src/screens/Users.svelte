<script>
  import { onMount } from 'svelte'
  import { api, buildURL } from '../lib/api.js'
  import { formatDate, formatNumber, truncate } from '../lib/format.js'
  import { notify } from '../lib/stores.js'
  import Icon from '../lib/components/Icon.svelte'
  import StateBlock from '../lib/components/StateBlock.svelte'
  import Modal from '../lib/components/Modal.svelte'
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte'

  let collections = $state([])
  let selectedCollection = $state('')
  let users = $state([])
  let loading = $state(false)
  let error = $state('')
  let search = $state('')
  let userModalOpen = $state(false)
  let mode = $state('create')
  let target = $state(null)
  let form = $state({ email: '', password: '', name: '' })
  let saving = $state(false)
  let deleteOpen = $state(false)
  let sessionsOpen = $state(false)
  let sessions = $state([])
  let sessionsLoading = $state(false)
  let sessionsError = $state('')

  const authCollections = $derived(collections.filter((c) => c.type === 'auth'))
  const visibleUsers = $derived(
    users.filter((user) => `${user.email || ''} ${user.name || ''} ${user.id || ''}`.toLowerCase().includes(search.trim().toLowerCase()))
  )

  async function loadCollections() {
    try {
      const data = await api('/api/admin/collections', { key: 'users-collections' })
      collections = data?.items || []
      if (!selectedCollection && authCollections.length) selectedCollection = authCollections[0].name
      if (selectedCollection) await loadUsers()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    }
  }

  async function loadUsers() {
    if (!selectedCollection) return
    loading = true
    error = ''
    try {
      const data = await api(buildURL(`/api/admin/collections/${encodeURIComponent(selectedCollection)}/records`, { limit: 500 }), { key: `users-${selectedCollection}` })
      users = data?.items || []
    } catch (err) {
      users = []
      error = err instanceof Error ? err.message : String(err)
    } finally {
      loading = false
    }
  }

  function openCreate() {
    mode = 'create'
    target = null
    form = { email: '', password: '', name: '' }
    userModalOpen = true
  }

  function openPassword(user) {
    mode = 'password'
    target = user
    form = { email: user.email || '', password: '', name: user.name || '' }
    userModalOpen = true
  }

  async function saveUser() {
    if (form.password.length < 8) {
      notify('La contraseña debe tener al menos 8 caracteres', 'error')
      return
    }
    saving = true
    try {
      if (mode === 'create') {
        const body = { email: form.email.trim(), password: form.password }
        if (form.name.trim()) body.name = form.name.trim()
        await api(`/api/collections/${encodeURIComponent(selectedCollection)}/auth-register`, { method: 'POST', body })
        notify('Usuario creado', 'success')
      } else {
        await api(`/api/collections/${encodeURIComponent(selectedCollection)}/records/${encodeURIComponent(target.id)}/password`, {
          method: 'PATCH',
          body: { password: form.password },
        })
        notify('Contraseña actualizada; las sesiones del usuario se han invalidado', 'success')
      }
      userModalOpen = false
      await loadUsers()
    } catch (err) {
      notify(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      saving = false
    }
  }

  async function deleteUser() {
    await api(`/api/collections/${encodeURIComponent(selectedCollection)}/records/${encodeURIComponent(target.id)}`, { method: 'DELETE' })
    notify('Usuario eliminado', 'success')
    await loadUsers()
  }

  async function loadSessions(user) {
    target = user
    sessions = []
    sessionsError = ''
    sessionsLoading = true
    sessionsOpen = true
    try {
      const data = await api(buildURL('/api/admin/sessions', { kind: 'user', limit: 500 }), { key: `user-sessions-${user.id}` })
      sessions = (data?.items || []).filter((s) => s.userId === user.id)
    } catch (err) {
      sessionsError = err instanceof Error ? err.message : String(err)
    } finally {
      sessionsLoading = false
    }
  }

  async function revokeUserSessions() {
    try {
      const result = await api('/api/admin/sessions/revoke', {
        method: 'POST',
        body: { identity: { kind: 'user', collection: selectedCollection, id: target.id } },
      })
      notify(`Sesiones revocadas: ${result?.revoked ?? '—'}`, 'success')
      await loadSessions(target)
    } catch (err) {
      notify(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  onMount(loadCollections)
</script>

<div class="stack">
  <div class="page-head">
    <div class="page-title">
      <h1>Usuarios y autenticación</h1>
      <p class="page-subtitle">Cuentas de las colecciones de tipo <code>auth</code>.</p>
    </div>
    <div class="page-actions">
      <select class="select" bind:value={selectedCollection} onchange={loadUsers} aria-label="Colección de autenticación">
        {#each authCollections as col (col.name)}<option value={col.name}>{col.name}</option>{/each}
        {#if !authCollections.length}<option value="">No hay colecciones auth</option>{/if}
      </select>
      <button type="button" class="btn btn-primary btn-sm" onclick={openCreate} disabled={!selectedCollection}>
        <Icon name="plus" size={14} /> Crear usuario
      </button>
    </div>
  </div>

  {#if !authCollections.length}
    <StateBlock kind="empty" title="Sin colecciones de autenticación" detail="Crea una colección de tipo auth desde Colecciones para gestionar usuarios." />
  {:else if error}
    <StateBlock kind="error" title="No se pudieron cargar los usuarios" detail={error} onRetry={loadUsers} />
  {:else}
    <section class="card">
      <div class="card-head">
        <div class="card-title-group">
          <h2>{formatNumber(users.length)} usuarios en <code>{selectedCollection}</code></h2>
          <p class="muted tiny">Los datos se leen del endpoint de administración de registros.</p>
        </div>
        <div class="search-input">
          <Icon name="search" size={14} />
          <input class="input" type="search" bind:value={search} placeholder="Buscar usuario…" aria-label="Buscar usuario" />
        </div>
      </div>
      <div class="card-body">
        {#if loading}
          <StateBlock kind="loading" title="Cargando usuarios…" />
        {:else if !visibleUsers.length}
          <StateBlock kind="empty" title="Sin usuarios" detail="Todavía no hay cuentas en esta colección." />
        {:else}
          <div class="table-wrap">
            <table>
              <thead><tr><th>ID</th><th>Email</th><th>Nombre</th><th>Verificado</th><th>Creado</th><th>Sesiones</th><th>Acciones</th></tr></thead>
              <tbody>
                {#each visibleUsers as user (user.id)}
                  <tr>
                    <td class="id-pill" title={user.id}>{truncate(user.id, 12)}</td>
                    <td>{user.email || '—'}</td>
                    <td>{user.name || '—'}</td>
                    <td>
                      <span class="badge {user.verified ? 'badge-ok' : 'badge-warn'}">{user.verified ? 'Sí' : 'No'}</span>
                    </td>
                    <td class="nowrap tiny muted">{formatDate(user.created)}</td>
                    <td><button type="button" class="btn btn-xs" onclick={() => loadSessions(user)}>Ver</button></td>
                    <td>
                      <div class="row-actions">
                        <button type="button" class="btn btn-xs" onclick={() => openPassword(user)}>Contraseña</button>
                        <button type="button" class="btn btn-xs btn-danger" onclick={() => { target = user; deleteOpen = true }}>Borrar</button>
                      </div>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</div>

<Modal bind:open={userModalOpen} title={mode === 'create' ? 'Nuevo usuario' : 'Cambiar contraseña'} subtitle={mode === 'create' ? selectedCollection : target?.email || target?.id}>
  <form class="form-stack" onsubmit={(e) => { e.preventDefault(); saveUser() }}>
    {#if mode === 'create'}
      <div class="field"><label class="field-label" for="user-email">Correo electrónico</label><input id="user-email" class="input" type="email" bind:value={form.email} required /></div>
      <div class="field"><label class="field-label" for="user-name">Nombre (opcional)</label><input id="user-name" class="input" type="text" bind:value={form.name} /></div>
    {/if}
    <div class="field">
      <label class="field-label" for="user-password">{mode === 'create' ? 'Contraseña' : 'Nueva contraseña'}</label>
      <input id="user-password" class="input" type="password" minlength="8" bind:value={form.password} required />
      <span class="field-hint">Mínimo 8 caracteres. Como superusuario no necesitas la contraseña actual.</span>
    </div>
    <div class="row">
      <button type="button" class="btn" onclick={() => (userModalOpen = false)}>Cancelar</button>
      <button type="submit" class="btn btn-primary" disabled={saving}>
        {#if saving}<span class="spinner" aria-hidden="true"></span>{/if}
        {mode === 'create' ? 'Crear usuario' : 'Actualizar contraseña'}
      </button>
    </div>
  </form>
</Modal>

<ConfirmDialog
  bind:open={deleteOpen}
  title="Eliminar usuario"
  message={`¿Eliminar al usuario ${target?.email || target?.id || ''}? Se borrará su registro y sus archivos asociados.`}
  confirmLabel="Eliminar usuario"
  danger={true}
  onConfirm={deleteUser}
/>

<Modal bind:open={sessionsOpen} title="Sesiones del usuario" subtitle={target?.email || target?.id} wide>
  <div class="stack">
    <div class="callout callout-info">
      <span aria-hidden="true">ℹ</span>
      <span>Sesiones activas e históricas devueltas por /api/admin/sessions filtradas por este usuario.</span>
    </div>
    {#if sessionsLoading}
      <StateBlock kind="loading" title="Cargando sesiones…" />
    {:else if sessionsError}
      <StateBlock kind="error" title="No se pudieron cargar las sesiones" detail={sessionsError} />
    {:else if !sessions.length}
      <StateBlock kind="empty" title="Sin sesiones" detail="Este usuario no tiene sesiones registradas." />
    {:else}
      <div class="table-wrap">
        <table>
          <thead><tr><th>Token</th><th>Familia</th><th>Colección</th><th>Creada</th><th>Expira</th><th>Uso</th></tr></thead>
          <tbody>
            {#each sessions as session (session.tokenId)}
              <tr>
                <td class="mono tiny">{truncate(session.tokenId, 12)}</td>
                <td class="mono tiny">{truncate(session.family, 10)}</td>
                <td class="mono tiny">{session.collection || '—'}</td>
                <td class="tiny nowrap">{formatDate(session.createdAt)}</td>
                <td class="tiny nowrap">{formatDate(session.expiresAt)}</td>
                <td>{session.used ? 'Usada' : 'Activa'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
    <div class="row">
      <button type="button" class="btn btn-danger btn-sm" onclick={revokeUserSessions} disabled={!sessions.length}>
        Revocar todas las sesiones de este usuario
      </button>
    </div>
  </div>
</Modal>

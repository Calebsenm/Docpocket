<script>
  import { api, errorMessage } from '../lib/api.js';
  import { formatDate, truncate } from '../lib/format.js';
  import { notify } from '../lib/stores.js';
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte';
  import Modal from '../lib/components/Modal.svelte';
  import StateBlock from '../lib/components/StateBlock.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let items = $state([]);
  let loading = $state(true);
  let error = $state('');
  let loaded = $state(false);
  let showCreate = $state(false);
  let saving = $state(false);
  let form = $state({ email: '', password: '' });
  let validation = $state('');
  let pendingDelete = $state(null);
  let deleting = $state(false);

  async function load() {
    loading = true;
    error = '';
    try {
      const data = await api('/api/admin/admins', { key: 'admin-admins' });
      items = Array.isArray(data?.items) ? data.items : [];
      loaded = true;
    } catch (err) {
      error = errorMessage(err);
      loaded = true;
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    form = { email: '', password: '' };
    validation = '';
    showCreate = true;
  }

  async function create(event) {
    event?.preventDefault?.();
    const email = form.email.trim();
    if (!email || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      validation = 'Introduce un correo electrónico válido.';
      return;
    }
    if (form.password.length < 8) {
      validation = 'La contraseña debe tener al menos 8 caracteres.';
      return;
    }
    saving = true;
    validation = '';
    try {
      await api('/api/admin/admins', { method: 'POST', body: { email, password: form.password } });
      notify('Superusuario creado.', 'success');
      showCreate = false;
      await load();
    } catch (err) {
      notify(errorMessage(err), 'error');
    } finally {
      saving = false;
    }
  }

  async function remove() {
    if (!pendingDelete) return;
    deleting = true;
    try {
      await api(`/api/admin/admins/${encodeURIComponent(pendingDelete.id)}`, { method: 'DELETE' });
      notify('Superusuario eliminado.', 'success');
      pendingDelete = null;
      await load();
    } catch (err) {
      notify(errorMessage(err), 'error');
      pendingDelete = null;
    } finally {
      deleting = false;
    }
  }

  $effect(() => { load(); });
</script>

<section class="page">
  <header class="page-head">
    <div>
      <h1>Superusuarios</h1>
      <p class="subtitle">Cuentas con acceso completo al panel de administración.</p>
    </div>
    <button class="btn primary" onclick={openCreate}>
      <Icon name="plus" size={15} /> Nuevo superusuario
    </button>
  </header>

  {#if error}
    <StateBlock kind="error" title="No se pudieron cargar los superusuarios" detail={error} onRetry={load} />
  {:else if loading && !loaded}
    <StateBlock kind="loading" title="Cargando superusuarios…" />
  {:else if items.length === 0}
    <StateBlock kind="empty" title="Sin superusuarios" detail="Todavía no hay cuentas de administrador." />
  {:else}
    <div class="panel table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th>Correo</th>
            <th>ID</th>
            <th>Creado</th>
            <th>Actualizado</th>
            <th class="num">Acciones</th>
          </tr>
        </thead>
        <tbody>
          {#each items as admin}
            <tr>
              <td class="email">{admin.email || '—'}</td>
              <td class="mono muted" title={admin.id || ''}>{truncate(admin.id || '—', 16)}</td>
              <td class="nowrap">{formatDate(admin.created)}</td>
              <td class="nowrap">{formatDate(admin.updated)}</td>
              <td class="num">
                <button
                  class="btn ghost small"
                  onclick={() => (pendingDelete = admin)}
                  title={`Eliminar a ${admin.email || 'este superusuario'}`}
                >
                  <Icon name="trash" size={14} /> Eliminar
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<Modal open={showCreate} title="Nuevo superusuario" onClose={() => (showCreate = false)}>
  <form class="create-form" onsubmit={create}>
    <label class="field">
      <span>Correo electrónico</span>
      <input type="email" autocomplete="username" bind:value={form.email} required />
    </label>
    <label class="field">
      <span>Contraseña</span>
      <input type="password" autocomplete="new-password" minlength="8" bind:value={form.password} required />
      <small>Mínimo 8 caracteres.</small>
    </label>
    {#if validation}<p class="validation" role="alert">{validation}</p>{/if}
    <div class="form-actions">
      <button class="btn primary" type="submit" disabled={saving}>{saving ? 'Creando…' : 'Crear'}</button>
      <button class="btn ghost" type="button" onclick={() => (showCreate = false)} disabled={saving}>Cancelar</button>
    </div>
  </form>
</Modal>

<ConfirmDialog
  open={!!pendingDelete}
  title="Eliminar superusuario"
  message={`¿Eliminar a «${pendingDelete?.email || 'este usuario'}»? Perderá el acceso al panel de forma inmediata.`}
  confirmLabel="Eliminar"
  danger={true}
  requireText={pendingDelete?.email || ''}
  onCancel={() => (pendingDelete = null)}
  onConfirm={remove}
/>

<style>
  .table-wrap { overflow-x: auto; }
  .data-table { width: 100%; border-collapse: collapse; font-size: 0.86rem; }
  .data-table th {
    text-align: left;
    padding: var(--space-sm) var(--space-md);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }
  .data-table td { padding: 0.55rem var(--space-md); border-bottom: 1px solid var(--border-subtle); }
  .data-table tbody tr:last-child td { border-bottom: 0; }
  .num { text-align: right; }
  .nowrap { white-space: nowrap; }
  .email { font-weight: 500; }
  .muted { color: var(--text-muted); }
  .small { padding: 0.25rem 0.5rem; }
  .create-form { display: grid; gap: var(--space-md); }
  .validation { color: var(--danger); font-size: 0.85rem; margin: 0; }
  .form-actions { display: flex; gap: var(--space-sm); }
</style>

<script>
  import { api, errorMessage } from '../lib/api.js';
  import { formatDate, truncate } from '../lib/format.js';
  import { notify } from '../lib/stores.js';
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte';
  import StateBlock from '../lib/components/StateBlock.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let items = $state([]);
  let loading = $state(true);
  let error = $state('');
  let loaded = $state(false);
  let filters = $state({ kind: '', limit: '100' });
  let pending = $state(null);
  let busy = $state(false);

  async function load() {
    loading = true;
    error = '';
    try {
      const params = new URLSearchParams();
      if (filters.kind) params.set('kind', filters.kind);
      if (filters.limit) params.set('limit', filters.limit);
      const qs = params.size ? `?${params}` : '';
      const data = await api(`/api/admin/sessions${qs}`, { key: 'admin-sessions' });
      items = Array.isArray(data?.items) ? data.items : [];
      loaded = true;
    } catch (err) {
      error = errorMessage(err);
      loaded = true;
    } finally {
      loading = false;
    }
  }

  function revokeFamily(row) {
    pending = { label: `la familia «${row.family || '—'}»`, body: { family: row.family }, require: row.family || '' };
  }

  function revokeIdentity(row) {
    const identity = {
      kind: row.kind || 'admin',
      collection: row.collection || '',
      id: row.userId || '',
    };
    pending = {
      label: `todas las sesiones de ${identity.kind === 'admin' ? 'este superusuario' : 'este usuario'}`,
      body: { identity },
      require: '',
    };
  }

  function revokeAll() {
    pending = { label: 'todas las sesiones activas', body: { all: true }, require: 'TODO' };
  }

  async function confirmRevoke() {
    if (!pending) return;
    busy = true;
    try {
      const data = await api('/api/admin/sessions/revoke', { method: 'POST', body: pending.body });
      notify(`Sesiones revocadas: ${data?.revoked ?? 0}.`, 'success');
      pending = null;
      await load();
    } catch (err) {
      notify(errorMessage(err), 'error');
      pending = null;
    } finally {
      busy = false;
    }
  }

  function subject(row) {
    if (row.kind === 'admin') return 'Superusuario';
    return [row.collection, row.userId].filter(Boolean).join(' / ') || 'Usuario';
  }

  $effect(() => { load(); });
</script>

<section class="page">
  <header class="page-head">
    <div>
      <h1>Sesiones</h1>
      <p class="subtitle">Sesiones de actualización activas y revocación de tokens.</p>
    </div>
    <button class="btn danger" onclick={revokeAll} disabled={loading}>
      <Icon name="trash" size={15} /> Revocar todas
    </button>
  </header>

  <div class="panel filters">
    <label class="field">
      <span>Tipo</span>
      <select bind:value={filters.kind} onchange={load}>
        <option value="">Todas</option>
        <option value="admin">Superusuarios</option>
        <option value="user">Usuarios</option>
      </select>
    </label>
    <label class="field narrow">
      <span>Límite</span>
      <select bind:value={filters.limit} onchange={load}>
        <option value="50">50</option>
        <option value="100">100</option>
        <option value="500">500</option>
      </select>
    </label>
  </div>

  {#if error}
    <StateBlock kind="error" message={error}>
      <button class="btn" onclick={load}>Reintentar</button>
    </StateBlock>
  {:else if loading && !loaded}
    <StateBlock kind="loading" message="Cargando sesiones…" />
  {:else if items.length === 0}
    <StateBlock kind="empty" title="Sin sesiones" message="No hay sesiones activas que coincidan con los filtros." />
  {:else}
    <div class="panel table-wrap">
      <table class="data-table">
        <thead>
          <tr>
            <th>Tipo</th>
            <th>Identidad</th>
            <th>Familia</th>
            <th>Creada</th>
            <th>Expira</th>
            <th>Estado</th>
            <th class="num">Acciones</th>
          </tr>
        </thead>
        <tbody>
          {#each items as row}
            <tr>
              <td><span class="badge">{row.kind === 'admin' ? 'Admin' : 'Usuario'}</span></td>
              <td class="mono" title={subject(row)}>{truncate(subject(row), 32)}</td>
              <td class="mono muted" title={row.family || ''}>{truncate(row.family || '—', 14)}</td>
              <td class="nowrap">{formatDate(row.createdAt)}</td>
              <td class="nowrap">{formatDate(row.expiresAt)}</td>
              <td>
                <span class:used={row.used} class="state">{row.used ? 'Usada' : 'Activa'}</span>
              </td>
              <td class="num actions-cell">
                <button class="btn ghost small" onclick={() => revokeFamily(row)} disabled={!row.family} title="Revocar esta familia de sesiones">
                  Familia
                </button>
                <button class="btn ghost small" onclick={() => revokeIdentity(row)} title="Revocar todas las sesiones de esta identidad">
                  Identidad
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<ConfirmDialog
  open={!!pending}
  title="Revocar sesiones"
  message={`Se revocarán ${pending?.label || 'las sesiones seleccionadas'}. Los afectados deberán iniciar sesión de nuevo.`}
  confirmLabel="Revocar"
  danger={true}
  requireText={pending?.require || ''}
  onCancel={() => (pending = null)}
  onConfirm={confirmRevoke}
/>

<style>
  .filters { display: flex; gap: var(--space-md); padding: var(--space-md); }
  .narrow { max-width: 140px; }
  .table-wrap { overflow-x: auto; margin-top: var(--space-lg); }
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
  .muted { color: var(--text-muted); }
  .badge {
    display: inline-block;
    padding: 0.1rem 0.45rem;
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    font-size: 0.78rem;
  }
  .state { color: var(--success); font-size: 0.8rem; }
  .state.used { color: var(--text-muted); }
  .actions-cell { white-space: nowrap; }
  .small { padding: 0.25rem 0.5rem; margin-left: 0.25rem; }
</style>

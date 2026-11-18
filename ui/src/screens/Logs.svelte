<script>
  import { onMount } from 'svelte';
  import { api, errorMessage } from '../lib/api.js';
  import { formatDate, formatNumber, formatDuration, truncate } from '../lib/format.js';
  import StateBlock from '../lib/components/StateBlock.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let items = $state([]);
  let total = $state(0);
  let truncated = $state(false);
  let loading = $state(true);
  let error = $state('');
  let loaded = $state(false);
  let filters = $state({ q: '', status: '', limit: '200' });
  let debounce;

  function statusTone(status) {
    if (status >= 500) return 'danger';
    if (status >= 400) return 'warning';
    return 'success';
  }

  async function load() {
    loading = true;
    error = '';
    const key = 'admin-logs';
    try {
      const params = new URLSearchParams();
      if (filters.q.trim()) params.set('q', filters.q.trim());
      if (filters.status) params.set('status', filters.status);
      if (filters.limit) params.set('limit', filters.limit);
      const qs = params.size ? `?${params}` : '';
      const data = await api(`/api/admin/logs${qs}`, { key });
      items = Array.isArray(data?.items) ? data.items : [];
      total = Number(data?.total) || items.length;
      truncated = Boolean(data?.truncated);
      loaded = true;
    } catch (err) {
      if (err?.name === 'AbortError') return;
      error = errorMessage(err);
      loaded = true;
    } finally {
      loading = false;
    }
  }

  function onFilterInput() {
    clearTimeout(debounce);
    debounce = setTimeout(load, 300);
  }

  onMount(load);
</script>

<section class="page">
  <header class="page-head">
    <div>
      <h1>Registro</h1>
      <p class="subtitle">Solicitudes recientes registradas por el servidor.</p>
    </div>
    <div class="head-actions">
      <button class="btn ghost" onclick={load} disabled={loading} title="Actualizar registro">
        <Icon name="refresh" size={15} /> Actualizar
      </button>
    </div>
  </header>

  <div class="panel">
    <div class="filters">
      <label class="field search-field">
        <span>Buscar</span>
        <input
          type="search"
          placeholder="Ruta, IP, método…"
          bind:value={filters.q}
          oninput={onFilterInput}
        />
      </label>
      <label class="field">
        <span>Estado</span>
        <select bind:value={filters.status} onchange={load}>
          <option value="">Todos</option>
          <option value="200">200 — correcto</option>
          <option value="400">400 — solicitud incorrecta</option>
          <option value="401">401 — no autorizado</option>
          <option value="403">403 — prohibido</option>
          <option value="404">404 — no encontrado</option>
          <option value="500">500 — error del servidor</option>
        </select>
      </label>
      <label class="field narrow">
        <span>Límite</span>
        <select bind:value={filters.limit} onchange={load}>
          <option value="50">50</option>
          <option value="100">100</option>
          <option value="200">200</option>
          <option value="500">500</option>
        </select>
      </label>
    </div>
    <div class="log-meta muted">
      {formatNumber(total)} entradas
      {#if truncated}· resultado truncado{/if}
    </div>
  </div>

  {#if error}
    <StateBlock kind="error" message={error}>
      <button class="btn" onclick={load}>Reintentar</button>
    </StateBlock>
  {:else if loading && !loaded}
    <StateBlock kind="loading" message="Cargando registro…" />
  {:else if items.length === 0}
    <StateBlock kind="empty" title="Sin entradas" message="No hay solicitudes que coincidan con los filtros." />
  {:else}
    <div class="panel table-wrap">
      <table class="data-table log-table">
        <thead>
          <tr>
            <th>Fecha</th>
            <th>Método</th>
            <th>Ruta</th>
            <th>Estado</th>
            <th class="num">Bytes</th>
            <th class="num">Duración</th>
            <th>IP</th>
          </tr>
        </thead>
        <tbody>
          {#each items as row}
            <tr>
              <td class="mono nowrap">{formatDate(row.time)}</td>
              <td><span class="method method-{String(row.method || '').toLowerCase()}">{row.method || '—'}</span></td>
              <td class="mono path" title={row.path || ''}>{truncate(row.path || '—', 70)}</td>
              <td><span class="status status-{statusTone(Number(row.status))}">{row.status ?? '—'}</span></td>
              <td class="num mono">{formatNumber(row.bytes || 0)}</td>
              <td class="num mono">{formatDuration(row.durationMs)}</td>
              <td class="mono nowrap" title={row.ip || ''}>{row.ip || '—'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<style>
  .filters {
    display: grid;
    grid-template-columns: 2fr 1fr 0.7fr;
    gap: var(--space-sm);
    padding: var(--space-md);
  }
  .narrow { max-width: 120px; }
  .log-meta {
    padding: 0 var(--space-md) var(--space-sm);
    font-size: 0.8rem;
  }
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
  .data-table td {
    padding: 0.55rem var(--space-md);
    border-bottom: 1px solid var(--border-subtle);
    vertical-align: middle;
  }
  .data-table tbody tr:last-child td { border-bottom: 0; }
  .num { text-align: right; }
  .nowrap { white-space: nowrap; }
  .path { max-width: 320px; overflow: hidden; text-overflow: ellipsis; }
  .method {
    font-family: var(--font-mono);
    font-size: 0.75rem;
    font-weight: 600;
  }
  .method-get { color: var(--accent); }
  .method-post { color: var(--success); }
  .method-put, .method-patch { color: var(--warning); }
  .method-delete { color: var(--danger); }
  .status {
    display: inline-block;
    min-width: 2.8em;
    text-align: center;
    padding: 0.1rem 0.4rem;
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: 0.78rem;
    font-weight: 600;
  }
  .status-success { color: var(--success); background: var(--success-bg); }
  .status-warning { color: var(--warning); background: var(--warning-bg); }
  .status-danger { color: var(--danger); background: var(--danger-bg); }
  @media (max-width: 720px) {
    .filters { grid-template-columns: 1fr 1fr; }
    .search-field { grid-column: 1 / -1; }
  }
</style>

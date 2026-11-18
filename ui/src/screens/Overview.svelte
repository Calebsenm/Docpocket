<script>
  import { onMount } from 'svelte'
  import { api, apiText } from '../lib/api.js'
  import { formatBytes, formatNumber, formatUptime, formatDate, parsePrometheus, formatDuration, truncate } from '../lib/format.js'
  import StateBlock from '../lib/components/StateBlock.svelte'
  import Icon from '../lib/components/Icon.svelte'

  let loading = $state(true)
  let health = $state(null)
  let clients = $state(null)
  let failures = $state([])
  let logsAvailable = $state(true)
  let collections = $state([])
  let aggregates = $state([])
  let errors = $state([])
  let updatedAt = $state(null)

  const totalDocuments = $derived(
    aggregates.length && aggregates.every((a) => a.countOk)
      ? aggregates.reduce((sum, a) => sum + (a.count || 0), 0)
      : null
  )
  const maxCount = $derived(Math.max(1, ...aggregates.map((a) => a.count || 0)))
  const charts = $derived([...aggregates].sort((a, b) => (b.count || 0) - (a.count || 0)).slice(0, 10))

  function bucket(count) {
    if (!count) return 0
    return Math.max(1, Math.min(10, Math.round((count / maxCount) * 10)))
  }

  async function load() {
    loading = true
    errors = []
    const [healthResult, metricsResult, logsResult, collectionsResult] = await Promise.allSettled([
      api('/api/health', { key: 'overview-health', timeoutMs: 8000 }),
      apiText('/metrics', { key: 'overview-metrics', timeoutMs: 8000 }),
      api('/api/admin/logs', { params: { limit: 50 }, key: 'overview-logs' }),
      api('/api/admin/collections', { key: 'overview-collections' }),
    ])

    health = healthResult.status === 'fulfilled' ? healthResult.value : null
    if (healthResult.status === 'rejected') errors.push(`Salud: ${healthResult.reason?.message || 'no disponible'}`)

    if (metricsResult.status === 'fulfilled') {
      clients = parsePrometheus(metricsResult.value).docpocket_realtime_clients ?? null
    } else {
      clients = null
      errors.push(`Métricas: ${metricsResult.reason?.message || 'no disponible'}`)
    }

    if (logsResult.status === 'fulfilled') {
      failures = (logsResult.value?.items || []).filter((entry) => Number(entry?.status) >= 400).slice(0, 6)
      logsAvailable = true
    } else {
      failures = []
      logsAvailable = false
      errors.push(`Registro: ${logsResult.reason?.message || 'no disponible'}`)
    }

    const cols = collectionsResult.status === 'fulfilled' ? collectionsResult.value?.items || [] : []
    collections = cols
    if (collectionsResult.status === 'rejected') errors.push(`Colecciones: ${collectionsResult.reason?.message || 'no disponible'}`)

    // Los gráficos se calculan exclusivamente con /aggregate del servidor.
    const numericFields = (col) => (col.fields || []).filter((f) => f.type === 'number')
    const jobs = cols.slice(0, 12).map(async (col) => {
      const field = numericFields(col)[0]?.name || ''
      const [countRes, sumRes] = await Promise.allSettled([
        api(`/api/collections/${encodeURIComponent(col.name)}/aggregate`, {
          method: 'POST',
          body: { op: 'count', filter: '' },
          key: `overview-count-${col.name}`,
        }),
        field
          ? api(`/api/collections/${encodeURIComponent(col.name)}/aggregate`, {
              method: 'POST',
              body: { op: 'sum', field, filter: '' },
              key: `overview-sum-${col.name}`,
            })
          : Promise.resolve(null),
      ])
      return {
        name: col.name,
        count: countRes.status === 'fulfilled' ? Number(countRes.value?.result) || 0 : null,
        countOk: countRes.status === 'fulfilled',
        field,
        sum: sumRes.status === 'fulfilled' && sumRes.value ? Number(sumRes.value?.result) : null,
        sumOk: sumRes.status === 'fulfilled' && Boolean(sumRes.value),
      }
    })
    aggregates = await Promise.all(jobs)
    updatedAt = new Date()
    loading = false
  }

  onMount(load)
</script>

<div class="stack">
  <div class="page-head">
    <div class="page-title">
      <h1>Resumen del sistema</h1>
      <p class="page-subtitle">
        Estado real del servidor, métricas y distribución de documentos.
        {#if updatedAt}<span class="muted"> · Actualizado {formatDate(updatedAt)}</span>{/if}
      </p>
    </div>
    <div class="page-actions">
      <button type="button" class="btn" onclick={load} disabled={loading}>
        {#if loading}<span class="spinner" aria-hidden="true"></span>{:else}<Icon name="refresh" size={14} />{/if}
        Actualizar
      </button>
    </div>
  </div>

  {#if errors.length}
    <div class="callout callout-warn">
      <span aria-hidden="true">⚠</span>
      <span>Algunos datos no están disponibles: {errors.join('; ')}. No se muestran valores estimados.</span>
    </div>
  {/if}

  <div class="grid grid-4">
    <div class="metric-card">
      <span class="metric-label">Estado</span>
      <span class="metric-value">{health?.status || '—'}</span>
      <span class="metric-note">Versión {health?.version || 'no disponible'}</span>
    </div>
    <div class="metric-card">
      <span class="metric-label">Tiempo activo</span>
      <span class="metric-value">{formatUptime(health?.uptime)}</span>
      <span class="metric-note">Desde el último arranque</span>
    </div>
    <div class="metric-card">
      <span class="metric-label">Tamaño de la BD</span>
      <span class="metric-value">{formatBytes(health?.dbSize)}</span>
      <span class="metric-note">Dato de /api/health</span>
    </div>
    <div class="metric-card">
      <span class="metric-label">Colecciones</span>
      <span class="metric-value">{health?.collections ?? '—'}</span>
      <span class="metric-note">Total Documents: {totalDocuments === null ? '—' : formatNumber(totalDocuments)}</span>
    </div>
    <div class="metric-card">
      <span class="metric-label">Clientes en tiempo real</span>
      <span class="metric-value">{clients === null ? '—' : formatNumber(clients)}</span>
      <span class="metric-note">Métrica realtime_clients de /metrics</span>
    </div>
    <div class="metric-card">
      <span class="metric-label">Errores recientes</span>
      <span class="metric-value">{logsAvailable ? failures.length : '—'}</span>
      <span class="metric-note">Últimas peticiones fallidas</span>
    </div>
  </div>

  <div class="grid grid-2">
    <section class="card">
      <div class="card-head">
        <div class="card-title-group">
          <h2>Documentos por colección</h2>
          <p class="muted tiny">Conteo real mediante POST /aggregate</p>
        </div>
      </div>
      <div class="card-body">
        {#if loading}
          <StateBlock kind="loading" title="Calculando agregados…" />
        {:else if !charts.length}
          <StateBlock kind="empty" title="Sin colecciones" detail="Crea una colección para ver su distribución real." />
        {:else}
          <div class="chart-list">
            {#each charts as item (item.name)}
              <div class="bar-row">
                <span class="bar-label" title={item.name}>{item.name}</span>
                <div
                  class="bar-track"
                  role="img"
                  aria-label={`${item.name}: ${item.countOk ? formatNumber(item.count) : 'no disponible'} documentos`}
                >
                  <div class="bar-fill bar-bucket-{bucket(item.count)}"></div>
                </div>
                <span class="bar-value">{item.countOk ? formatNumber(item.count) : '—'}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </section>

    <section class="card">
      <div class="card-head">
        <div class="card-title-group">
          <h2>Sumas por campo numérico</h2>
          <p class="muted tiny">Sin sumas estimadas: solo resultados del servidor</p>
        </div>
      </div>
      <div class="card-body">
        {#if aggregates.filter((a) => a.field).length === 0}
          <StateBlock kind="empty" title="Sin campos numéricos" detail="Declara campos de tipo number en tus colecciones para poder sumarlos." />
        {:else}
          <div class="kv-list">
            {#each aggregates.filter((a) => a.field) as item (item.name)}
              <div class="kv-row">
                <span class="kv-key">{item.name} · <code>{item.field}</code></span>
                <span class="mono">{item.sumOk ? formatNumber(item.sum) : 'No disponible'}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </section>
  </div>

  <section class="card">
    <div class="card-head">
      <div class="card-title-group">
        <h2>Fallos recientes</h2>
        <p class="muted tiny">Registro de peticiones del servidor con estado de error</p>
      </div>
    </div>
    <div class="card-body">
      {#if !logsAvailable}
        <StateBlock kind="error" title="Registro no disponible" detail="El endpoint /api/admin/logs no respondió." onRetry={load} />
      {:else if !failures.length}
        <StateBlock kind="empty" title="Sin fallos registrados" detail="No hay peticiones fallidas en el búfer del servidor." />
      {:else}
        <div class="table-wrap">
          <table>
            <thead>
              <tr><th>Hora</th><th>ID</th><th>Método</th><th>Ruta</th><th>Estado</th><th>Duración</th><th>IP</th></tr>
            </thead>
            <tbody>
              {#each failures as log (log.id || `${log.time}-${log.path}`)}
                <tr>
                  <td class="nowrap">{formatDate(log.time)}</td>
                  <td class="mono tiny">{truncate(log.id, 10)}</td>
                  <td class="mono tiny">{log.method}</td>
                  <td class="mono tiny truncate">{log.path}</td>
                  <td class="log-status error">{log.status}</td>
                  <td class="nowrap">{formatDuration(log.durationMs)}</td>
                  <td class="mono tiny">{log.ip || '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </section>
</div>

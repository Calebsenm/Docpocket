<script>
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api.js'
  import { parseJson, formatDate, truncate } from '../lib/format.js'
  import { notify } from '../lib/stores.js'
  import Icon from '../lib/components/Icon.svelte'
  import StateBlock from '../lib/components/StateBlock.svelte'

  let collections = $state([])
  let subscriptionsText = $state('')
  let clientId = $state('')
  let lastEventId = $state('')
  let status = $state('desconectado')
  let readyState = $state(-1)
  let events = $state([])
  let eventsError = $state('')
  let filter = $state('')
  let reconnectCount = $state(0)

  let source = null
  let retryTimer = null
  let mounted = false

  const visibleEvents = $derived(
    events.filter((event) => `${event.action} ${event.collection} ${event.id}`.toLowerCase().includes(filter.trim().toLowerCase()))
  )

  function pushEvent(event, type) {
    const parsed = parseJson(event.data)
    if (event.lastEventId) lastEventId = event.lastEventId
    if (!parsed.ok) return
    const payload = parsed.value || {}
    events = [
      {
        action: payload.action || type || 'evento',
        collection: payload.collection || type || '—',
        id: payload.id || '',
        time: new Date(),
        seq: event.lastEventId || '',
      },
      ...events,
    ].slice(0, 250)
  }

  function attachListeners(stream, cols) {
    stream.addEventListener('connect', (event) => {
      const parsed = parseJson(event.data)
      clientId = parsed.value?.clientId || ''
      readyState = stream.readyState
      status = 'conectado'
      if (clientId) subscribe(false)
    })
    for (const col of cols) {
      stream.addEventListener(col.name, (event) => pushEvent(event, col.name))
    }
  }

  async function subscribe(showToast = true) {
    if (!clientId) return
    try {
      const subscriptions = subscriptionsText
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      await api('/api/realtime', {
        method: 'POST',
        body: { clientId, subscriptions },
      })
      if (showToast) notify('Suscripciones actualizadas', 'success')
    } catch (error) {
      if (showToast) notify(error instanceof Error ? error.message : String(error), 'error')
    }
  }

  function connect(resume = false) {
    if (!mounted) return
    if (retryTimer) {
      clearTimeout(retryTimer)
      retryTimer = null
    }
    source?.close()
    const suffix = resume && lastEventId ? `?lastEventId=${encodeURIComponent(lastEventId)}` : ''
    const stream = new EventSource(`/api/realtime${suffix}`)
    source = stream
    readyState = stream.readyState
    status = 'conectando'
    attachListeners(stream, collections)
    stream.onopen = () => {
      readyState = stream.readyState
      status = 'conectado'
      reconnectCount = 0
    }
    stream.onerror = () => {
      readyState = stream.readyState
      // EventSource reintenta automáticamente mientras esté en CONNECTING;
      // solo programamos un reconector manual si el navegador lo cerró.
      if (stream.readyState === EventSource.CLOSED) {
        status = 'cerrado'
        reconnectCount += 1
        const delayMs = Math.min(15000, 1000 * 2 ** Math.min(reconnectCount, 4))
        retryTimer = setTimeout(() => connect(Boolean(lastEventId)), delayMs)
      } else {
        status = 'reconectando'
      }
    }
  }

  function reconnect() {
    reconnectCount = 0
    connect(Boolean(lastEventId))
  }

  function disconnect() {
    source?.close()
    source = null
    if (retryTimer) clearTimeout(retryTimer)
    retryTimer = null
    readyState = -1
    status = 'desconectado'
  }

  async function loadCollections() {
    eventsError = ''
    try {
      const data = await api('/api/admin/collections', { key: 'realtime-collections' })
      collections = data?.items || []
      subscriptionsText = collections.map((c) => c.name).join(', ')
      if (source) {
        // La reconexión instala listeners para las colecciones actuales.
        reconnect()
      }
    } catch (error) {
      eventsError = error instanceof Error ? error.message : String(error)
    }
  }

  const stateLabel = $derived(
    status === 'conectado'
      ? 'Conectado'
      : status === 'conectando'
        ? 'Conectando…'
        : status === 'reconectando'
          ? 'Reconectando…'
          : status === 'cerrado'
            ? 'Cerrado'
            : 'Desconectado'
  )
  const stateClass = $derived(status === 'conectado' ? 'is-ok' : status === 'desconectado' ? '' : 'is-warn')

  onMount(() => {
    mounted = true
    loadCollections().finally(() => connect())
  })

  onDestroy(() => {
    mounted = false
    disconnect()
  })
</script>

<div class="stack">
  <div class="page-head">
    <div class="page-title">
      <h1>Tiempo real</h1>
      <p class="page-subtitle">Flujo SSE real de /api/realtime con suscripciones y eventos en vivo.</p>
    </div>
    <div class="page-actions">
      <div class="header-status">
        <span class="status-dot {stateClass}" aria-hidden="true"></span>
        <span>{stateLabel} · readyState {readyState}</span>
      </div>
      {#if status === 'desconectado' || status === 'cerrado'}
        <button type="button" class="btn btn-sm btn-primary" onclick={reconnect}>Conectar</button>
      {:else}
        <button type="button" class="btn btn-sm" onclick={disconnect}>Desconectar</button>
      {/if}
    </div>
  </div>

  <div class="callout callout-info">
    <span aria-hidden="true">ℹ</span>
    <span>
      El navegador no permite cabeceras en EventSource, por lo que el flujo se abre sin token y las
      suscripciones se envían con POST /api/realtime usando Bearer. Los eventos se reconectan con
      Last-Event-ID cuando está disponible.
    </span>
  </div>

  <section class="card">
    <div class="card-head">
      <div class="card-title-group">
        <h2>Suscripciones</h2>
        <p class="muted tiny">ID de cliente: <code>{clientId || 'pendiente de connect'}</code></p>
      </div>
      <button type="button" class="btn btn-sm" onclick={loadCollections}>Recargar colecciones</button>
    </div>
    <div class="card-body row">
      <div class="field">
        <span class="field-label">Colecciones (separadas por comas)</span>
        <input class="input" type="text" bind:value={subscriptionsText} placeholder="articulos, usuarios" aria-label="Suscripciones" />
      </div>
      <button type="button" class="btn btn-primary" onclick={() => subscribe()} disabled={!clientId}>Aplicar suscripciones</button>
      {#if lastEventId}<span class="badge">Último ID: {lastEventId}</span>{/if}
      {#if reconnectCount}<span class="badge badge-warn">Reconexiones: {reconnectCount}</span>{/if}
    </div>
  </section>

  <section class="card">
    <div class="card-head">
      <div class="card-title-group">
        <h2>Feed de eventos</h2>
        <p class="muted tiny">Máximo 250 eventos en memoria. No se fabrican eventos de ejemplo.</p>
      </div>
      <div class="search-input">
        <Icon name="search" size={14} />
        <input class="input" type="search" bind:value={filter} placeholder="Filtrar eventos…" aria-label="Filtrar eventos" />
      </div>
    </div>
    <div class="card-body">
      {#if eventsError}<StateBlock kind="error" title="Error al cargar colecciones" detail={eventsError} onRetry={loadCollections} />{/if}
      {#if !visibleEvents.length}
        <StateBlock kind="empty" title="Sin eventos" detail="Esperando cambios en las colecciones suscritas…" />
      {:else}
        <div class="event-list">
          {#each visibleEvents as event, index (index)}
            <div class="event-row">
              <span class="event-action {event.action}">{event.action}</span>
              <span class="mono tiny truncate">{event.collection}</span>
              <span class="mono tiny truncate">{event.id || '—'}</span>
              <span class="muted tiny">{formatDate(event.time)}</span>
              <span class="muted tiny mono">{event.seq ? `#${truncate(event.seq, 10)}` : '—'}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </section>
</div>

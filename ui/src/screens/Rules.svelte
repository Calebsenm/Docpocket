<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import { lineNumbers, tokenizeRule, parseJson, isRecord } from '../lib/format.js'
  import { notify } from '../lib/stores.js'
  import Icon from '../lib/components/Icon.svelte'
  import StateBlock from '../lib/components/StateBlock.svelte'
  import JsonEditor from '../lib/components/JsonEditor.svelte'

  const operations = ['list', 'view', 'create', 'update', 'delete']
  const presets = {
    public: { label: 'Acceso público', rule: '' },
    auth: { label: 'Solo autenticados', rule: 'request.auth != null' },
    owner: { label: 'Solo el propietario', rule: 'request.auth != null && resource.owner == request.auth.id' },
    admin: { label: 'Solo administradores', rule: "request.auth != null && request.auth.role == 'admin'" },
    verified: { label: 'Solo verificados', rule: 'request.auth != null && request.auth.verified == true' },
  }
  const tokensToInsert = ['request.auth', 'resource', 'request.body', 'request.query', 'now', '&&', '||']

  let collections = $state([])
  let collection = $state('')
  let operation = $state('view')
  let rule = $state('request.auth != null && resource.owner == request.auth.id')
  let contextJson = $state(`{
  "auth": { "id": "user_123", "role": "member" },
  "resource": { "id": "doc_01", "owner": "user_123" },
  "body": {},
  "query": {}
}`)
  let result = $state(null)
  let error = $state('')
  let busy = $state(false)
  let textareaEl

  const lines = $derived(lineNumbers(rule))
  const tokens = $derived(tokenizeRule(rule))

  async function loadCollections() {
    try {
      const data = await api('/api/admin/collections', { key: 'rules-collections' })
      collections = data?.items || []
      if (!collection && collections.length) collection = collections[0].name
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    }
  }

  function applyPreset(key) {
    rule = presets[key]?.rule || ''
    textareaEl?.focus()
  }

  function insertToken(token) {
    const el = textareaEl
    if (!el) {
      rule += token
      return
    }
    const start = el.selectionStart ?? rule.length
    const end = el.selectionEnd ?? start
    el.setRangeText(token, start, end, 'end')
    rule = el.value
    el.focus()
  }

  async function evaluate() {
    error = ''
    result = null
    const parsed = parseJson(contextJson)
    if (!parsed.ok || !isRecord(parsed.value)) {
      notify(`Contexto JSON inválido: ${parsed.error || 'debe ser un objeto'}`, 'error')
      return
    }
    const context = parsed.value
    busy = true
    try {
      result = await api('/api/admin/rules/test', {
        method: 'POST',
        body: {
          collection,
          operation,
          rule,
          auth: context.auth ?? null,
          resource: context.resource ?? null,
          body: context.body ?? null,
          query: context.query ?? null,
        },
      })
      notify(result?.locked ? 'Operación bloqueada para usuarios normales' : result?.allowed ? 'Regla evaluada: PERMITIDA' : 'Regla evaluada: DENEGADA', result?.allowed && !result?.locked ? 'success' : 'info')
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
      result = { error }
      notify(error, 'error')
    } finally {
      busy = false
    }
  }

  onMount(loadCollections)
</script>

<div class="stack">
  <div class="page-head">
    <div class="page-title">
      <h1>Probador de reglas CEL</h1>
      <p class="page-subtitle">Evalúa expresiones con el motor real del servidor y un contexto de prueba.</p>
    </div>
    <div class="page-actions">
      <select class="select" bind:value={collection} aria-label="Colección">
        {#each collections as col (col.name)}<option value={col.name}>{col.name}</option>{/each}
        {#if !collections.length}<option value="">Sin colecciones</option>{/if}
      </select>
      <select class="select" bind:value={operation} aria-label="Operación">
        {#each operations as op (op)}<option value={op}>{op}</option>{/each}
      </select>
    </div>
  </div>

  {#if error && !result}<StateBlock kind="error" title="Error" detail={error} onRetry={loadCollections} />{/if}

  <section class="card">
    <div class="card-head">
      <div class="card-title-group">
        <h2>Expresión</h2>
        <p class="muted tiny">Resaltado seguro: fragmentos de texto, sin HTML inyectado.</p>
      </div>
      <div class="row">
        <span class="muted tiny">Plantilla:</span>
        <select class="select" aria-label="Plantilla de regla" onchange={(e) => applyPreset(e.currentTarget.value)}>
          <option value="">Seleccionar…</option>
          {#each Object.entries(presets) as [key, preset] (key)}<option value={key}>{preset.label}</option>{/each}
        </select>
      </div>
    </div>
    <div class="card-body stack">
      <div class="editor-shell">
        <div class="editor-toolbar">
          <span class="tiny muted">Regla CEL para <code>{collection || '—'}</code> · <code>{operation}</code></span>
          <span class="badge">{lines.length} {lines.length === 1 ? 'línea' : 'líneas'}</span>
        </div>
        <div class="editor-main">
          <div class="editor-gutter" aria-hidden="true">{lines.join('\n')}</div>
          <textarea
            bind:this={textareaEl}
            bind:value={rule}
            class="editor-area"
            rows="4"
            spellcheck="false"
            aria-label="Expresión CEL"
            placeholder="request.auth != null"
          ></textarea>
        </div>
      </div>
      <pre class="token-preview" aria-label="Vista con resaltado de tokens">{#each tokens as token}<span class="tok-{token.kind}">{token.text}</span>{/each}</pre>
      <div class="token-chips">
        <span class="muted tiny">Insertar:</span>
        {#each tokensToInsert as token (token)}
          <button type="button" class="chip-btn" onclick={() => insertToken(token)}>{token}</button>
        {/each}
      </div>
    </div>
  </section>

  <div class="grid grid-2">
    <section class="card">
      <div class="card-head"><h2>Contexto de prueba</h2></div>
      <div class="card-body">
        <JsonEditor label="auth / resource / body / query" bind:value={contextJson} rows={12} />
      </div>
    </section>

    <section class="card">
      <div class="card-head">
        <h2>Resultado</h2>
        <button type="button" class="btn btn-primary btn-sm" onclick={evaluate} disabled={busy}>
          {#if busy}<span class="spinner" aria-hidden="true"></span>{:else}<Icon name="play" size={13} />{/if}
          Evaluar regla
        </button>
      </div>
      <div class="card-body stack">
        {#if !result}
          <StateBlock kind="empty" title="Sin evaluación" detail="Pulsa «Evaluar regla» para consultar el motor CEL del servidor." />
        {:else if result.error}
          <div class="callout callout-danger wrap-text">{result.error}</div>
        {:else}
          <div class="metric-card">
            <span class="metric-label">Permitida</span>
            <span class="metric-value">{result.locked ? 'Bloqueada' : result.allowed ? 'Sí' : 'No'}</span>
            <span class="metric-note">{result.message || `Operación ${operation} en ${collection || '—'}`}</span>
          </div>
          <div class="kv-list">
            <div class="kv-row"><span class="kv-key">locked</span><span>{String(Boolean(result.locked))}</span></div>
            <div class="kv-row"><span class="kv-key">rule evaluada</span><code class="wrap-text">{result.rule || '—'}</code></div>
          </div>
        {/if}
        {#if error && result}<div class="callout callout-danger mt-2">{error}</div>{/if}
      </div>
    </section>
  </div>
</div>

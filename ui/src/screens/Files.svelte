<script>
  import { onMount } from 'svelte'
  import { api, buildURL, downloadBlob } from '../lib/api.js'
  import { formatDate, truncate, parseJson } from '../lib/format.js'
  import { notify } from '../lib/stores.js'
  import Icon from '../lib/components/Icon.svelte'
  import StateBlock from '../lib/components/StateBlock.svelte'
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte'

  const reserved = new Set(['id', 'created', 'updated', 'email', 'passwordHash', 'tokenKey', 'verified'])
  const looksLikeFile = (value) => typeof value === 'string' && /\.[a-z0-9]{2,8}$/i.test(value) && !value.includes(' ')

  let collections = $state([])
  let selectedCollection = $state('')
  let detail = $state(null)
  let records = $state([])
  let files = $state([])
  let loading = $state(false)
  let error = $state('')
  let uploadTarget = $state('')
  let uploadField = $state('file')
  let selectedFile = $state(null)
  let uploading = $state(false)
  let deleteOpen = $state(false)
  let deleteTarget = $state(null)

  const fileFields = $derived((detail?.fields || []).filter((f) => f.type === 'file').map((f) => f.name))

  function scanFiles() {
    const declared = new Set(fileFields)
    const rows = []
    for (const record of records) {
      for (const [field, value] of Object.entries(record || {})) {
        if (reserved.has(field) || !value) continue
        if (declared.has(field) || looksLikeFile(value)) {
          const names = Array.isArray(value) ? value.filter(looksLikeFile) : [value]
          for (const name of names) {
            if (typeof name !== 'string') continue
            rows.push({ id: record.id, field, name, updated: record.updated || record.created })
          }
        }
      }
    }
    files = rows
  }

  async function loadCollections() {
    try {
      const data = await api('/api/admin/collections', { key: 'files-collections' })
      collections = data?.items || []
      if (!selectedCollection && collections.length) selectedCollection = collections[0].name
      await loadData()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    }
  }

  async function loadData() {
    if (!selectedCollection) return
    loading = true
    error = ''
    try {
      const [d, page] = await Promise.all([
        api(`/api/admin/collections/${encodeURIComponent(selectedCollection)}`, { key: `files-detail-${selectedCollection}` }),
        api(buildURL(`/api/admin/collections/${encodeURIComponent(selectedCollection)}/records`, { limit: 500 }), { key: `files-records-${selectedCollection}` }),
      ])
      detail = d
      records = page?.items || []
      if (!uploadTarget && records.length) uploadTarget = records[0].id
      if (fileFields.length && !uploadField) uploadField = fileFields[0]
      scanFiles()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
      files = []
      records = []
    } finally {
      loading = false
    }
  }

  async function upload() {
    if (!selectedFile || !uploadTarget) {
      notify('Selecciona un documento y un archivo', 'error')
      return
    }
    uploading = true
    try {
      const form = new FormData()
      form.append('file', selectedFile)
      if (uploadField.trim()) form.append('field', uploadField.trim())
      await api(`/api/collections/${encodeURIComponent(selectedCollection)}/records/${encodeURIComponent(uploadTarget)}/files`, {
        method: 'POST',
        body: form,
      })
      notify('Archivo subido correctamente', 'success')
      selectedFile = null
      await loadData()
    } catch (err) {
      notify(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      uploading = false
    }
  }

  async function download(row) {
    try {
      await downloadBlob(
        `/api/files/${encodeURIComponent(selectedCollection)}/${encodeURIComponent(row.id)}/${encodeURIComponent(row.name)}`,
        row.name
      )
      notify('Descarga iniciada', 'success')
    } catch (err) {
      notify(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  async function removeFile() {
    if (!deleteTarget) return
    const params = deleteTarget.field ? `?field=${encodeURIComponent(deleteTarget.field)}` : ''
    await api(`/api/collections/${encodeURIComponent(selectedCollection)}/records/${encodeURIComponent(deleteTarget.id)}/files/${encodeURIComponent(deleteTarget.name)}${params}`, { method: 'DELETE' })
    notify('Archivo eliminado', 'success')
    await loadData()
  }

  onMount(loadCollections)
</script>

<div class="stack">
  <div class="page-head">
    <div class="page-title">
      <h1>Archivos</h1>
      <p class="page-subtitle">Archivos adjuntos a documentos, con descarga autenticada mediante fetch.</p>
    </div>
    <div class="page-actions">
      <select class="select" bind:value={selectedCollection} onchange={loadData} aria-label="Colección">
        {#each collections as col (col.name)}<option value={col.name}>{col.name}</option>{/each}
        {#if !collections.length}<option value="">Sin colecciones</option>{/if}
      </select>
      <button type="button" class="btn btn-sm" onclick={loadData}><Icon name="refresh" size={14} /> Actualizar</button>
    </div>
  </div>

  {#if error}
    <StateBlock kind="error" title="No se pudieron cargar los archivos" detail={error} onRetry={loadData} />
  {:else}
    <section class="card">
      <div class="card-head">
        <div class="card-title-group">
          <h2>Subir archivo</h2>
          <p class="muted tiny">Multipart con parte <code>file</code> y campo opcional <code>field</code>.</p>
        </div>
      </div>
      <div class="card-body">
        <div class="row">
          <div class="field">
            <span class="field-label">Documento destino</span>
            <select class="select" bind:value={uploadTarget} aria-label="Documento destino">
              <option value="">Selecciona un documento</option>
              {#each records as record (record.id)}<option value={record.id}>{truncate(record.id, 24)}</option>{/each}
            </select>
          </div>
          <div class="field">
            <span class="field-label">Campo del documento</span>
            <input class="input" type="text" bind:value={uploadField} placeholder="file" aria-label="Campo del documento" />
          </div>
          <div class="field">
            <span class="field-label">Archivo</span>
            <input class="input" type="file" onchange={(e) => (selectedFile = e.currentTarget.files?.[0] || null)} aria-label="Archivo a subir" />
          </div>
          <button type="button" class="btn btn-primary" onclick={upload} disabled={uploading || !selectedFile || !uploadTarget}>
            {#if uploading}<span class="spinner" aria-hidden="true"></span>{:else}<Icon name="upload" size={14} />{/if}
            Subir
          </button>
        </div>
        {#if fileFields.length}
          <p class="muted tiny mt-2">Campos de tipo file declarados: {fileFields.join(', ')}</p>
        {/if}
      </div>
    </section>

    <section class="card">
      <div class="card-head">
        <div class="card-title-group">
          <h2>{files.length} archivos detectados en {selectedCollection || '—'}</h2>
          <p class="muted tiny">Se detectan campos declarados como <code>file</code> y cadenas con extensión de archivo.</p>
        </div>
      </div>
      <div class="card-body">
        {#if loading}
          <StateBlock kind="loading" title="Escaneando registros…" />
        {:else if !files.length}
          <StateBlock kind="empty" title="Sin archivos" detail="No se encontraron referencias a archivos en los documentos de esta colección." />
        {:else}
          <div class="table-wrap">
            <table>
              <thead><tr><th>Documento</th><th>Campo</th><th>Archivo</th><th>Actualizado</th><th>Acciones</th></tr></thead>
              <tbody>
                {#each files as file, index (index)}
                  <tr>
                    <td class="id-pill">{truncate(file.id, 14)}</td>
                    <td class="mono tiny">{file.field}</td>
                    <td class="mono tiny wrap-text">{file.name}</td>
                    <td class="tiny muted nowrap">{formatDate(file.updated)}</td>
                    <td>
                      <div class="row-actions">
                        <button type="button" class="btn btn-xs" onclick={() => download(file)}><Icon name="download" size={12} /> Descargar</button>
                        <button type="button" class="btn btn-xs btn-danger" onclick={() => { deleteTarget = file; deleteOpen = true }}>Eliminar</button>
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

<ConfirmDialog
  bind:open={deleteOpen}
  title="Eliminar archivo"
  message={`¿Eliminar "${deleteTarget?.name || ''}" del documento ${deleteTarget?.id || ''}?`}
  confirmLabel="Eliminar archivo"
  danger={true}
  onConfirm={removeFile}
/>

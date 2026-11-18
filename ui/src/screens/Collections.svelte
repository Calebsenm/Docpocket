<script>
  import { onMount } from 'svelte'
  import { api, buildURL } from '../lib/api.js'
  import { formatDate, formatNumber, truncate, isRecord, parseJson } from '../lib/format.js'
  import { notify } from '../lib/stores.js'
  import Icon from '../lib/components/Icon.svelte'
  import StateBlock from '../lib/components/StateBlock.svelte'
  import Modal from '../lib/components/Modal.svelte'
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte'
  import JsonEditor from '../lib/components/JsonEditor.svelte'

  const ruleOps = ['list', 'view', 'create', 'update', 'delete']
  const fieldTypes = ['string', 'number', 'bool', 'array', 'object', 'file', 'any']
  const copy = (value) => JSON.parse(JSON.stringify(value ?? null))

  let collections = $state([])
  let selected = $state('')
  let detail = $state(null)
  let search = $state('')
  let loading = $state(false)
  let listError = $state('')
  let detailError = $state('')
  let tab = $state('records')

  let query = $state({ filter: '', sort: '', limit: 25, fields: '', include: '', view: '' })
  let records = $state([])
  let nextCursor = $state('')
  let cursorStack = $state([])
  let recordsLoading = $state(false)
  let recordsError = $state('')
  let views = $state([])
  let viewsError = $state('')

  let schemaOpen = $state(false)
  let schemaDraft = $state(null)
  let ruleConfig = $state([])
  let savingSchema = $state(false)

  const templates = [
    {
      id: 'blog',
      label: 'Artículos o blog',
      hint: 'Noticias, entradas, texto largo',
      icon: 'file',
      type: 'base',
      sampleName: 'articulos',
      fields: [
        { name: 'titulo', type: 'string', required: true },
        { name: 'contenido', type: 'string' },
        { name: 'publicado', type: 'bool' },
      ],
      sample: { titulo: 'Mi primer artículo', contenido: 'Escribe aquí el contenido', publicado: false },
    },
    {
      id: 'productos',
      label: 'Productos o tienda',
      hint: 'Catálogo con precio y stock',
      icon: 'grid',
      type: 'base',
      sampleName: 'productos',
      fields: [
        { name: 'nombre', type: 'string', required: true },
        { name: 'precio', type: 'number', required: true },
        { name: 'stock', type: 'number' },
      ],
      sample: { nombre: 'Mi primer producto', precio: 9.99, stock: 10 },
    },
    {
      id: 'tareas',
      label: 'Tareas o proyectos',
      hint: 'Listas de cosas por hacer',
      icon: 'list',
      type: 'base',
      sampleName: 'tareas',
      fields: [
        { name: 'titulo', type: 'string', required: true },
        { name: 'hecha', type: 'bool' },
        { name: 'prioridad', type: 'number' },
      ],
      sample: { titulo: 'Mi primera tarea', hecha: false, prioridad: 1 },
    },
    {
      id: 'usuarios',
      label: 'Usuarios con acceso',
      hint: 'Cuentas con correo y contraseña',
      icon: 'users',
      type: 'auth',
      sampleName: 'clientes',
      fields: [{ name: 'nombre', type: 'string' }],
      sample: { nombre: 'Ana García' },
    },
    {
      id: 'blanco',
      label: 'En blanco',
      hint: 'Empieza de cero, añade campos después',
      icon: 'plus',
      type: 'base',
      sampleName: 'nueva_coleccion',
      fields: [],
      sample: {},
    },
  ]

  let createOpen = $state(false)
  let createStep = $state(1)
  let createTemplate = $state('blog')
  let newCollection = $state({ name: '', public: false })
  let creating = $state(false)
  let createdName = $state('')

  const activeTemplate = $derived(templates.find((t) => t.id === createTemplate) || templates[0])

  let recordOpen = $state(false)
  let recordMode = $state('create')
  let recordId = $state('')
  let recordJson = $state('{}')
  let recordHttp = $state('PATCH')
  let savingRecord = $state(false)
  const hiddenRecordFields = ['id', 'created', 'updated', 'passwordHash', 'tokenKey', 'verified']
  let recordEditor = $state('form')
  let recordValues = $state({})
  let recordBase = $state({})
  let recordErrors = $state({})

  let deleteCollectionOpen = $state(false)
  let deleteRecordTarget = $state(null)
  let deleteRecordOpen = $state(false)

  let agg = $state({ op: 'count', field: '', filter: '' })
  let aggResult = $state(null)
  let aggError = $state('')

  let batchRows = $state([{ method: 'POST', collection: '', id: '', body: '{}' }])
  let useTransaction = $state(false)
  let batchResult = $state(null)
  let batchError = $state('')
  let batchBusy = $state(false)

  let viewOpen = $state(false)
  let newView = $state({ name: '', filter: '', sort: '', shared: false })

  const filtered = $derived(
    collections.filter((c) => c.name.toLowerCase().includes(search.trim().toLowerCase()))
  )
  const recordKeys = $derived(
    (detail?.fields || [])
      .map((f) => f.name)
      .filter((k) => !['id', 'created', 'updated', 'passwordHash', 'tokenKey'].includes(k))
      .slice(0, 4)
  )

  function display(value) {
    if (value === null || value === undefined) return '—'
    if (typeof value === 'object') return truncate(JSON.stringify(value), 40)
    return truncate(String(value), 40)
  }

  const labelOverrides = {
    email: 'Correo electrónico',
    password: 'Contraseña',
    titulo: 'Título',
    nombre: 'Nombre',
    contenido: 'Contenido',
    descripcion: 'Descripción',
    precio: 'Precio',
    stock: 'Stock',
    hecha: '¿Está hecha?',
    prioridad: 'Prioridad',
    publicado: 'Publicado',
    status: 'Estado',
    autor: 'Autor',
    tags: 'Etiquetas',
    fecha: 'Fecha',
    url: 'URL',
    notas: 'Notas',
    mensaje: 'Mensaje',
  }

  function fieldLabel(name) {
    if (labelOverrides[name]) return labelOverrides[name]
    const words = String(name || '')
      .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
      .replace(/[_-]+/g, ' ')
      .trim()
    return words ? words.charAt(0).toUpperCase() + words.slice(1) : String(name)
  }

  function inputKind(field) {
    if (field.type === 'password') return 'password'
    if (field.type === 'bool') return 'bool'
    if (field.type === 'number') return 'number'
    if (field.type === 'array') return 'array'
    if (field.type === 'object' || field.type === 'any') return 'json'
    if (/contenido|descripcion|detalle|notas|comentarios|mensaje|texto|body|resumen/i.test(field.name) || (field.maxLen || 0) > 120) return 'textarea'
    return 'text'
  }

  function defaultFor(field) {
    if (field.type === 'bool') return false
    if (field.type === 'object' || field.type === 'any') return '{}'
    return ''
  }

  const formFields = $derived.by(() => {
    const fields = (detail?.fields || [])
      .filter((field) => !hiddenRecordFields.includes(field.name))
      .map((field) => ({ ...field, label: fieldLabel(field.name) }))
    // En una colección de usuarios, la contraseña se solicita sola al crear.
    if (detail?.type === 'auth' && recordMode === 'create') {
      fields.unshift({ name: 'password', type: 'password', required: true, label: 'Contraseña' })
    }
    return fields
  })

  function fieldId(name) {
    return `rec-${String(name).replace(/[^A-Za-z0-9_-]/g, '_')}`
  }

  function fieldPlaceholder(field) {
    if (field.type === 'password') return 'Al menos 8 caracteres'
    if (field.type === 'number') return '0'
    if (field.type === 'array') return 'Uno, dos, tres'
    if (field.type === 'object' || field.type === 'any') return '{"clave": "valor"}'
    return `Escribe ${field.label.toLowerCase()}`
  }

  function syncFormFromJson() {
    const parsed = parseJson(recordJson)
    const source = parsed.ok && isRecord(parsed.value) ? parsed.value : {}
    const values = {}
    for (const field of formFields) {
      const raw = source[field.name]
      if (raw === undefined || raw === null) {
        values[field.name] = defaultFor(field)
      } else if (field.type === 'array') {
        values[field.name] = Array.isArray(raw)
          ? raw.map((item) => (typeof item === 'object' ? JSON.stringify(item) : String(item))).join(', ')
          : String(raw)
      } else if (field.type === 'object' || field.type === 'any') {
        values[field.name] = JSON.stringify(raw, null, 2)
      } else if (field.type === 'bool') {
        values[field.name] = Boolean(raw)
      } else {
        values[field.name] = String(raw)
      }
    }
    recordValues = values
    recordBase = copy(source)
    recordErrors = {}
  }

  function syncJsonFromForm() {
    const body = copy(recordBase || {})
    const errors = {}
    for (const field of formFields) {
      const value = recordValues[field.name]
      const empty = value === '' || value === undefined || value === null
      if (field.required && empty && field.type !== 'bool') {
        errors[field.name] = field.type === 'password' ? 'Escribe una contraseña.' : 'Este campo es obligatorio.'
        continue
      }
      if (empty) {
        delete body[field.name]
        continue
      }
      switch (field.type) {
        case 'bool':
          body[field.name] = Boolean(value)
          break
        case 'number': {
          const number = Number(value)
          if (!Number.isFinite(number)) errors[field.name] = 'Escribe un número válido.'
          else body[field.name] = number
          break
        }
        case 'array':
          body[field.name] = String(value)
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean)
            .map((item) =>
              /^(true|false)$/i.test(item)
                ? /^true$/i.test(item)
                : Number.isFinite(Number(item))
                  ? Number(item)
                  : item
            )
          break
        case 'object':
        case 'any': {
          const parsed = parseJson(String(value))
          if (!parsed.ok) errors[field.name] = `El valor no es válido: ${parsed.error}`
          else body[field.name] = parsed.value
          break
        }
        case 'password':
          body.password = String(value)
          break
        default: {
          const text = String(value)
          if (field.maxLen && [...text].length > field.maxLen) {
            errors[field.name] = `Máximo ${field.maxLen} caracteres.`
          } else {
            body[field.name] = text
          }
        }
      }
    }
    recordErrors = errors
    if (Object.keys(errors).length) return false
    for (const key of ['id', 'created', 'updated', 'passwordHash', 'tokenKey']) delete body[key]
    recordJson = JSON.stringify(body, null, 2)
    return true
  }

  function setRecordEditor(mode) {
    if (mode === recordEditor) return
    if (mode === 'json') syncJsonFromForm()
    else syncFormFromJson()
    recordEditor = mode
  }

  async function loadCollections() {
    listError = ''
    try {
      const data = await api('/api/admin/collections', { key: 'collections-list' })
      collections = data?.items || []
      if (!selected && collections.length) selected = collections[0].name
      if (selected && !collections.some((c) => c.name === selected)) {
        selected = collections[0]?.name || ''
        detail = null
      }
      if (selected) await loadSelected()
      else detail = null
    } catch (error) {
      listError = error instanceof Error ? error.message : String(error)
    }
  }

  // El servidor exige: empieza por letra, letras/números/_ y 64 caracteres como máximo.
  function slugify(raw) {
    let value = String(raw || '')
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '')
      .toLowerCase()
      .replace(/[^a-z0-9_]+/g, '_')
      .replace(/^_+|_+$/g, '')
      .replace(/_+/g, '_')
      .slice(0, 64)
    if (value && !/^[a-z]/.test(value)) value = `col_${value}`.slice(0, 64)
    return value
  }

  function openCreate() {
    createStep = 1
    createTemplate = 'blog'
    newCollection = { name: slugify(templates[0].sampleName), public: false }
    createdName = ''
    createOpen = true
  }

  function chooseTemplate(template) {
    const previous = templates.find((t) => t.id === createTemplate)
    createTemplate = template.id
    if (!newCollection.name || newCollection.name === slugify(previous?.sampleName || '')) {
      newCollection = { ...newCollection, name: slugify(template.sampleName) }
    }
  }

  function accessRules(template, isPublic) {
    if (isPublic) return Object.fromEntries(ruleOps.map((op) => [op, '']))
    const rules = {}
    for (const op of ruleOps) {
      // El registro debe seguir abierto para poder crear cuentas.
      if (template.type === 'auth' && op === 'create') {
        rules[op] = ''
        continue
      }
      // En colecciones de usuarios, cada persona solo ve y edita su propio registro.
      rules[op] =
        template.type === 'auth'
          ? 'request.auth != null && resource.id == request.auth.id'
          : 'request.auth != null'
    }
    return rules
  }

  async function createCollectionFromForm() {
    const name = slugify(newCollection.name)
    newCollection = { ...newCollection, name }
    if (!name) {
      notify('Escribe un nombre para la colección.', 'error')
      return
    }
    creating = true
    try {
      const template = activeTemplate
      await api('/api/admin/collections', {
        method: 'POST',
        body: {
          name,
          type: template.type,
          fields: template.fields.map((field) => ({
            name: field.name,
            type: field.type,
            ...(field.required ? { required: true } : {}),
          })),
          indexes: [],
          rules: accessRules(template, newCollection.public),
        },
      })
      createdName = name
      createStep = 3
      selected = name
      detail = null
      records = []
      await loadCollections()
      notify(`Colección "${name}" creada.`, 'success')
    } catch (error) {
      notify(error instanceof Error ? error.message : String(error), 'error')
    } finally {
      creating = false
    }
  }

  async function addFirstDocument() {
    const sample = activeTemplate.sample
    createOpen = false
    tab = 'records'
    if (!detail || detail.name !== selected) await loadSelected()
    openRecord(null, sample)
  }

  async function loadSelected() {
    if (!selected) return
    loading = true
    detailError = ''
    try {
      const d = await api(`/api/admin/collections/${encodeURIComponent(selected)}`, { key: `col-detail-${selected}` })
      detail = d
      if (!batchRows[0].collection) batchRows[0].collection = selected
      await Promise.allSettled([loadRecords(), loadViews()])
    } catch (error) {
      detailError = error instanceof Error ? error.message : String(error)
      detail = null
    } finally {
      loading = false
    }
  }

  function recordParams(withCursor = false) {
    return {
      filter: query.filter || undefined,
      sort: query.sort || undefined,
      limit: Number(query.limit) || 25,
      fields: query.fields || undefined,
      include: query.include || undefined,
      view: query.view || undefined,
      cursor: withCursor ? nextCursor || undefined : undefined,
    }
  }

  async function loadRecords(withCursor = false, append = false) {
    if (!selected) return
    recordsLoading = true
    recordsError = ''
    try {
      const path = buildURL(`/api/admin/collections/${encodeURIComponent(selected)}/records`, recordParams(withCursor))
      const data = await api(path, { key: `records-${selected}` })
      const items = data?.items || []
      records = append ? [...records, ...items] : items
      nextCursor = data?.nextCursor || ''
    } catch (error) {
      if (error?.code !== 'aborted') recordsError = error instanceof Error ? error.message : String(error)
    } finally {
      recordsLoading = false
    }
  }

  async function loadViews() {
    viewsError = ''
    try {
      const data = await api(`/api/collections/${encodeURIComponent(selected)}/views`, { key: `views-${selected}` })
      views = data?.views || []
    } catch (error) {
      views = []
      viewsError = error instanceof Error ? error.message : String(error)
    }
  }

  function applyQuery() {
    cursorStack = []
    nextCursor = ''
    loadRecords()
  }

  function nextPage() {
    if (!nextCursor) return
    cursorStack = [...cursorStack, nextCursor]
    loadRecords(true)
  }

  function prevPage() {
    if (!cursorStack.length) return
    cursorStack = cursorStack.slice(0, -1)
    const previous = cursorStack[cursorStack.length - 1] || ''
    nextCursor = previous
    loadRecords(Boolean(previous))
  }

  function openSchema() {
    if (!detail) return
    schemaDraft = copy(detail)
    schemaDraft.fields = schemaDraft.fields || []
    schemaDraft.indexes = schemaDraft.indexes || []
    schemaDraft.cascade = schemaDraft.cascade || []
    schemaDraft.views = schemaDraft.views || []
    ruleConfig = ruleOps.map((op) => ({
      op,
      enabled: Object.prototype.hasOwnProperty.call(detail.rules || {}, op),
      expr: detail.rules?.[op] || '',
    }))
    schemaOpen = true
  }

  async function saveSchema() {
    if (!schemaDraft || !selected) return
    savingSchema = true
    try {
      const rules = {}
      for (const rule of ruleConfig) if (rule.enabled) rules[rule.op] = rule.expr
      const cleanFields = schemaDraft.fields
        .filter((f) => f.name?.trim())
        .map((f) => ({
          name: f.name.trim(),
          type: f.type || 'any',
          ...(f.required ? { required: true } : {}),
          ...(f.maxLen ? { maxLen: Number(f.maxLen) } : {}),
        }))
      const cleanIndexes = schemaDraft.indexes
        .filter((i) => i.name?.trim() && i.field?.trim())
        .map((i) => ({
          name: i.name.trim(),
          field: i.field.trim(),
          ...(i.unique ? { unique: true } : {}),
          ...(i.fulltext ? { fulltext: true } : {}),
          ...(i.state ? { state: i.state } : {}),
        }))
      const cleanCascade = schemaDraft.cascade
        .filter((c) => c.collection?.trim() && c.field?.trim())
        .map((c) => ({ collection: c.collection.trim(), field: c.field.trim() }))
      const cleanViews = schemaDraft.views
        .filter((v) => v.name?.trim())
        .map((v) => ({ name: v.name.trim(), filter: v.filter || '', sort: v.sort || '', ...(v.shared ? { shared: true } : {}) }))
      await api(`/api/admin/collections/${encodeURIComponent(selected)}`, {
        method: 'PATCH',
        body: { fields: cleanFields, indexes: cleanIndexes, rules, cascade: cleanCascade, views: cleanViews },
      })
      schemaOpen = false
      notify('Esquema guardado. Los índices pendientes se reconstruyen en el servidor.', 'success')
      await loadCollections()
    } catch (error) {
      notify(error instanceof Error ? error.message : String(error), 'error')
    } finally {
      savingSchema = false
    }
  }

  async function reindex(indexName = '') {
    try {
      const suffix = indexName ? `?index=${encodeURIComponent(indexName)}` : ''
      await api(`/api/admin/collections/${encodeURIComponent(selected)}/reindex${suffix}`, { method: 'POST' })
      notify(indexName ? `Índice ${indexName} reconstruido` : 'Índices reconstruidos', 'success')
      await loadSelected()
    } catch (error) {
      notify(error instanceof Error ? error.message : String(error), 'error')
    }
  }

  async function deleteCollection() {
    const name = selected
    await api(`/api/admin/collections/${encodeURIComponent(name)}?confirm=${encodeURIComponent(name)}`, { method: 'DELETE' })
    selected = ''
    detail = null
    records = []
    notify('Colección eliminada', 'success')
    await loadCollections()
  }

  function openRecord(record = null, sample = null) {
    recordMode = record ? 'edit' : 'create'
    recordId = record?.id || ''
    recordHttp = 'PATCH'
    recordEditor = 'form'
    const clean = copy(record || sample || {})
    for (const key of ['id', 'created', 'updated', 'passwordHash', 'tokenKey']) delete clean[key]
    recordJson = JSON.stringify(clean, null, 2)
    syncFormFromJson()
    recordOpen = true
  }

  async function saveRecord() {
    if (recordEditor === 'form' && !syncJsonFromForm()) {
      notify('Revisa los campos marcados antes de guardar.', 'error')
      return
    }
    const parsed = parseJson(recordJson)
    if (!parsed.ok) {
      notify(`JSON inválido: ${parsed.error}`, 'error')
      return
    }
    if (!isRecord(parsed.value)) {
      notify('El documento debe ser un objeto JSON', 'error')
      return
    }
    const body = copy(parsed.value)
    for (const key of ['id', 'created', 'updated', 'passwordHash', 'tokenKey']) delete body[key]
    savingRecord = true
    try {
      const path = `/api/collections/${encodeURIComponent(selected)}/records`
      if (recordMode === 'create') {
        await api(path, { method: 'POST', body })
        notify('Documento creado', 'success')
      } else {
        await api(`${path}/${encodeURIComponent(recordId)}`, { method: recordHttp, body })
        notify(recordHttp === 'PUT' ? 'Documento reemplazado' : 'Documento actualizado', 'success')
      }
      recordOpen = false
      await Promise.all([loadRecords(), loadCollections()])
    } catch (error) {
      notify(error instanceof Error ? error.message : String(error), 'error')
    } finally {
      savingRecord = false
    }
  }

  async function deleteRecord() {
    const id = deleteRecordTarget?.id
    if (!id) return
    await api(`/api/collections/${encodeURIComponent(selected)}/records/${encodeURIComponent(id)}`, { method: 'DELETE' })
    notify('Documento eliminado', 'success')
    await Promise.all([loadRecords(), loadCollections()])
  }

  async function runAggregate() {
    aggError = ''
    aggResult = null
    try {
      const body = { op: agg.op, filter: agg.filter || '' }
      if (agg.op === 'sum') body.field = agg.field
      aggResult = await api(`/api/collections/${encodeURIComponent(selected)}/aggregate`, { method: 'POST', body })
    } catch (error) {
      aggError = error instanceof Error ? error.message : String(error)
    }
  }

  async function runBatch() {
    batchError = ''
    batchResult = null
    const operations = []
    for (const [index, row] of batchRows.entries()) {
      const method = String(row.method || '').toUpperCase()
      if (!['POST', 'PATCH', 'PUT', 'DELETE'].includes(method)) {
        batchError = `Operación ${index + 1}: método no admitido`
        return
      }
      const op = { method, collection: row.collection || selected }
      if (row.id?.trim()) op.id = row.id.trim()
      if (method !== 'DELETE') {
        const parsed = parseJson(row.body || '{}')
        if (!parsed.ok || !isRecord(parsed.value)) {
          batchError = `Operación ${index + 1}: el cuerpo debe ser un objeto JSON válido`
          return
        }
        op.record = parsed.value
      }
      operations.push(op)
    }
    batchBusy = true
    try {
      const body = useTransaction ? { batches: [{ operations }] } : { operations }
      const path = useTransaction ? '/api/transactions' : '/api/batch'
      batchResult = await api(path, { method: 'POST', body })
      notify(useTransaction ? 'Transacción ejecutada' : 'Lote ejecutado', 'success')
      await Promise.all([loadRecords(), loadCollections()])
    } catch (error) {
      batchError = error instanceof Error ? error.message : String(error)
    } finally {
      batchBusy = false
    }
  }

  async function createView() {
    try {
      await api(`/api/collections/${encodeURIComponent(selected)}/views`, {
        method: 'POST',
        body: {
          name: newView.name.trim(),
          filter: newView.filter || '',
          sort: newView.sort || '',
          shared: Boolean(newView.shared),
        },
      })
      viewOpen = false
      newView = { name: '', filter: '', sort: '', shared: false }
      notify('Vista guardada', 'success')
      await loadViews()
      await loadCollections()
    } catch (error) {
      notify(error instanceof Error ? error.message : String(error), 'error')
    }
  }

  async function deleteView(name) {
    await api(`/api/collections/${encodeURIComponent(selected)}/views/${encodeURIComponent(name)}`, { method: 'DELETE' })
    if (query.view === name) query.view = ''
    notify('Vista eliminada', 'success')
    await loadViews()
    await loadRecords()
  }

  onMount(loadCollections)
</script>

<div class="stack">
  <div class="page-head">
    <div class="page-title">
      <h1>Colecciones</h1>
      <p class="page-subtitle">Organiza tus datos en colecciones y añade registros cuando quieras.</p>
    </div>
    <div class="page-actions">
      <button type="button" class="btn" onclick={loadCollections} disabled={loading}>
        <Icon name="refresh" size={14} /> Recargar
      </button>
    </div>
  </div>

  <div class="split">
    <aside class="card side-panel">
      <div class="row mb-2">
        <h2>Colecciones</h2>
        <span class="spacer"></span>
        <span class="badge">{collections.length}</span>
      </div>
      <button
        type="button"
        class="btn btn-sm btn-primary btn-block"
        onclick={openCreate}
        title="Te guiamos paso a paso con plantillas ya listas"
      >
        <Icon name="plus" size={14} /> Crear colección
      </button>
      <p class="tiny muted mt-1 mb-2">Una colección es como una carpeta para tus datos.</p>
      <div class="search-input mb-2">
        <Icon name="search" size={14} />
        <input class="input" type="search" placeholder="Filtrar colecciones…" bind:value={search} aria-label="Filtrar colecciones" />
      </div>
      <div class="stack list-stack">
        {#each filtered as col (col.name)}
          <button
            type="button"
            class="btn btn-ghost"
            class:btn-subtle={selected === col.name}
            onclick={async () => { selected = col.name; query.view = ''; cursorStack = []; await loadSelected() }}
          >
            <Icon name={col.type === 'auth' ? 'users' : 'database'} size={14} />
            <span class="truncate">{col.name}</span>
            <span class="spacer"></span>
            <span class="badge">{col.documentCount}</span>
          </button>
        {:else}
          <StateBlock kind="empty" title="Sin colecciones" detail="Todavía no hay colecciones definidas." />
        {/each}
      </div>
      {#if listError}
        <StateBlock kind="error" title="Error al listar" detail={listError} onRetry={loadCollections} />
      {/if}
    </aside>

    <section class="card detail-panel">
      {#if loading && !detail}
        <StateBlock kind="loading" title="Cargando colección…" />
      {:else if detailError && !detail}
        <StateBlock kind="error" title="No se pudo cargar la colección" detail={detailError} onRetry={loadSelected} />
      {:else if !detail}
        <div class="welcome stack">
          <div class="welcome-icon" aria-hidden="true"><Icon name="layers" size={28} /></div>
          <h2>{collections.length ? 'Selecciona una colección' : 'Empecemos con tu primera colección'}</h2>
          <p class="muted">
            {collections.length
              ? 'Elige una del panel izquierdo para ver sus registros, o crea otra nueva.'
              : 'Elige una plantilla, ponle un nombre y ya podrás añadir tus primeros datos. No necesitas saber nada de esquemas ni reglas.'}
          </p>
          <button type="button" class="btn btn-primary" onclick={openCreate}>
            <Icon name="plus" size={15} /> {collections.length ? 'Crear otra colección' : 'Crear mi primera colección'}
          </button>
        </div>
      {:else}
        <div class="page-head">
          <div class="page-title">
            <h2>{detail.name}</h2>
            <div class="badge-row">
              <span class="badge badge-info">{detail.type === 'auth' ? 'Auth' : 'Base'}</span>
              <span class="badge">{formatNumber(detail.documentCount)} documentos</span>
              {#each detail.indexes || [] as idx (idx.name)}
                <span class="badge {idx.state === 'ready' ? 'badge-ok' : 'badge-warn'}">
                  {idx.name}: {idx.state || 'building'}
                </span>
              {/each}
            </div>
          </div>
          <div class="page-actions">
            <button type="button" class="btn btn-sm" onclick={openSchema}><Icon name="edit" size={14} /> Editar esquema</button>
            <button type="button" class="btn btn-sm" onclick={() => reindex()}><Icon name="refresh" size={14} /> Reindexar todo</button>
            <button type="button" class="btn btn-sm btn-danger" onclick={() => (deleteCollectionOpen = true)}>
              <Icon name="trash" size={14} /> Eliminar
            </button>
          </div>
        </div>

        <div class="tabs" role="tablist" aria-label="Secciones de la colección">
          {#each [['records', 'Registros'], ['schema', 'Esquema e índices'], ['views', 'Vistas'], ['aggregate', 'Agregados'], ['batch', 'Lote y transacciones']] as [id, label] (id)}
            <button type="button" role="tab" class="tab" class:active={tab === id} aria-selected={tab === id} onclick={() => (tab = id)}>{label}</button>
          {/each}
        </div>

        <div class="mt-3">
          {#if tab === 'records'}
            <div class="stack">
              <div class="card card-body stack">
                <div class="row">
                  <div class="search-input">
                    <Icon name="search" size={14} />
                    <input class="input" type="search" placeholder='Filtro CEL, p. ej. status = "activo"' bind:value={query.filter} aria-label="Filtro CEL" onkeydown={(e) => e.key === 'Enter' && applyQuery()} />
                  </div>
                  <input class="input" type="text" placeholder="Orden (ej. -created)" bind:value={query.sort} aria-label="Orden" onkeydown={(e) => e.key === 'Enter' && applyQuery()} />
                  <input class="input" type="number" min="1" max="500" bind:value={query.limit} aria-label="Límite" />
                </div>
                <div class="row">
                  <input class="input" type="text" placeholder="Proyección: id,title,status" bind:value={query.fields} aria-label="Campos proyectados" />
                  <input class="input" type="text" placeholder="Joins: autor,autor.empresa" bind:value={query.include} aria-label="Campos incluidos" />
                  <select class="select" bind:value={query.view} aria-label="Vista guardada">
                    <option value="">Sin vista guardada</option>
                    {#each views as view (view.name)}<option value={view.name}>{view.name}</option>{/each}
                  </select>
                </div>
                <div class="row">
                  <button type="button" class="btn btn-primary btn-sm" onclick={applyQuery}>Aplicar consulta</button>
                  <button type="button" class="btn btn-sm" onclick={() => openRecord()}>Nuevo documento</button>
                  <span class="spacer"></span>
                  <span class="muted tiny">{records.length} registros en esta página</span>
                </div>
              </div>

              {#if recordsError}
                <StateBlock kind="error" title="No se pudieron cargar los registros" detail={recordsError} onRetry={() => loadRecords()} />
              {:else if recordsLoading}
                <StateBlock kind="loading" title="Cargando registros…" />
              {:else if !records.length}
                <StateBlock kind="empty" title="Sin documentos" detail="No hay documentos que coincidan con la consulta." />
              {:else}
                <div class="table-wrap">
                  <table>
                    <thead>
                      <tr><th>ID</th>{#each recordKeys as key (key)}<th>{key}</th>{/each}<th>Actualizado</th><th>Acciones</th></tr>
                    </thead>
                    <tbody>
                      {#each records as record (record.id)}
                        <tr>
                          <td class="id-pill" title={record.id}>{truncate(record.id, 12)}</td>
                          {#each recordKeys as key (key)}<td class="truncate">{display(record[key])}</td>{/each}
                          <td class="nowrap tiny muted">{formatDate(record.updated || record.created)}</td>
                          <td>
                            <div class="row-actions">
                              <button type="button" class="btn btn-xs" onclick={() => openRecord(record)}>Editar</button>
                              <button type="button" class="btn btn-xs btn-danger" onclick={() => { deleteRecordTarget = record; deleteRecordOpen = true }}>Borrar</button>
                            </div>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}
              <div class="row">
                <button type="button" class="btn btn-sm" onclick={prevPage} disabled={cursorStack.length === 0}>← Anterior</button>
                <button type="button" class="btn btn-sm" onclick={nextPage} disabled={!nextCursor}>Siguiente →</button>
                <span class="muted tiny">Cursor: {nextCursor ? truncate(nextCursor, 18) : 'fin de la lista'}</span>
              </div>
            </div>

          {:else if tab === 'schema'}
            <div class="stack">
              <div class="grid grid-2">
                <div class="card card-body">
                  <h3>Campos declarados</h3>
                  {#if (detail.fields || []).length}
                    <div class="kv-list mt-2">
                      {#each detail.fields as field (field.name)}
                        <div class="kv-row">
                          <span><code>{field.name}</code> <span class="muted">· {field.type}</span></span>
                          <span class="badge-row">
                            {#if field.required}<span class="badge badge-warn">requerido</span>{/if}
                            {#if field.maxLen}<span class="badge">max {field.maxLen}</span>{/if}
                          </span>
                        </div>
                      {/each}
                    </div>
                  {:else}
                    <p class="muted mt-2">Sin campos declarados: la colección acepta cualquier JSON.</p>
                  {/if}
                </div>
                <div class="card card-body">
                  <h3>Borrado en cascada</h3>
                  {#if (detail.cascade || []).length}
                    <div class="kv-list mt-2">
                      {#each detail.cascade as item (item.collection + item.field)}
                        <div class="kv-row"><code>{item.collection}.{item.field}</code></div>
                      {/each}
                    </div>
                  {:else}
                    <p class="muted mt-2">Sin reglas de cascada.</p>
                  {/if}
                </div>
              </div>
              <div class="card card-body">
                <div class="row">
                  <h3>Índices</h3>
                  <span class="spacer"></span>
                  <button type="button" class="btn btn-sm" onclick={openSchema}>Gestionar índices</button>
                </div>
                {#if (detail.indexes || []).length}
                  <div class="table-wrap mt-2">
                    <table>
                      <thead><tr><th>Nombre</th><th>Campo</th><th>Opciones</th><th>Estado</th><th>Acción</th></tr></thead>
                      <tbody>
                        {#each detail.indexes as idx (idx.name)}
                          <tr>
                            <td class="mono">{idx.name}</td>
                            <td class="mono">{idx.field}</td>
                            <td><span class="badge-row">{#if idx.unique}<span class="badge">único</span>{/if}{#if idx.fulltext}<span class="badge">fulltext</span>{/if}</span></td>
                            <td><span class="badge {idx.state === 'ready' ? 'badge-ok' : 'badge-warn'}">{idx.state || 'building'}</span></td>
                            <td><button type="button" class="btn btn-xs" onclick={() => reindex(idx.name)}>Reconstruir</button></td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                {:else}
                  <p class="muted mt-2">Sin índices secundarios.</p>
                {/if}
              </div>
              <div class="card card-body">
                <h3>Reglas CEL por operación</h3>
                <div class="kv-list mt-2">
                  {#each ruleOps as op (op)}
                    <div class="kv-row">
                      <span class="kv-key">{op}</span>
                      <code class="truncate">{Object.prototype.hasOwnProperty.call(detail.rules || {}, op) ? detail.rules[op] || '(público)' : '(bloqueado: solo superusuarios)'}</code>
                    </div>
                  {/each}
                </div>
              </div>
            </div>

          {:else if tab === 'views'}
            <div class="stack">
              <div class="row">
                <h2>Vistas guardadas</h2>
                <span class="spacer"></span>
                <button type="button" class="btn btn-sm btn-primary" onclick={() => (viewOpen = true)}>Nueva vista</button>
              </div>
              {#if viewsError}<StateBlock kind="error" title="No se pudieron cargar las vistas" detail={viewsError} onRetry={loadViews} />
              {:else if !views.length}<StateBlock kind="empty" title="Sin vistas guardadas" detail="Guarda un filtro y orden para reutilizarlos desde el explorador." />
              {:else}
                <div class="table-wrap">
                  <table>
                    <thead><tr><th>Nombre</th><th>Filtro</th><th>Orden</th><th>Ámbito</th><th>Acciones</th></tr></thead>
                    <tbody>
                      {#each views as view (view.name)}
                        <tr>
                          <td class="mono">{view.name}</td>
                          <td class="mono tiny truncate">{view.filter || '—'}</td>
                          <td class="mono tiny">{view.sort || '—'}</td>
                          <td>{view.shared ? 'Compartida' : 'Privada'}</td>
                          <td>
                            <div class="row-actions">
                              <button type="button" class="btn btn-xs" onclick={() => { query.view = view.name; tab = 'records'; applyQuery() }}>Usar</button>
                              <button type="button" class="btn btn-xs btn-danger" onclick={() => deleteView(view.name)}>Eliminar</button>
                            </div>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}
            </div>

          {:else if tab === 'aggregate'}
            <div class="stack">
              <h2>Panel de agregación</h2>
              <p class="muted">Los resultados proceden de POST /api/collections/{detail.name}/aggregate.</p>
              <div class="row">
                <div class="field">
                  <span class="field-label">Operación</span>
                  <select class="select" bind:value={agg.op}>
                    <option value="count">count</option>
                    <option value="sum">sum</option>
                  </select>
                </div>
                <div class="field">
                  <span class="field-label">Campo (para sum)</span>
                  <input class="input" type="text" bind:value={agg.field} placeholder="views, total…" />
                </div>
                <div class="field">
                  <span class="field-label">Filtro CEL</span>
                  <input class="input" type="text" bind:value={agg.filter} placeholder='status = "activo"' />
                </div>
                <button type="button" class="btn btn-primary" onclick={runAggregate}><Icon name="play" size={14} /> Calcular</button>
              </div>
              {#if aggError}<div class="callout callout-danger mt-2">{aggError}</div>{/if}
              {#if aggResult}
                <div class="metric-card mt-2">
                  <span class="metric-label">Resultado</span>
                  <span class="metric-value mono">{formatNumber(aggResult.result)}</span>
                  <span class="metric-note">{aggResult.op}{aggResult.field ? ` · ${aggResult.field}` : ''}{aggResult.filter ? ` · ${aggResult.filter}` : ''}</span>
                </div>
              {/if}
            </div>

          {:else if tab === 'batch'}
            <div class="stack">
              <div class="page-head">
                <div class="page-title">
                  <h2>Operaciones por lote</h2>
                  <p class="page-subtitle">Hasta 100 operaciones atómicas en /api/batch o /api/transactions.</p>
                </div>
                <label class="checkbox-row"><input type="checkbox" bind:checked={useTransaction} /> Ejecutar como transacción</label>
              </div>
              {#each batchRows as row, index (index)}
                <div class="card card-body stack">
                  <div class="row">
                    <span class="badge">#{index + 1}</span>
                    <select class="select" bind:value={row.method} aria-label="Método de la operación">
                      <option value="POST">POST (crear)</option>
                      <option value="PATCH">PATCH (fusionar)</option>
                      <option value="PUT">PUT (reemplazar)</option>
                      <option value="DELETE">DELETE (borrar)</option>
                    </select>
                    <input class="input" type="text" bind:value={row.collection} placeholder="colección" aria-label="Colección" />
                    <input class="input" type="text" bind:value={row.id} placeholder="id (para PATCH/PUT/DELETE)" aria-label="ID del documento" />
                    <span class="spacer"></span>
                    <button type="button" class="btn btn-xs btn-danger" onclick={() => (batchRows = batchRows.filter((_, i) => i !== index))} disabled={batchRows.length === 1}>Quitar</button>
                  </div>
                  {#if row.method !== 'DELETE'}
                    <JsonEditor label={`Registro ${index + 1}`} bind:value={row.body} rows={5} />
                  {/if}
                </div>
              {/each}
              <div class="row">
                <button type="button" class="btn btn-sm" onclick={() => (batchRows = [...batchRows, { method: 'POST', collection: selected, id: '', body: '{}' }])}>Añadir operación</button>
                <button type="button" class="btn btn-sm btn-primary" onclick={runBatch} disabled={batchBusy}>
                  {#if batchBusy}<span class="spinner" aria-hidden="true"></span>{/if}
                  {useTransaction ? 'Ejecutar transacción' : 'Ejecutar lote'}
                </button>
              </div>
              {#if batchError}<div class="callout callout-danger">{batchError}</div>{/if}
              {#if batchResult}
                <div class="card card-body">
                  <h3>Resultado</h3>
                  <pre class="code-input mt-2">{JSON.stringify(batchResult, null, 2)}</pre>
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </section>
  </div>
</div>

<Modal bind:open={schemaOpen} title={`Editar esquema: ${selected}`} subtitle="Campos, índices, reglas, cascada y vistas" wide>
  {#if schemaDraft}
    <form class="form-stack" onsubmit={(e) => { e.preventDefault(); saveSchema() }}>
      <div class="card card-body stack">
        <div class="row"><h3>Campos</h3><button type="button" class="btn btn-sm" onclick={() => (schemaDraft.fields = [...schemaDraft.fields, { name: '', type: 'string', required: false, maxLen: 0 }])}>Añadir campo</button></div>
        {#each schemaDraft.fields as field, index (index)}
          <div class="row">
            <input class="input" type="text" bind:value={field.name} placeholder="nombre" aria-label="Nombre del campo" />
            <select class="select" bind:value={field.type} aria-label="Tipo del campo">
              {#each fieldTypes as type (type)}<option value={type}>{type}</option>{/each}
            </select>
            <label class="checkbox-row"><input type="checkbox" bind:checked={field.required} /> requerido</label>
            <input class="input" type="number" min="0" bind:value={field.maxLen} placeholder="maxLen" aria-label="Longitud máxima" />
            <button type="button" class="btn btn-xs btn-danger" onclick={() => (schemaDraft.fields = schemaDraft.fields.filter((_, i) => i !== index))}>Quitar</button>
          </div>
        {/each}
      </div>

      <div class="card card-body stack">
        <div class="row"><h3>Índices</h3><button type="button" class="btn btn-sm" onclick={() => (schemaDraft.indexes = [...schemaDraft.indexes, { name: '', field: '', unique: false, fulltext: false, state: 'building' }])}>Añadir índice</button></div>
        <p class="muted tiny">Al guardar, el servidor construye los índices en estado <code>building</code>. También puedes reconstruirlos individualmente desde la pestaña Esquema.</p>
        {#each schemaDraft.indexes as idx, index (index)}
          <div class="row">
            <input class="input" type="text" bind:value={idx.name} placeholder="nombre_indice" aria-label="Nombre del índice" />
            <input class="input" type="text" bind:value={idx.field} placeholder="campo" aria-label="Campo indexado" />
            <label class="checkbox-row"><input type="checkbox" bind:checked={idx.unique} /> único</label>
            <label class="checkbox-row"><input type="checkbox" bind:checked={idx.fulltext} /> fulltext</label>
            <button type="button" class="btn btn-xs btn-danger" onclick={() => (schemaDraft.indexes = schemaDraft.indexes.filter((_, i) => i !== index))}>Quitar</button>
          </div>
        {/each}
      </div>

      <div class="card card-body stack">
        <h3>Reglas CEL</h3>
        <p class="muted tiny">Una operación sin configurar queda bloqueada para usuarios normales (solo superusuarios). Una regla vacía la hace pública.</p>
        {#each ruleConfig as rule (rule.op)}
          <div class="row">
            <label class="checkbox-row"><input type="checkbox" bind:checked={rule.enabled} /> {rule.op}</label>
            <input class="input mono" type="text" bind:value={rule.expr} disabled={!rule.enabled} placeholder="p. ej. request.auth != null" aria-label={`Regla ${rule.op}`} />
          </div>
        {/each}
      </div>

      <div class="card card-body stack">
        <div class="row"><h3>Borrado en cascada</h3><button type="button" class="btn btn-sm" onclick={() => (schemaDraft.cascade = [...schemaDraft.cascade, { collection: '', field: '' }])}>Añadir cascada</button></div>
        {#each schemaDraft.cascade as item, index (index)}
          <div class="row">
            <input class="input" type="text" bind:value={item.collection} placeholder="colección hija" aria-label="Colección hija" />
            <input class="input" type="text" bind:value={item.field} placeholder="campo de referencia" aria-label="Campo de referencia" />
            <button type="button" class="btn btn-xs btn-danger" onclick={() => (schemaDraft.cascade = schemaDraft.cascade.filter((_, i) => i !== index))}>Quitar</button>
          </div>
        {/each}
      </div>

      <div class="card card-body stack">
        <div class="row"><h3>Vistas guardadas (definición)</h3><button type="button" class="btn btn-sm" onclick={() => (schemaDraft.views = [...schemaDraft.views, { name: '', filter: '', sort: '', shared: false }])}>Añadir vista</button></div>
        {#each schemaDraft.views as view, index (index)}
          <div class="row">
            <input class="input" type="text" bind:value={view.name} placeholder="nombre" aria-label="Nombre de la vista" />
            <input class="input" type="text" bind:value={view.filter} placeholder="filtro CEL" aria-label="Filtro de la vista" />
            <input class="input" type="text" bind:value={view.sort} placeholder="orden" aria-label="Orden de la vista" />
            <label class="checkbox-row"><input type="checkbox" bind:checked={view.shared} /> compartida</label>
            <button type="button" class="btn btn-xs btn-danger" onclick={() => (schemaDraft.views = schemaDraft.views.filter((_, i) => i !== index))}>Quitar</button>
          </div>
        {/each}
      </div>

      <div class="row">
        <button type="button" class="btn" onclick={() => (schemaOpen = false)}>Cancelar</button>
        <button type="submit" class="btn btn-primary" disabled={savingSchema}>
          {#if savingSchema}<span class="spinner" aria-hidden="true"></span>{/if}
          Guardar esquema
        </button>
      </div>
    </form>
  {/if}
</Modal>

<Modal bind:open={recordOpen} title={recordMode === 'create' ? 'Nuevo documento' : 'Editar documento'} subtitle={recordMode === 'edit' ? `ID ${recordId}` : `Rellena los datos de ${selected}`} wide>
  <form class="form-stack" onsubmit={(event) => { event.preventDefault(); saveRecord() }}>
    <div class="editor-switch" role="group" aria-label="Modo de edición">
      <button type="button" class="btn btn-sm" class:btn-primary={recordEditor === 'form'} onclick={() => setRecordEditor('form')}>
        <Icon name="list" size={14} /> Rellenar campos
      </button>
      <button type="button" class="btn btn-sm" class:btn-primary={recordEditor === 'json'} onclick={() => setRecordEditor('json')}>
        <Icon name="code" size={14} /> Avanzado (JSON)
      </button>
    </div>

    {#if recordEditor === 'form'}
      {#if formFields.length}
        <p class="muted tiny">Rellena los campos. Los marcados con <span class="required-mark">*</span> son obligatorios.</p>
        <div class="form-grid">
          {#each formFields as field (field.name)}
            {@const kind = inputKind(field)}
            <div class="field" class:field-full={kind === 'textarea' || kind === 'json' || kind === 'array'}>
              <label class="field-label" for={fieldId(field.name)}>
                {field.label}
                {#if field.required}<span class="required-mark" aria-label="obligatorio">*</span>{/if}
              </label>
              {#if kind === 'bool'}
                <label class="checkbox-row" for={fieldId(field.name)}>
                  <input id={fieldId(field.name)} type="checkbox" bind:checked={recordValues[field.name]} />
                  <span>Sí</span>
                </label>
              {:else if kind === 'textarea'}
                <textarea id={fieldId(field.name)} class="textarea" rows={5} bind:value={recordValues[field.name]} placeholder={fieldPlaceholder(field)}></textarea>
              {:else if kind === 'json'}
                <textarea id={fieldId(field.name)} class="textarea code-input" rows={4} bind:value={recordValues[field.name]} placeholder={fieldPlaceholder(field)}></textarea>
                <span class="field-hint">Escribe un valor JSON, por ejemplo {fieldPlaceholder(field)}</span>
              {:else}
                <input
                  id={fieldId(field.name)}
                  class="input"
                  type={kind === 'password' ? 'password' : kind === 'number' ? 'number' : 'text'}
                  step={kind === 'number' ? 'any' : undefined}
                  autocomplete={kind === 'password' ? 'new-password' : undefined}
                  bind:value={recordValues[field.name]}
                  placeholder={fieldPlaceholder(field)}
                />
                {#if kind === 'array'}
                  <span class="field-hint">Separa los valores con comas.</span>
                {:else if field.type === 'file'}
                  <span class="field-hint">Escribe el nombre del archivo que quieres asociar.</span>
                {/if}
              {/if}
              {#if recordErrors[field.name]}
                <p class="field-error">{recordErrors[field.name]}</p>
              {/if}
            </div>
          {/each}
        </div>
        {#if recordMode === 'edit'}
          <div class="field">
            <span class="field-label">Al guardar</span>
            <div class="row">
              <label class="checkbox-row"><input type="radio" name="record-http" value="PATCH" bind:group={recordHttp} /> Solo actualizar estos campos</label>
              <label class="checkbox-row"><input type="radio" name="record-http" value="PUT" bind:group={recordHttp} /> Reemplazar el documento completo</label>
            </div>
          </div>
        {/if}
      {:else}
        <div class="callout callout-info">
          <span aria-hidden="true">ℹ</span>
          <span>Esta colección todavía no tiene campos. Añádelos desde el esquema para aparecer aquí, o usa el modo avanzado.</span>
        </div>
        <button type="button" class="btn" onclick={() => { recordOpen = false; tab = 'schema'; openSchema() }}>
          <Icon name="wrench" size={14} /> Añadir campos al esquema
        </button>
      {/if}
    {:else}
      <JsonEditor label="Contenido del documento" bind:value={recordJson} rows={12} />
    {/if}

    <div class="row">
      <button type="button" class="btn" onclick={() => (recordOpen = false)}>Cancelar</button>
      <span class="spacer"></span>
      <button type="submit" class="btn btn-primary" disabled={savingRecord}>
        {#if savingRecord}<span class="spinner" aria-hidden="true"></span>{/if}
        Guardar documento
      </button>
    </div>
  </form>
</Modal>

<Modal
  bind:open={createOpen}
  wide={createStep === 1}
  title={createStep === 3 ? '¡Colección creada!' : 'Nueva colección'}
  subtitle={createStep === 1 ? 'Paso 1 de 2 · Elige una plantilla' : createStep === 2 ? 'Paso 2 de 2 · Ponle un nombre' : 'Ya puedes empezar a añadir datos'}
>
  {#if createStep === 1}
    <div class="stack">
      <div class="callout callout-info">
        <span aria-hidden="true">ℹ</span>
        <span>Una colección agrupa datos parecidos. Elige la plantilla que más se parezca a lo que quieres guardar: después podrás cambiar los campos cuando quieras.</span>
      </div>
      <p class="field-label">¿Qué vas a guardar?</p>
      <div class="template-grid" role="group" aria-label="Plantillas de colección">
        {#each templates as template (template.id)}
          <button
            type="button"
            class="template-card"
            class:selected={createTemplate === template.id}
            aria-pressed={createTemplate === template.id}
            onclick={() => chooseTemplate(template)}
          >
            <span class="template-icon" aria-hidden="true"><Icon name={template.icon} size={18} /></span>
            <span class="template-copy">
              <strong>{template.label}</strong>
              <small class="muted">{template.hint}</small>
              {#if template.fields.length}
                <small class="muted template-fields">Incluye: {template.fields.map((f) => f.name).join(', ')}</small>
              {/if}
            </span>
          </button>
        {/each}
      </div>
      <div class="row">
        <span class="spacer"></span>
        <button type="button" class="btn btn-primary" onclick={() => (createStep = 2)}>
          Continuar <Icon name="chevron" size={14} />
        </button>
      </div>
    </div>
  {:else if createStep === 2}
    <form class="form-stack" onsubmit={(e) => { e.preventDefault(); createCollectionFromForm() }}>
      <div class="callout callout-info">
        <span aria-hidden="true">ℹ</span>
        <span>Solo necesitas un nombre. Nosotros preparamos los campos y los permisos para que empiece a funcionar.</span>
      </div>
      <div class="field">
        <label class="field-label" for="new-col-name">¿Cómo la vas a llamar?</label>
        <input
          id="new-col-name"
          class="input"
          value={newCollection.name}
          oninput={(event) => (newCollection = { ...newCollection, name: slugify(event.currentTarget.value) })}
          placeholder="articulos"
          autocomplete="off"
          required
        />
        <span class="field-hint">Escribe con libertad: convertimos espacios y acentos en un nombre válido.</span>
        {#if newCollection.name}
          <span class="badge badge-info">Nombre final: {newCollection.name}</span>
        {/if}
      </div>
      <div class="field">
        <span class="field-label">¿Quién puede ver estos datos desde tu aplicación?</span>
        <label class="choice-row">
          <input type="radio" name="create-access" bind:group={newCollection.public} value={false} />
          <span>
            <strong>Solo usuarios con sesión</strong>
            <small class="muted">Recomendado. Quien no haya iniciado sesión no podrá leer ni escribir.</small>
          </span>
        </label>
        <label class="choice-row">
          <input type="radio" name="create-access" bind:group={newCollection.public} value={true} />
          <span>
            <strong>Público</strong>
            <small class="muted">Cualquiera podrá leer y escribir. Úsalo solo con datos no sensibles.</small>
          </span>
        </label>
      </div>
      <div class="card card-body summary-box stack">
        <strong>Resumen</strong>
        <span class="tiny muted">Tipo: {activeTemplate.type === 'auth' ? 'Usuarios con acceso' : 'Datos normales'}</span>
        <span class="tiny muted">
          Campos iniciales: {activeTemplate.fields.length
            ? activeTemplate.fields.map((f) => f.name).join(', ')
            : 'ninguno (los añadirás después)'}
        </span>
      </div>
      <div class="row">
        <button type="button" class="btn" onclick={() => (createStep = 1)}>Atrás</button>
        <span class="spacer"></span>
        <button type="submit" class="btn btn-primary" disabled={creating || !newCollection.name}>
          {#if creating}<span class="spinner" aria-hidden="true"></span>{/if}
          Crear colección
        </button>
      </div>
    </form>
  {:else}
    <div class="stack success-stack">
      <div class="success-icon" aria-hidden="true"><Icon name="check" size={26} /></div>
      <h2>¡Listo!</h2>
      <p class="muted">
        Tu colección <strong>{createdName}</strong> ya existe y tiene sus campos iniciales.
      </p>
      <div class="row">
        <button type="button" class="btn btn-primary" onclick={addFirstDocument}>
          <Icon name="plus" size={15} /> Añadir mi primer documento
        </button>
        <button type="button" class="btn" onclick={() => (createOpen = false)}>Ver la colección</button>
      </div>
    </div>
  {/if}
</Modal>

<Modal bind:open={viewOpen} title="Nueva vista guardada" subtitle={selected}>
  <form class="form-stack" onsubmit={(e) => { e.preventDefault(); createView() }}>
    <div class="field"><label class="field-label" for="view-name">Nombre</label><input id="view-name" class="input" bind:value={newView.name} required /></div>
    <div class="field"><label class="field-label" for="view-filter">Filtro CEL</label><input id="view-filter" class="input" bind:value={newView.filter} /></div>
    <div class="field"><label class="field-label" for="view-sort">Orden</label><input id="view-sort" class="input" bind:value={newView.sort} /></div>
    <label class="checkbox-row"><input type="checkbox" bind:checked={newView.shared} /> Vista compartida</label>
    <div class="row"><button type="button" class="btn" onclick={() => (viewOpen = false)}>Cancelar</button><button type="submit" class="btn btn-primary">Guardar vista</button></div>
  </form>
</Modal>

<ConfirmDialog
  bind:open={deleteCollectionOpen}
  title="Eliminar colección"
  message={`Esta acción borrará "${selected}" y todos sus documentos de forma irreversible.`}
  confirmLabel="Eliminar colección"
  danger={true}
  requireText={selected}
  onConfirm={deleteCollection}
/>

<ConfirmDialog
  bind:open={deleteRecordOpen}
  title="Eliminar documento"
  message={`¿Eliminar definitivamente el documento ${deleteRecordTarget?.id || ''}?`}
  confirmLabel="Eliminar documento"
  danger={true}
  onConfirm={async () => { await deleteRecord(); deleteRecordTarget = null }}
  onCancel={() => { deleteRecordTarget = null; deleteRecordOpen = false }}
/>

<style>
  .list-stack { gap: 4px; }

  .editor-switch {
    display: inline-flex;
    align-self: flex-start;
    gap: 4px;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-subtle);
  }
  .editor-switch .btn { border-color: transparent; background: transparent; }
  .editor-switch .btn.btn-primary { background: var(--brand); border-color: var(--brand); color: #fff; }
  .field-full { grid-column: 1 / -1; }
  .required-mark { color: var(--danger); font-weight: 700; }

  .welcome {
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: 56px 24px;
    min-height: 320px;
  }
  .welcome h2 { font-size: 20px; }
  .welcome p { max-width: 520px; line-height: 1.6; }
  .welcome-icon {
    display: grid;
    place-items: center;
    width: 56px;
    height: 56px;
    border-radius: 16px;
    background: var(--brand-soft);
    color: var(--brand);
  }

  .template-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }
  .template-card {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 14px;
    text-align: left;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    color: var(--text);
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s, box-shadow 0.15s;
  }
  .template-card:hover { border-color: var(--brand-border); background: var(--bg-subtle); }
  .template-card.selected {
    border-color: var(--brand);
    background: var(--brand-soft);
    box-shadow: inset 0 0 0 1px var(--brand);
  }
  .template-card:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .template-icon {
    flex: 0 0 auto;
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border-radius: 9px;
    background: var(--bg-subtle);
    color: var(--text-muted);
  }
  .template-card.selected .template-icon { background: var(--bg-elevated); color: var(--brand); }
  .template-copy { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .template-copy strong { font-size: 13px; }
  .template-copy small { font-size: 11px; line-height: 1.4; }
  .template-fields { opacity: 0.85; }

  .choice-row {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    cursor: pointer;
  }
  .choice-row:hover { border-color: var(--brand-border); }
  .choice-row input { margin-top: 3px; }
  .choice-row span { display: flex; flex-direction: column; gap: 2px; }
  .choice-row strong { font-size: 13px; }
  .choice-row small { font-size: 11px; line-height: 1.4; }

  .summary-box { gap: 6px; background: var(--bg-subtle); }
  .success-stack { align-items: center; text-align: center; padding: 20px 0 8px; }
  .success-icon {
    display: grid;
    place-items: center;
    width: 56px;
    height: 56px;
    border-radius: 50%;
    background: var(--success-soft);
    color: var(--success);
  }
  .success-stack p { max-width: 420px; line-height: 1.6; }

  @media (max-width: 640px) {
    .template-grid { grid-template-columns: minmax(0, 1fr); }
  }
</style>

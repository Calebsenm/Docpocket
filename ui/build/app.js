"use strict";

const state = {
  token: localStorage.getItem("dp.token") || "",
  refresh: localStorage.getItem("dp.refresh") || "",
  email: localStorage.getItem("dp.email") || "",
  collections: [],
  selected: "",
  detail: null,
  records: [],
  recordCursor: "", // cursor de paginación del explorador
  recordHasMore: false,
  recordFilter: "",
  recordSort: "",
  editId: "",
  editingCollection: "", // nombre de la colección que se está editando ("" = nueva)
  userMode: "create", // create | password
  userId: "",
  userCollection: "",
  logs: [], // registro de peticiones en memoria
};

const $ = (s) => document.querySelector(s);
const $$ = (s) => document.querySelectorAll(s);

function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null) continue;
    if (k === "class") node.className = v;
    else if (k === "dataset") Object.assign(node.dataset, v);
    else if (k.startsWith("on") && typeof v === "function") node.addEventListener(k.slice(2), v);
    else node.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c === undefined || c === null) continue;
    node.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return node;
}

function toast(message, isError = false) {
  const box = $("#toast");
  box.textContent = (isError ? "⚠️ " : "✓ ") + message;
  box.classList.toggle("error", isError);
  box.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => { box.hidden = true; }, 4000);
}

// ---- registro de peticiones (logs en memoria) ----
function logRequest(method, path, status, ms, note) {
  state.logs.unshift({ at: new Date(), method, path, status, ms, note });
  if (state.logs.length > 200) state.logs.pop();
}

function saveSession(s) {
  state.token = s.token || "";
  state.refresh = s.refreshToken || "";
  if (s.record && s.record.email) state.email = s.record.email;
  localStorage.setItem("dp.token", state.token);
  localStorage.setItem("dp.refresh", state.refresh);
  localStorage.setItem("dp.email", state.email);
}
function clearSession() {
  state.token = state.refresh = state.email = "";
  localStorage.removeItem("dp.token");
  localStorage.removeItem("dp.refresh");
  localStorage.removeItem("dp.email");
}
async function refreshSession() {
  if (!state.refresh) return false;
  try {
    const res = await fetch("/api/admin/auth-refresh", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: state.refresh }),
    });
    if (!res.ok) return false;
    saveSession(await res.json());
    return true;
  } catch { return false; }
}

async function api(path, options = {}) {
  const started = performance.now();
  const opts = { ...options, headers: { ...(options.headers || {}) } };
  if (state.token) opts.headers.Authorization = "Bearer " + state.token;
  if (opts.body && typeof opts.body === "object" && !(opts.body instanceof FormData) && !(opts.body instanceof Blob)) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(opts.body);
  }
  let res = await fetch(path, opts);
  if (res.status === 401 && state.refresh && !options.retried) {
    if (await refreshSession()) return api(path, { ...options, retried: true });
  }
  const text = await res.text();
  let data = null;
  if (text) { try { data = JSON.parse(text); } catch { data = { error: { message: text } }; } }
  const ms = Math.round(performance.now() - started);
  logRequest(opts.method || "GET", path, res.status, ms);
  if (!res.ok) {
    const message = (data && data.error && data.error.message) || res.statusText || ("HTTP " + res.status);
    const error = new Error(message);
    error.status = res.status; error.data = data;
    throw error;
  }
  return data;
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return "0 B";
  const k = 1024, sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}
function formatUptime(s) {
  if (!s || s <= 0) return "< 1m";
  const m = Math.floor(s / 60), h = Math.floor(m / 60);
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m ${s % 60}s`;
}
function formatTimestamp(ts) {
  if (!ts) return "—";
  try { return new Date(ts).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }); }
  catch { return String(ts); }
}
function esc(s) { return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])); }

function showAuth(setup) {
  $("#auth-section").hidden = false; $("#dashboard").hidden = true;
  $("#user-badge").hidden = true; $("#logout").hidden = true;
  $("#setup-box").hidden = !setup; $("#login-box").hidden = setup;
  $("#auth-title").textContent = setup ? "Configuración Inicial" : "Bienvenido de nuevo";
}
function showDashboard() {
  $("#auth-section").hidden = true; $("#dashboard").hidden = false;
  $("#user-badge").hidden = false; $("#logout").hidden = false;
  $("#user-email").textContent = state.email || "superusuario";
}

async function boot() {
  bindEvents();
  try {
    const status = await fetch("/api/admin/setup").then((r) => r.json());
    if (!status.configured) { showAuth(true); return; }
    if (!state.token) { showAuth(false); return; }
    const me = await api("/api/admin/me");
    state.email = me.email || state.email;
    showDashboard();
    await loadCollections();
    await loadOverview();
  } catch { clearSession(); showAuth(false); }
}

function bindEvents() {
  $("#setup-form").addEventListener("submit", async (e) => {
    e.preventDefault(); const f = e.currentTarget;
    try {
      const res = await fetch("/api/admin/setup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email: f.email.value.trim(), password: f.password.value }) });
      if (!res.ok) { const d = await res.json().catch(() => null); throw new Error((d && d.error && d.error.message) || "No se pudo crear el superusuario"); }
      await login(f.email.value.trim(), f.password.value);
    } catch (err) { toast(err.message, true); }
  });
  $("#login-form").addEventListener("submit", (e) => { e.preventDefault(); const f = e.currentTarget; login(f.email.value.trim(), f.password.value); });
  $("#logout").addEventListener("click", async () => {
    const refreshToken = state.refresh; clearSession();
    state.collections = []; state.selected = ""; showAuth(false);
    if (refreshToken) { try { await fetch("/api/auth/logout", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ refreshToken }) }); } catch {} }
    toast("Sesión cerrada");
  });

  // Tabs
  $$(".nav-tab").forEach((tab) => tab.addEventListener("click", () => {
    $$(".nav-tab").forEach((t) => t.classList.toggle("active", t === tab));
    $$(".content-panel").forEach((p) => { p.hidden = p.id !== "tab-" + tab.dataset.tab; });
    const t = tab.dataset.tab;
    if (t === "maintenance") loadMetrics();
    if (t === "overview") loadOverview();
    if (t === "users") loadUsers();
    if (t === "storage") loadStorage();
    if (t === "logs") renderLogs();
  }));

  // Colecciones
  $("#btn-new-col").addEventListener("click", openNewCollection);
  $$(".modal-close").forEach((b) => b.addEventListener("click", (e) => { const id = e.currentTarget.dataset.close; if (id) $("#" + id).close(); }));
  $("#collection-search").addEventListener("input", (e) => {
    const term = e.target.value.toLowerCase().trim();
    $$(".collection-item-btn").forEach((b) => { b.parentElement.style.display = b.dataset.name.toLowerCase().includes(term) ? "" : "none"; });
  });
  $("#refresh-collections").addEventListener("click", () => loadCollections().catch((e) => toast(e.message, true)));
  $("#collection-form").addEventListener("submit", saveCollection);
  $("#add-field").addEventListener("click", () => addFieldRow());
  $("#col-type").addEventListener("change", (e) => {
    const rules = $("#col-rules");
    if (e.target.value === "auth") rules.value = "{}";
    else rules.value = '{\n  "list": "",\n  "view": "",\n  "create": "",\n  "update": "",\n  "delete": ""\n}';
  });

  // Reglas
  $("#rule-preset").addEventListener("change", applyRulePreset);
  $("#rule-form").addEventListener("submit", testRule);
  $$(".token-chip").forEach((c) => c.addEventListener("click", () => {
    const ta = $("#rule-expr"); const p = ta.selectionStart;
    ta.value = ta.value.slice(0, p) + c.dataset.token + ta.value.slice(ta.selectionEnd);
    ta.focus(); ta.selectionStart = ta.selectionEnd = p + c.dataset.token.length;
    updateHighlight();
  }));
  $("#rule-expr").addEventListener("input", updateHighlight);

  // Documentos
  $("#doc-editor-format").addEventListener("click", () => {
    const ed = $("#record-editor");
    try { ed.value = JSON.stringify(JSON.parse(ed.value), null, 2); } catch { toast("JSON con errores de sintaxis", true); }
  });
  $("#doc-save-btn").addEventListener("click", saveDocument);
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter" && $("#modal-doc-editor").open) saveDocument();
  });

  // Usuarios
  $("#auth-collection-select").addEventListener("change", () => loadUsers());
  $("#btn-new-user").addEventListener("click", () => openUserModal("create"));
  $("#user-form").addEventListener("submit", saveUser);

  // Archivos
  $("#storage-collection-select").addEventListener("change", () => loadStorage());
  $("#refresh-storage").addEventListener("click", () => loadStorage());

  // Logs
  $("#clear-log").addEventListener("click", () => { state.logs = []; renderLogs(); renderOverviewLog(); });
  $("#clear-log2").addEventListener("click", () => { state.logs = []; renderLogs(); renderOverviewLog(); });
  $("#log-filter").addEventListener("change", renderLogs);

  // Mantenimiento
  $("#run-check").addEventListener("click", runCheck);
  $("#download-backup").addEventListener("click", downloadBackup);
  $("#refresh-metrics").addEventListener("click", loadMetrics);
  $("#refresh-overview").addEventListener("click", loadOverview);
}

async function login(email, password) {
  try {
    const session = await fetch("/api/admin/auth-with-password", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, password }) }).then(async (res) => {
      const data = await res.json().catch(() => null);
      if (!res.ok) throw new Error((data && data.error && data.error.message) || "Credenciales incorrectas");
      return data;
    });
    saveSession(session);
    if (!state.email) state.email = email;
    localStorage.setItem("dp.email", state.email);
    $("#login-form").reset();
    showDashboard();
    await loadCollections();
    await loadOverview();
    toast("Bienvenido a DocPocket");
  } catch (err) { toast(err.message, true); }
}

// =================== COLECCIONES ===================
async function loadCollections() {
  const data = await api("/api/admin/collections");
  state.collections = data.items || [];
  renderCollectionList();
  fillSelectors();
  if (state.selected) {
    const exists = state.collections.some((c) => c.name === state.selected);
    if (exists) await openCollection(state.selected); else renderEmptyDetail();
  } else if (state.collections.length > 0) {
    await openCollection(state.collections[0].name);
  }
}

function fillSelectors() {
  const authCols = state.collections.filter((c) => c.type === "auth");
  const userSel = $("#auth-collection-select");
  userSel.replaceChildren(...authCols.map((c) => el("option", { value: c.name }, c.name)));
  if (!authCols.length) userSel.replaceChildren(el("option", { value: "" }, "No hay colecciones auth"));

  const allSel = $("#storage-collection-select");
  allSel.replaceChildren(...state.collections.map((c) => el("option", { value: c.name }, c.name)));
  if (!state.collections.length) allSel.replaceChildren(el("option", { value: "" }, "Sin colecciones"));
}

function renderCollectionList() {
  const list = $("#collection-list");
  list.replaceChildren();
  if (!state.collections.length) { list.append(el("li", { class: "muted p-2" }, "Sin colecciones creadas.")); return; }
  for (const col of state.collections) {
    const icon = col.type === "auth" ? "👤 " : "📄 ";
    const button = el("button", { type: "button", class: "collection-item-btn" + (col.name === state.selected ? " active" : ""), dataset: { name: col.name }, onclick: () => openCollection(col.name).catch((e) => toast(e.message, true)) },
      el("span", { class: "col-item-title" }, icon, col.name),
      el("span", { class: "col-badge" }, String(col.documentCount || 0))
    );
    list.append(el("li", {}, button));
  }
}

function renderEmptyDetail() {
  $("#collection-detail").replaceChildren(el("div", { class: "empty-state" },
    el("div", { class: "empty-icon" }, "📂"), el("h3", {}, "Selecciona una colección"),
    el("p", { class: "muted" }, "Elige una colección o crea una nueva.")));
}

async function openCollection(name, opts = {}) {
  state.selected = name;
  state.recordFilter = opts.filter ?? "";
  state.recordSort = opts.sort ?? "";
  state.recordCursor = "";
  renderCollectionList();
  await fetchCollectionData();
}

async function fetchCollectionData(append = false) {
  const name = state.selected;
  const params = new URLSearchParams({ limit: "50" });
  if (state.recordFilter) params.set("filter", state.recordFilter);
  if (state.recordSort) params.set("sort", state.recordSort);
  if (append && state.recordCursor) params.set("cursor", state.recordCursor);

  const [def, page] = await Promise.all([
    api("/api/admin/collections/" + encodeURIComponent(name)),
    api("/api/admin/collections/" + encodeURIComponent(name) + "/records?" + params.toString()),
  ]);
  state.detail = def;
  const items = page.items || [];
  state.records = append ? [...state.records, ...items] : items;
  state.recordCursor = page.nextCursor || "";
  state.recordHasMore = !!page.nextCursor;
  renderDetail();
}

function renderDetail() {
  const col = state.detail, detail = $("#collection-detail");
  detail.replaceChildren();
  if (!col) { renderEmptyDetail(); return; }

  const idxChips = (col.indexes || []).map((i) => el("span", { class: "chip " + (i.state === "ready" ? "chip-ok" : "chip-warn"), title: `campo: ${i.field}${i.fulltext ? " · fulltext" : ""}${i.unique ? " · único" : ""}` }, `idx: ${i.name} (${i.state || "building"})`));
  const titleGroup = el("div", { class: "detail-title-group" }, el("h2", {}, col.name),
    el("div", { class: "badge-row" }, el("span", { class: "chip" }, col.type === "auth" ? "👤 Auth" : "📄 Base"), el("span", { class: "chip" }, `${col.documentCount || 0} documentos`), ...idxChips));

  const actionGroup = el("div", { class: "row" },
    el("button", { class: "btn btn-sm", type: "button", onclick: () => openEditCollection(col) }, "Editar esquema"),
    el("button", { class: "btn btn-sm btn-ghost", type: "button", onclick: async () => { try { await api("/api/admin/collections/" + encodeURIComponent(col.name) + "/reindex", { method: "POST" }); toast("Índices reconstruidos"); await loadCollections(); } catch (e) { toast(e.message, true); } } }, "Reindexar"),
    el("button", { class: "btn btn-sm btn-danger", type: "button", onclick: async () => { if (!confirm(`¿Eliminar "${col.name}" y todos sus documentos de forma irreversible?`)) return; try { await api("/api/admin/collections/" + encodeURIComponent(col.name) + "?confirm=" + encodeURIComponent(col.name), { method: "DELETE" }); state.selected = ""; state.detail = null; state.records = []; renderEmptyDetail(); toast("Colección eliminada"); await loadCollections(); } catch (e) { toast(e.message, true); } } }, "Eliminar"));
  detail.append(el("div", { class: "detail-header-bar" }, titleGroup, actionGroup));

  // Secciones de esquema: campos, índices, cascada, vistas
  detail.append(renderSchemaSection(col));

  // Filtro + orden + nuevo
  const filterInput = el("input", { type: "search", placeholder: 'Filtrar (ej: status = "active" && views > 10)…', value: state.recordFilter });
  const sortInput = el("input", { type: "search", placeholder: "Orden (ej: -created, status)", value: state.recordSort, style: "max-width:190px" });
  const apply = () => openCollection(col.name, { filter: filterInput.value.trim(), sort: sortInput.value.trim() });
  filterInput.addEventListener("keydown", (e) => { if (e.key === "Enter") apply(); });
  sortInput.addEventListener("keydown", (e) => { if (e.key === "Enter") apply(); });
  detail.append(el("div", { class: "doc-filter-bar" }, filterInput, sortInput,
    el("button", { class: "btn btn-sm", type: "button", onclick: apply }, "Aplicar"),
    el("button", { class: "btn btn-primary btn-sm", type: "button", onclick: () => openDocEditor() }, "+ Nuevo Documento")));

  if (!state.records.length) {
    detail.append(el("div", { class: "empty-state" }, el("p", { class: "muted" }, state.recordFilter ? "No hay documentos que coincidan con el filtro." : "Esta colección aún no tiene documentos."),
      el("button", { class: "btn btn-primary btn-sm mt-2", onclick: () => openDocEditor() }, "+ Crear primer documento")));
  } else {
    const hidden = new Set(["id", "created", "updated", "passwordHash", "tokenKey"]);
    const declared = (col.fields || []).map((f) => f.name).filter((n) => !hidden.has(n));
    const sample = state.records[0] || {};
    const keys = declared.length ? declared.slice(0, 6) : Object.keys(sample).filter((k) => !hidden.has(k)).slice(0, 5);

    const thead = el("thead", {}, el("tr", {}, el("th", {}, "ID"), ...keys.map((k) => el("th", {}, k)), el("th", {}, "Actualizado"), el("th", {}, "Acciones")));
    const tbody = el("tbody");
    for (const rec of state.records) {
      const id = rec.id || "";
      const idPill = el("span", { class: "id-pill", title: "Clic para copiar ID", onclick: () => { navigator.clipboard.writeText(id); toast("ID copiado"); } }, id.substring(0, 10) + "…", "📋");
      const cells = keys.map((k) => { let d = rec[k] == null ? "—" : typeof rec[k] === "object" ? JSON.stringify(rec[k]) : String(rec[k]); if (d.length > 30) d = d.substring(0, 28) + "…"; return el("td", {}, d); });
      tbody.append(el("tr", {}, el("td", {}, idPill), ...cells, el("td", {}, formatTimestamp(rec.updated || rec.created)),
        el("td", {}, el("div", { class: "row-actions" }, el("button", { class: "btn btn-xs", onclick: () => openDocEditor(rec) }, "Editar"), el("button", { class: "btn btn-xs btn-danger", onclick: () => deleteRecord(id) }, "Borrar")))));
    }
    detail.append(el("div", { class: "table-responsive" }, el("table", {}, thead, tbody)));
    if (state.recordHasMore) detail.append(el("button", { class: "btn btn-sm mt-2", onclick: () => fetchCollectionData(true).catch((e) => toast(e.message, true)) }, "Cargar más →"));
  }
}

function renderSchemaSection(col) {
  const box = el("div", { class: "schema-grid" });
  const fields = (col.fields || []).filter((f) => !["email", "passwordHash", "verified", "tokenKey"].includes(f.name));
  box.append(el("div", { class: "schema-card" }, el("h4", {}, "Campos"), fields.length
    ? el("ul", { class: "schema-list" }, ...fields.map((f) => el("li", {}, el("code", {}, f.name), " · ", el("span", { class: "muted" }, f.type || "any"), f.required ? el("span", { class: "chip chip-warn" }, " req") : null)))
    : el("p", { class: "muted tiny" }, "Sin campos declarados (acepta cualquier JSON).")));
  box.append(el("div", { class: "schema-card" }, el("h4", {}, "Índices"), (col.indexes || []).length
    ? el("ul", { class: "schema-list" }, ...col.indexes.map((i) => el("li", {}, el("code", {}, i.name), " → ", i.field, i.fulltext ? el("span", { class: "chip chip-ok" }, " fulltext") : null, i.unique ? el("span", { class: "chip" }, " único") : null)))
    : el("p", { class: "muted tiny" }, "Sin índices.")));
  box.append(el("div", { class: "schema-card" }, el("h4", {}, "Borrado en cascada"), (col.cascade || []).length
    ? el("ul", { class: "schema-list" }, ...col.cascade.map((c) => el("li", {}, `${c.collection}.${c.field}`)))
    : el("p", { class: "muted tiny" }, "Sin cascadas.")));
  box.append(el("div", { class: "schema-card" }, el("h4", {}, "Vistas guardadas"), (col.views || []).length
    ? el("ul", { class: "schema-list" }, ...col.views.map((v) => el("li", {}, el("code", {}, v.name), " · ", el("span", { class: "muted" }, v.filter || "sin filtro"))))
    : el("p", { class: "muted tiny" }, "Sin vistas.")));
  return box;
}

// ---- diseñador visual de campos ----
let fieldRowCount = 0;
function addFieldRow(field = {}) {
  const row = el("div", { class: "field-row", "data-row": ++fieldRowCount });
  row.append(
    el("input", { type: "text", class: "field-name", placeholder: "nombre", value: field.name || "", "aria-label": "Nombre del campo" }),
    el("select", { class: "field-type", "aria-label": "Tipo" }, ...["string", "number", "bool", "array", "object", "file", "any"].map((t) => el("option", { value: t, selected: (field.type || "string") === t }, t))),
    el("label", { class: "field-req", title: "Campo requerido" }, el("input", { type: "checkbox", class: "field-required", checked: !!field.required }), " req"),
    el("input", { type: "number", class: "field-maxlen", placeholder: "maxLen", value: field.maxLen || "", "aria-label": "Longitud máxima", style: "max-width:90px" }),
    el("button", { type: "button", class: "btn btn-icon btn-xs btn-danger", title: "Quitar campo", onclick: () => row.remove() }, "✕")
  );
  $("#fields-list").append(row);
}

function openNewCollection() {
  state.editingCollection = "";
  $("#col-modal-title").textContent = "Nueva Colección";
  $("#col-submit").textContent = "Crear Colección";
  $("#collection-form").reset();
  $("#fields-list").replaceChildren();
  addFieldRow();
  $("#col-rules").value = '{\n  "list": "",\n  "view": "",\n  "create": "",\n  "update": "",\n  "delete": ""\n}';
  $("#col-indexes").value = "[]"; $("#col-cascade").value = "[]"; $("#col-views").value = "[]";
  $("#col-name").disabled = false;
  $("#modal-new-col").showModal();
  $("#col-name").focus();
}

function openEditCollection(col) {
  state.editingCollection = col.name;
  $("#col-modal-title").textContent = "Editar Colección: " + col.name;
  $("#col-submit").textContent = "Guardar cambios";
  $("#col-name").value = col.name;
  $("#col-name").disabled = true;
  $("#col-type").value = col.type || "base";
  $("#fields-list").replaceChildren();
  const fields = (col.fields || []).filter((f) => !["email", "passwordHash", "verified", "tokenKey"].includes(f.name));
  fields.forEach((f) => addFieldRow(f));
  if (!fields.length) addFieldRow();
  $("#col-rules").value = JSON.stringify(col.rules || {}, null, 2);
  $("#col-indexes").value = JSON.stringify(col.indexes || [], null, 2);
  $("#col-cascade").value = JSON.stringify(col.cascade || [], null, 2);
  $("#col-views").value = JSON.stringify(col.views || [], null, 2);
  $("#modal-new-col").showModal();
}

async function saveCollection(event) {
  event.preventDefault(); const f = event.currentTarget;
  let rules, indexes, cascade, views, fields;
  try {
    rules = JSON.parse(f.rules.value || "{}");
    indexes = JSON.parse(f.indexes.value || "[]");
    cascade = JSON.parse(f.cascade.value || "[]");
    views = JSON.parse(f.views.value || "[]");
  } catch { toast("La configuración avanzada no contiene JSON válido", true); return; }
  fields = [];
  $$("#fields-list .field-row").forEach((row) => {
    const name = row.querySelector(".field-name").value.trim();
    if (!name) return;
    const field = { name, type: row.querySelector(".field-type").value };
    if (row.querySelector(".field-required").checked) field.required = true;
    const ml = row.querySelector(".field-maxlen").value;
    if (ml) field.maxLen = parseInt(ml, 10);
    fields.push(field);
  });
  const def = { name: f.name.value.trim(), type: f.type.value, fields, rules, indexes, cascade, views };
  try {
    if (state.editingCollection) {
      await api("/api/admin/collections/" + encodeURIComponent(state.editingCollection), { method: "PATCH", body: def });
      toast("Colección actualizada");
    } else {
      await api("/api/admin/collections", { method: "POST", body: def });
      state.selected = def.name;
      toast("Colección creada");
    }
    $("#modal-new-col").close();
    await loadCollections();
    if (def.name) await openCollection(def.name);
  } catch (e) { toast(e.message, true); }
}

// ---- documentos ----
function openDocEditor(record = null) {
  const modal = $("#modal-doc-editor"), title = $("#doc-modal-title"), hint = $("#doc-editor-hint"), editor = $("#record-editor");
  if (record) {
    state.editId = record.id;
    title.textContent = "Editar Documento (" + record.id + ")";
    hint.textContent = "Modifica los campos del documento";
    const copy = { ...record };
    ["id", "created", "updated", "passwordHash", "tokenKey"].forEach((k) => delete copy[k]);
    editor.value = JSON.stringify(copy, null, 2);
  } else {
    state.editId = "";
    title.textContent = "Nuevo Documento";
    hint.textContent = "Ingresa un objeto JSON válido";
    editor.value = state.detail && state.detail.type === "auth"
      ? '{\n  "email": "usuario@ejemplo.com",\n  "password": "clave-segura-123",\n  "name": "Nombre"\n}'
      : '{\n  "title": "Nuevo registro",\n  "status": "active"\n}';
  }
  modal.showModal(); editor.focus();
}

async function saveDocument() {
  let payload;
  try { payload = JSON.parse($("#record-editor").value); } catch { toast("El payload no es un JSON válido", true); return; }
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) { toast("El documento debe ser un objeto JSON", true); return; }
  ["id", "created", "updated", "passwordHash", "tokenKey"].forEach((k) => delete payload[k]);
  const col = state.detail.name;
  try {
    if (state.editId) {
      await api("/api/collections/" + encodeURIComponent(col) + "/records/" + state.editId, { method: "PATCH", body: payload });
      toast("Documento actualizado");
    } else {
      await api("/api/collections/" + encodeURIComponent(col) + "/records", { method: "POST", body: payload });
      toast("Documento creado");
    }
    $("#modal-doc-editor").close(); state.editId = "";
    await fetchCollectionData();
  } catch (e) { toast(e.message, true); }
}

async function deleteRecord(id) {
  if (!confirm(`¿Eliminar definitivamente el documento ${id}?`)) return;
  try { await api("/api/collections/" + encodeURIComponent(state.detail.name) + "/records/" + id, { method: "DELETE" }); toast("Documento eliminado"); await fetchCollectionData(); }
  catch (e) { toast(e.message, true); }
}

// =================== USUARIOS ===================
async function loadUsers() {
  const sel = $("#auth-collection-select");
  const col = sel.value;
  const panel = $("#users-panel");
  if (!col) { panel.innerHTML = '<div class="empty-state"><p class="muted">No hay colecciones de tipo <code>auth</code>. Crea una desde Colecciones.</p></div>'; return; }
  try {
    const page = await api("/api/admin/collections/" + encodeURIComponent(col) + "/records?limit=200");
    const users = page.items || [];
    panel.replaceChildren(
      el("div", { class: "users-toolbar" }, el("span", { class: "muted" }, `${users.length} usuarios en `), el("code", {}, col)),
      users.length ? el("div", { class: "table-responsive" }, el("table", {},
        el("thead", {}, el("tr", {}, el("th", {}, "ID"), el("th", {}, "Email"), el("th", {}, "Nombre"), el("th", {}, "Verificado"), el("th", {}, "Creado"), el("th", {}, "Acciones"))),
        el("tbody", {}, ...users.map((u) => el("tr", {},
          el("td", {}, el("span", { class: "id-pill" }, String(u.id || "").substring(0, 10) + "…")),
          el("td", {}, u.email || "—"), el("td", {}, u.name || "—"),
          el("td", {}, u.verified ? el("span", { class: "chip chip-ok" }, "sí") : el("span", { class: "chip chip-warn" }, "no")),
          el("td", {}, formatTimestamp(u.created)),
          el("td", {}, el("div", { class: "row-actions" },
            el("button", { class: "btn btn-xs", onclick: () => openUserModal("password", u) }, "Contraseña"),
            el("button", { class: "btn btn-xs btn-danger", onclick: () => deleteUser(col, u) }, "Borrar")))))))
      ) : el("div", { class: "empty-state" }, el("p", { class: "muted" }, "Sin usuarios todavía."))
    );
  } catch (e) { panel.innerHTML = '<div class="empty-state"><p class="muted">No se pudieron cargar los usuarios: ' + esc(e.message) + '</p></div>'; }
}

function openUserModal(mode, user = null) {
  state.userMode = mode;
  state.userId = user ? user.id : "";
  state.userCollection = $("#auth-collection-select").value;
  $("#user-modal-title").textContent = mode === "create" ? "Nuevo Usuario" : "Cambiar Contraseña";
  $("#user-submit").textContent = mode === "create" ? "Crear usuario" : "Cambiar contraseña";
  $("#user-email-field").value = user ? (user.email || "") : "";
  $("#user-password-field").value = "";
  $("#user-name-field").value = "";
  $("#user-email-field").disabled = mode === "password";
  $("#user-email-group").hidden = false;
  $("#user-name-group").hidden = mode === "password";
  if (mode === "password") $("#user-email-field").value = user.email || user.id;
  $("#modal-user").showModal();
}

async function saveUser(e) {
  e.preventDefault();
  const col = state.userCollection;
  const password = $("#user-password-field").value;
  if (password.length < 8) { toast("La contraseña debe tener al menos 8 caracteres", true); return; }
  try {
    if (state.userMode === "create") {
      const email = $("#user-email-field").value.trim();
      const name = $("#user-name-field").value.trim();
      const body = { email, password };
      if (name) body.name = name;
      await api("/api/collections/" + encodeURIComponent(col) + "/auth-register", { method: "POST", body });
      toast("Usuario creado");
    } else {
      // Los superusuarios pueden cambiar cualquier contraseña sin la actual.
      await api("/api/collections/" + encodeURIComponent(col) + "/records/" + state.userId + "/password", { method: "PATCH", body: { password } });
      toast("Contraseña actualizada (sesiones invalidadas)");
    }
    $("#modal-user").close();
    await loadUsers();
  } catch (err) { toast(err.message, true); }
}

async function deleteUser(col, user) {
  if (!confirm(`¿Eliminar al usuario ${user.email || user.id}?`)) return;
  try { await api("/api/collections/" + encodeURIComponent(col) + "/records/" + user.id, { method: "DELETE" }); toast("Usuario eliminado"); await loadUsers(); }
  catch (e) { toast(e.message, true); }
}

// =================== ARCHIVOS ===================
async function loadStorage() {
  const col = $("#storage-collection-select").value;
  const panel = $("#storage-panel");
  if (!col) { panel.innerHTML = '<div class="empty-state"><p class="muted">Sin colecciones.</p></div>'; return; }
  try {
    const page = await api("/api/admin/collections/" + encodeURIComponent(col) + "/records?limit=200");
    const docs = page.items || [];
    const rows = [];
    for (const d of docs) {
      for (const [k, v] of Object.entries(d)) {
        if (typeof v === "string" && /\.[a-z0-9]{2,5}$/i.test(v) && !["email", "created", "updated", "id"].includes(k)) {
          rows.push({ id: d.id, field: k, name: v });
        }
      }
    }
    const toolbar = el("div", { class: "users-toolbar" },
      el("span", { class: "muted" }, `${rows.length} archivos referenciados en `), el("code", {}, col));
    let body;
    if (rows.length) {
      const thead = el("thead", {}, el("tr", {},
        el("th", {}, "Documento"), el("th", {}, "Campo"), el("th", {}, "Archivo"), el("th", {}, "Acción")));
      const tbody = el("tbody");
      for (const r of rows) {
        tbody.append(el("tr", {},
          el("td", {}, el("span", { class: "id-pill" }, String(r.id).substring(0, 10) + "…")),
          el("td", {}, el("code", {}, r.field)),
          el("td", {}, esc(r.name)),
          el("td", {}, el("button", { class: "btn btn-xs", onclick: () => downloadFile(col, r.id, r.name) }, "↓ Descargar"))));
      }
      body = el("div", { class: "table-responsive" }, el("table", {}, thead, tbody));
    } else {
      body = el("div", { class: "empty-state" }, el("p", { class: "muted" }, "No se detectaron campos de archivo en esta colección."));
    }
    panel.replaceChildren(toolbar, body);
  } catch (e) { panel.innerHTML = '<div class="empty-state"><p class="muted">' + esc(e.message) + '</p></div>'; }
}

async function downloadFile(col, id, name) {
  try {
    const res = await fetch(`/api/files/${encodeURIComponent(col)}/${encodeURIComponent(id)}/${encodeURIComponent(name)}`, { headers: { Authorization: "Bearer " + state.token } });
    if (!res.ok) throw new Error("No se pudo descargar el archivo");
    const blob = await res.blob(); const url = URL.createObjectURL(blob);
    const a = el("a", { href: url, download: name }); document.body.append(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
    logRequest("GET", `/api/files/${col}/${id}/${name}`, res.status, 0);
  } catch (e) { toast(e.message, true); }
}

// =================== REGLAS CEL ===================
function updateHighlight() {
  const src = $("#rule-expr").value;
  const tokens = src.split(/(\s+|&&|\|\||!=|==|>=|<=|[().,]|'[^']*'|"[^"]*")/g).filter((t) => t !== undefined);
  const html = tokens.map((t) => {
    if (/^\s+$/.test(t)) return esc(t);
    if (/^(request|resource|now|auth)$/.test(t)) return `<span class="hl-var">${esc(t)}</span>`;
    if (/^(true|false|null)$/.test(t)) return `<span class="hl-lit">${esc(t)}</span>`;
    if (/^['"]/.test(t)) return `<span class="hl-str">${esc(t)}</span>`;
    if (/^(&&|\|\||!=|==|>=|<=|[()])/.test(t)) return `<span class="hl-op">${esc(t)}</span>`;
    if (/^\d/.test(t)) return `<span class="hl-num">${esc(t)}</span>`;
    return esc(t);
  }).join("");
  const pre = $("#rule-highlight");
  pre.innerHTML = html || " ";
  const lines = src.split("\n").length;
  $("#rule-line-numbers").textContent = Array.from({ length: lines }, (_, i) => i + 1).join("\n");
}

function applyRulePreset(e) {
  const v = e.target.value, rule = $("#rule-expr"), ctx = $("#rule-ctx");
  if (v === "public") rule.value = "";
  else if (v === "auth") rule.value = "request.auth != null";
  else if (v === "owner") { rule.value = "request.auth != null && resource.owner == request.auth.id"; ctx.value = '{\n  "operation": "view",\n  "auth": { "id": "user_123" },\n  "resource": { "id": "doc_01", "owner": "user_123" },\n  "body": {},\n  "query": {}\n}'; }
  else if (v === "admin") { rule.value = "request.auth != null && request.auth.role == 'admin'"; ctx.value = '{\n  "operation": "delete",\n  "auth": { "id": "admin_01", "role": "admin" },\n  "resource": { "id": "doc_01" },\n  "body": {},\n  "query": {}\n}'; }
  else if (v === "verified") rule.value = "request.auth != null && request.auth.verified == true";
  updateHighlight();
}

async function testRule(e) {
  e.preventDefault(); const f = e.currentTarget;
  let context;
  try { context = JSON.parse(f.context.value || "{}"); } catch { toast("El contexto no es un JSON válido", true); return; }
  const payload = { rule: $("#rule-expr").value, operation: context.operation || "view", auth: context.auth || null, resource: context.resource || null, body: context.body || null, query: context.query || null };
  try {
    const result = await api("/api/admin/rules/test", { method: "POST", body: payload });
    $("#rule-result").textContent = JSON.stringify(result, null, 2);
    toast(result.allowed ? "Regla evaluada: PERMITIDA" : "Regla evaluada: DENEGADA", !result.allowed);
  } catch (err) { $("#rule-result").textContent = err.data ? JSON.stringify(err.data, null, 2) : err.message; toast(err.message, true); }
}

// =================== RESUMEN / MÉTRICAS ===================
async function loadOverview() {
  try {
    const [health, metricsText] = await Promise.all([
      api("/api/health"),
      fetch("/metrics").then((r) => r.text()).catch(() => ""),
    ]);
    const collections = state.collections || [];
    const totalDocs = collections.reduce((s, c) => s + (c.documentCount || 0), 0);
    const clientsMatch = metricsText.match(/docpocket_realtime_clients\s+(\d+)/);
    const cards = [
      { label: "Colecciones", value: String(collections.length), icon: "📚" },
      { label: "Documentos", value: String(totalDocs), icon: "📄" },
      { label: "Tamaño BD", value: formatBytes(health.dbSize), icon: "💾" },
      { label: "Tiempo activo", value: formatUptime(health.uptime), icon: "⏱️" },
      { label: "Clientes SSE", value: clientsMatch ? clientsMatch[1] : "0", icon: "🔌" },
      { label: "Estado", value: health.status === "ok" ? "OK" : "?", icon: "❤️" },
    ];
    $("#overview-cards").replaceChildren(...cards.map((c) => el("div", { class: "metric-item" }, el("span", { class: "metric-val" }, c.icon, " ", c.value), el("span", { class: "metric-lbl" }, c.label))));

    // Gráfico de barras
    const max = Math.max(1, ...collections.map((c) => c.documentCount || 0));
    const chart = $("#chart-collections");
    chart.replaceChildren(...(collections.length ? collections.slice().sort((a, b) => (b.documentCount || 0) - (a.documentCount || 0)).slice(0, 10).map((c) => el("div", { class: "bar-row" },
      el("span", { class: "bar-label", title: c.name }, c.name),
      el("div", { class: "bar-track" }, el("div", { class: "bar-fill", style: `width:${Math.max(2, ((c.documentCount || 0) / max) * 100)}%` })),
      el("span", { class: "bar-val" }, String(c.documentCount || 0)))) : [el("p", { class: "muted" }, "Sin colecciones.")]));

    // Salud
    let building = 0, ready = 0;
    collections.forEach((c) => (c.indexes || []).forEach((i) => i.state === "ready" ? ready++ : building++));
    const healthItems = [
      ["Motor Bbolt", "Activo"], ["Índices listos", String(ready)], ["Índices construyendo", String(building)],
      ["Integridad", "Usa 'Comprobar Integridad'"], ["Backup", "Disponible"], ["Tiempo real", clientsMatch ? clientsMatch[1] + " clientes" : "0 clientes"],
    ];
    $("#health-list").replaceChildren(...healthItems.map(([k, v]) => el("div", { class: "health-item" }, el("span", {}, k), el("span", { class: "muted" }, v))));
    renderOverviewLog();
  } catch (e) { toast(e.message, true); }
}

function renderOverviewLog() { const box = $("#overview-log"); if (box) box.replaceChildren(...renderLogRows(state.logs.slice(0, 8))); }

function renderLogRows(logs) {
  if (!logs.length) return [el("p", { class: "muted" }, "Sin peticiones registradas.")];
  return logs.map((l) => el("div", { class: "log-row " + (l.status >= 400 ? "log-error" : "log-ok") },
    el("span", { class: "log-time" }, l.at.toLocaleTimeString()),
    el("span", { class: "log-method method-" + (l.method || "get").toLowerCase() }, l.method || "GET"),
    el("span", { class: "log-path" }, esc(l.path.length > 70 ? l.path.substring(0, 70) + "…" : l.path)),
    el("span", { class: "log-status" }, String(l.status)),
    el("span", { class: "log-ms" }, l.ms + " ms")));
}

function renderLogs() {
  const filter = $("#log-filter").value;
  let logs = state.logs;
  if (filter === "ok") logs = logs.filter((l) => l.status < 400);
  if (filter === "error") logs = logs.filter((l) => l.status >= 400);
  $("#logs-panel").replaceChildren(el("div", { class: "log-list" }, ...renderLogRows(logs)));
  fetch("/metrics").then((r) => r.text()).then((t) => { const pre = $("#prometheus-raw"); if (pre) pre.textContent = t || "(vacío)"; }).catch(() => {});
}

async function loadMetrics() {
  try {
    const data = await api("/api/health");
    $("#metric-uptime").textContent = formatUptime(data.uptime);
    $("#metric-dbsize").textContent = formatBytes(data.dbSize);
    $("#metric-cols").textContent = String(data.collections || 0);
    const t = await fetch("/metrics").then((r) => r.text()).catch(() => "");
    const m = t.match(/docpocket_realtime_clients\s+(\d+)/);
    $("#metric-clients").textContent = m ? m[1] : "0";
  } catch { toast("No se pudieron cargar las métricas", true); }
}

async function runCheck() {
  $("#maintenance-output").textContent = "Ejecutando auditoría de integridad…";
  try {
    const report = await api("/api/admin/check", { method: "POST" });
    $("#maintenance-output").textContent = JSON.stringify(report, null, 2);
    const issues = report.issues || [];
    toast(issues.length ? `Se encontraron ${issues.length} problemas` : "Base de datos 100% consistente", issues.length > 0);
  } catch (e) { $("#maintenance-output").textContent = e.message; toast(e.message, true); }
}

async function downloadBackup() {
  try {
    toast("Generando copia de seguridad…");
    const res = await fetch("/api/admin/backup", { headers: { Authorization: "Bearer " + state.token } });
    if (!res.ok) throw new Error("No se pudo generar el backup");
    const blob = await res.blob(), url = URL.createObjectURL(blob);
    const a = el("a", { href: url, download: `docpocket-backup-${new Date().toISOString().slice(0, 10)}.db` });
    document.body.append(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
    logRequest("GET", "/api/admin/backup", res.status, 0);
    toast("Copia de seguridad descargada");
  } catch (e) { toast(e.message, true); }
}

updateHighlight();
boot();

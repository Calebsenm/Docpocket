"use strict";

const state = {
  token: localStorage.getItem("dp.token") || "",
  refresh: localStorage.getItem("dp.refresh") || "",
  email: localStorage.getItem("dp.email") || "",
  collections: [],
  selected: "",
  detail: null,
  records: [],
  editId: "",
  filterQuery: "",
};

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => document.querySelectorAll(selector);

function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (value === undefined || value === null) continue;
    if (key === "class") node.className = value;
    else if (key === "dataset") Object.assign(node.dataset, value);
    else if (key.startsWith("on") && typeof value === "function") node.addEventListener(key.slice(2), value);
    else node.setAttribute(key, value);
  }
  for (const child of children.flat()) {
    if (child === undefined || child === null) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
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

function saveSession(session) {
  state.token = session.token || "";
  state.refresh = session.refreshToken || "";
  if (session.record && session.record.email) state.email = session.record.email;
  localStorage.setItem("dp.token", state.token);
  localStorage.setItem("dp.refresh", state.refresh);
  localStorage.setItem("dp.email", state.email);
}

function clearSession() {
  state.token = "";
  state.refresh = "";
  state.email = "";
  localStorage.removeItem("dp.token");
  localStorage.removeItem("dp.refresh");
  localStorage.removeItem("dp.email");
}

async function refreshSession() {
  if (!state.refresh) return false;
  try {
    const res = await fetch("/api/admin/auth-refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: state.refresh }),
    });
    if (!res.ok) return false;
    saveSession(await res.json());
    return true;
  } catch {
    return false;
  }
}

async function api(path, options = {}) {
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
  if (text) {
    try { data = JSON.parse(text); } catch { data = { error: { message: text } }; }
  }
  if (!res.ok) {
    const message = (data && data.error && data.error.message) || res.statusText || ("HTTP " + res.status);
    const error = new Error(message);
    error.status = res.status;
    error.data = data;
    throw error;
  }
  return data;
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

function formatUptime(seconds) {
  if (!seconds || seconds <= 0) return "< 1m";
  const m = Math.floor(seconds / 60);
  const h = Math.floor(m / 60);
  if (h > 0) return `${h}h ${m % 60}m`;
  return `${m}m ${seconds % 60}s`;
}

function showAuth(setup) {
  $("#auth-section").hidden = false;
  $("#dashboard").hidden = true;
  $("#user-badge").hidden = true;
  $("#logout").hidden = true;
  $("#setup-box").hidden = !setup;
  $("#login-box").hidden = setup;
  $("#auth-title").textContent = setup ? "Configuración Inicial" : "Bienvenido de nuevo";
  if (setup) $("#setup-form input[name=email]").focus();
  else $("#login-form input[name=email]").focus();
}

function showDashboard() {
  $("#auth-section").hidden = true;
  $("#dashboard").hidden = false;
  $("#user-badge").hidden = false;
  $("#logout").hidden = false;
  $("#user-email").textContent = state.email || "superusuario";
}

async function boot() {
  bindEvents();
  try {
    const status = await fetch("/api/admin/setup").then((r) => r.json());
    if (!status.configured) {
      showAuth(true);
      return;
    }
    if (!state.token) {
      showAuth(false);
      return;
    }
    const me = await api("/api/admin/me");
    state.email = me.email || state.email;
    showDashboard();
    await loadCollections();
  } catch (err) {
    clearSession();
    showAuth(false);
  }
}

function bindEvents() {
  // Setup form
  $("#setup-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const email = form.email.value.trim();
    const password = form.password.value;
    try {
      await fetch("/api/admin/setup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      }).then(async (res) => {
        if (!res.ok) {
          const data = await res.json().catch(() => null);
          throw new Error((data && data.error && data.error.message) || "No se pudo crear el superusuario");
        }
      });
      await login(email, password);
    } catch (err) {
      toast(err.message, true);
    }
  });

  // Login form
  $("#login-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    await login(form.email.value.trim(), form.password.value);
  });

  // Logout
  $("#logout").addEventListener("click", async () => {
    const refreshToken = state.refresh;
    clearSession();
    state.collections = [];
    state.selected = "";
    showAuth(false);
    if (refreshToken) {
      try {
        await fetch("/api/auth/logout", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refreshToken }),
        });
      } catch {}
    }
    toast("Sesión cerrada");
  });

  // Navegación de Pestañas
  $$(".nav-tab").forEach((tab) => {
    tab.addEventListener("click", () => {
      $$(".nav-tab").forEach((t) => t.classList.toggle("active", t === tab));
      $("#tab-collections").hidden = tab.dataset.tab !== "collections";
      $("#tab-tools").hidden = tab.dataset.tab !== "tools";
      $("#tab-maintenance").hidden = tab.dataset.tab !== "maintenance";
      if (tab.dataset.tab === "maintenance") {
        loadMetrics();
      }
    });
  });

  // Modales
  $("#btn-new-col").addEventListener("click", () => {
    $("#modal-new-col").showModal();
    $("#col-name").focus();
  });

  $$(".modal-close").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const targetId = e.currentTarget.dataset.close;
      if (targetId) $(`#${targetId}`).close();
    });
  });

  // Filtrado de lista de colecciones
  $("#collection-search").addEventListener("input", (e) => {
    const term = e.target.value.toLowerCase().trim();
    $$(".collection-item-btn").forEach((btn) => {
      const name = btn.dataset.name.toLowerCase();
      btn.parentElement.style.display = name.includes(term) ? "" : "none";
    });
  });

  $("#refresh-collections").addEventListener("click", () => loadCollections().catch((err) => toast(err.message, true)));
  $("#collection-form").addEventListener("submit", createCollection);
  $("#col-type").addEventListener("change", (e) => {
    const rulesArea = $("#col-rules");
    if (e.target.value === "auth") {
      rulesArea.value = "{}";
    } else {
      rulesArea.value = '{\n  "list": "",\n  "view": "",\n  "create": "",\n  "update": "",\n  "delete": ""\n}';
    }
  });

  // Probador de reglas CEL y plantillas
  $("#rule-preset").addEventListener("change", (e) => {
    const val = e.target.value;
    const ruleArea = $("#rule-expr");
    const ctxArea = $("#rule-ctx");
    if (val === "public") {
      ruleArea.value = "";
    } else if (val === "auth") {
      ruleArea.value = "request.auth != null";
    } else if (val === "owner") {
      ruleArea.value = "request.auth != null && resource.owner == request.auth.id";
      ctxArea.value = JSON.stringify({
        operation: "view",
        auth: { id: "user_123" },
        resource: { id: "doc_01", owner: "user_123" },
        body: {},
        query: {},
      }, null, 2);
    } else if (val === "admin") {
      ruleArea.value = "request.auth != null && request.auth.role == 'admin'";
      ctxArea.value = JSON.stringify({
        operation: "delete",
        auth: { id: "admin_01", role: "admin" },
        resource: { id: "doc_01" },
        body: {},
        query: {},
      }, null, 2);
    }
  });

  $("#rule-form").addEventListener("submit", testRule);
  $("#run-check").addEventListener("click", runCheck);
  $("#download-backup").addEventListener("click", downloadBackup);
  $("#refresh-metrics").addEventListener("click", loadMetrics);

  // Formateador de JSON en el editor
  $("#doc-editor-format").addEventListener("click", () => {
    const editor = $("#record-editor");
    try {
      const parsed = JSON.parse(editor.value);
      editor.value = JSON.stringify(parsed, null, 2);
    } catch {
      toast("JSON con errores de sintaxis", true);
    }
  });

  $("#doc-save-btn").addEventListener("click", saveDocument);

  // Atajos de teclado: Ctrl+Enter / Cmd+Enter guarda el modal activo
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
      if ($("#modal-doc-editor").open) {
        saveDocument();
      }
    }
  });
}

async function login(email, password) {
  try {
    const session = await fetch("/api/admin/auth-with-password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    }).then(async (res) => {
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
    toast("Bienvenido a DocPocket");
  } catch (err) {
    toast(err.message, true);
  }
}

async function loadCollections() {
  const data = await api("/api/admin/collections");
  state.collections = data.items || [];
  renderCollectionList();
  if (state.selected) {
    const exists = state.collections.some((c) => c.name === state.selected);
    if (exists) await openCollection(state.selected);
    else renderEmptyDetail();
  } else if (state.collections.length > 0) {
    await openCollection(state.collections[0].name);
  }
}

function renderCollectionList() {
  const list = $("#collection-list");
  list.replaceChildren();
  if (!state.collections.length) {
    list.append(el("li", { class: "muted p-2" }, "Sin colecciones creadas."));
    return;
  }
  for (const col of state.collections) {
    const isAuth = col.type === "auth";
    const icon = isAuth ? "👤 " : "📄 ";
    const countBadge = el("span", { class: "col-badge" }, String(col.documentCount || 0));

    const button = el("button", {
      type: "button",
      class: "collection-item-btn" + (col.name === state.selected ? " active" : ""),
      dataset: { name: col.name },
      onclick: () => openCollection(col.name).catch((err) => toast(err.message, true)),
    },
      el("span", { class: "col-item-title" }, icon, col.name),
      countBadge
    );
    list.append(el("li", {}, button));
  }
}

function renderEmptyDetail() {
  const detail = $("#collection-detail");
  detail.replaceChildren(
    el("div", { class: "empty-state" },
      el("div", { class: "empty-icon" }, "📂"),
      el("h3", {}, "Selecciona una colección"),
      el("p", { class: "muted" }, "Selecciona una colección para explorar registros o crea una nueva.")
    )
  );
}

async function openCollection(name, filterQuery = "") {
  state.selected = name;
  state.filterQuery = filterQuery;
  renderCollectionList();

  const queryPart = filterQuery ? `&filter=${encodeURIComponent(filterQuery)}` : "";
  const [def, page] = await Promise.all([
    api("/api/admin/collections/" + encodeURIComponent(name)),
    api("/api/admin/collections/" + encodeURIComponent(name) + "/records?limit=50" + queryPart),
  ]);
  state.detail = def;
  state.records = page.items || [];
  renderDetail();
}

function renderDetail() {
  const col = state.detail;
  const detail = $("#collection-detail");
  detail.replaceChildren();
  if (!col) { renderEmptyDetail(); return; }

  // Header de la Colección
  const titleGroup = el("div", { class: "detail-title-group" },
    el("h2", {}, col.name),
    el("div", { class: "badge-row" },
      el("span", { class: "chip" }, (col.type === "auth" ? "👤 Auth" : "📄 Base")),
      el("span", { class: "chip" }, `${col.documentCount || 0} documentos`),
      ...(col.indexes || []).map((idx) =>
        el("span", { class: "chip " + (idx.state === "ready" ? "chip-ok" : "chip-warn") }, `idx: ${idx.name}`)
      )
    )
  );

  const actionGroup = el("div", { class: "row" },
    el("button", {
      class: "btn btn-sm btn-ghost",
      type: "button",
      onclick: async () => {
        try {
          await api("/api/admin/collections/" + encodeURIComponent(col.name) + "/reindex", { method: "POST" });
          toast("Índices reconstruidos con éxito");
          await loadCollections();
        } catch (err) { toast(err.message, true); }
      },
    }, "Reindexar"),
    el("button", {
      class: "btn btn-sm btn-danger",
      type: "button",
      onclick: async () => {
        if (!confirm(`¿Eliminar la colección "${col.name}" y todos sus documentos de forma irreversible?`)) return;
        try {
          await api("/api/admin/collections/" + encodeURIComponent(col.name) + "?confirm=" + encodeURIComponent(col.name), { method: "DELETE" });
          state.selected = "";
          state.detail = null;
          state.records = [];
          renderEmptyDetail();
          toast("Colección eliminada");
          await loadCollections();
        } catch (err) { toast(err.message, true); }
      },
    }, "Eliminar")
  );

  const headerBar = el("div", { class: "detail-header-bar" }, titleGroup, actionGroup);
  detail.append(headerBar);

  // Barra de Búsqueda y Botón Nuevo
  const searchInput = el("input", {
    type: "search",
    placeholder: 'Filtrar registros (ej: status = "active" && views > 10)…',
    value: state.filterQuery || "",
  });
  searchInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      openCollection(col.name, searchInput.value.trim());
    }
  });

  const filterBtn = el("button", {
    class: "btn btn-sm",
    type: "button",
    onclick: () => openCollection(col.name, searchInput.value.trim()),
  }, "Filtrar");

  const newDocBtn = el("button", {
    class: "btn btn-primary btn-sm",
    type: "button",
    onclick: () => openDocEditor(),
  }, "+ Nuevo Documento");

  const filterBar = el("div", { class: "doc-filter-bar" }, searchInput, filterBtn, newDocBtn);
  detail.append(filterBar);

  // Tabla de Registros
  if (!state.records.length) {
    detail.append(
      el("div", { class: "empty-state" },
        el("p", { class: "muted" }, state.filterQuery ? "No se encontraron documentos que coincidan con el filtro." : "Esta colección aún no tiene documentos."),
        el("button", { class: "btn btn-primary btn-sm mt-2", onclick: () => openDocEditor() }, "+ Crear primer documento")
      )
    );
  } else {
    // Detectar campos primarios para visualización limpia
    const sample = state.records[0] || {};
    const candidateKeys = Object.keys(sample).filter(
      (k) => !["id", "created", "updated", "passwordHash", "tokenKey"].includes(k)
    ).slice(0, 3);

    const thead = el("thead", {},
      el("tr", {},
        el("th", {}, "ID"),
        ...candidateKeys.map((k) => el("th", {}, k)),
        el("th", {}, "Actualizado"),
        el("th", {}, "Acciones")
      )
    );

    const tbody = el("tbody");
    for (const record of state.records) {
      const id = record.id || "";
      const idPill = el("span", {
        class: "id-pill",
        title: "Clic para copiar ID",
        onclick: () => {
          navigator.clipboard.writeText(id);
          toast("ID copiado al portapapeles");
        },
      }, id.substring(0, 10) + "…", "📋");

      const dynamicCells = candidateKeys.map((k) => {
        const val = record[k];
        let display = val === undefined || val === null ? "—" : typeof val === "object" ? JSON.stringify(val) : String(val);
        if (display.length > 35) display = display.substring(0, 32) + "…";
        return el("td", {}, display);
      });

      const actions = el("div", { class: "row-actions" },
        el("button", { class: "btn btn-xs", type: "button", onclick: () => openDocEditor(record) }, "Editar"),
        el("button", { class: "btn btn-xs btn-danger", type: "button", onclick: () => deleteRecord(id) }, "Borrar")
      );

      const row = el("tr", {},
        el("td", {}, idPill),
        ...dynamicCells,
        el("td", {}, formatTimestamp(record.updated || record.created)),
        el("td", {}, actions)
      );
      tbody.append(row);
    }

    const tableWrap = el("div", { class: "table-responsive" }, el("table", {}, thead, tbody));
    detail.append(tableWrap);
  }
}

function formatTimestamp(ts) {
  if (!ts) return "—";
  try {
    const d = new Date(ts);
    return d.toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  } catch {
    return String(ts);
  }
}

function openDocEditor(record = null) {
  const modal = $("#modal-doc-editor");
  const title = $("#doc-modal-title");
  const hint = $("#doc-editor-hint");
  const editor = $("#record-editor");

  if (record) {
    state.editId = record.id;
    title.textContent = "Editar Documento (" + record.id + ")";
    hint.textContent = "Modifica los campos del documento";
    const copy = { ...record };
    delete copy.id; delete copy.created; delete copy.updated;
    delete copy.passwordHash; delete copy.tokenKey;
    editor.value = JSON.stringify(copy, null, 2);
  } else {
    state.editId = "";
    title.textContent = "Nuevo Documento";
    hint.textContent = "Ingresa un objeto JSON válido";
    if (state.detail && state.detail.type === "auth") {
      editor.value = '{\n  "email": "usuario@ejemplo.com",\n  "password": "clave-segura-123",\n  "name": "Nombre"\n}';
    } else {
      editor.value = '{\n  "title": "Nuevo registro",\n  "status": "active"\n}';
    }
  }
  modal.showModal();
  editor.focus();
}

async function saveDocument() {
  const editor = $("#record-editor");
  let payload;
  try {
    payload = JSON.parse(editor.value);
  } catch {
    toast("El payload no es un JSON válido", true);
    return;
  }
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    toast("El documento debe ser un objeto JSON {...}", true);
    return;
  }
  delete payload.id; delete payload.created; delete payload.updated;
  delete payload.passwordHash; delete payload.tokenKey;

  const colName = state.detail.name;
  try {
    if (state.editId) {
      await api("/api/collections/" + encodeURIComponent(colName) + "/records/" + state.editId, {
        method: "PATCH",
        body: payload,
      });
      toast("Documento actualizado correctamente");
    } else {
      await api("/api/collections/" + encodeURIComponent(colName) + "/records", {
        method: "POST",
        body: payload,
      });
      toast("Documento creado exitosamente");
    }
    $("#modal-doc-editor").close();
    state.editId = "";
    await openCollection(colName, state.filterQuery);
  } catch (err) {
    toast(err.message, true);
  }
}

async function deleteRecord(id) {
  if (!confirm(`¿Eliminar definitivamente el documento ${id}?`)) return;
  try {
    await api("/api/collections/" + encodeURIComponent(state.detail.name) + "/records/" + id, { method: "DELETE" });
    toast("Documento eliminado");
    await openCollection(state.detail.name, state.filterQuery);
  } catch (err) { toast(err.message, true); }
}

async function createCollection(event) {
  event.preventDefault();
  const form = event.currentTarget;
  let rules, indexes;
  try {
    rules = JSON.parse(form.rules.value || "{}");
    indexes = JSON.parse(form.indexes.value || "[]");
  } catch {
    toast("Las reglas o los índices no contienen JSON válido", true);
    return;
  }
  try {
    const def = {
      name: form.name.value.trim(),
      type: form.type.value,
      rules,
      indexes,
    };
    await api("/api/admin/collections", { method: "POST", body: def });
    form.reset();
    $("#modal-new-col").close();
    toast("Colección creada");
    state.selected = def.name;
    await loadCollections();
    await openCollection(def.name);
  } catch (err) {
    toast(err.message, true);
  }
}

async function testRule(event) {
  event.preventDefault();
  const form = event.currentTarget;
  let context;
  try {
    context = JSON.parse(form.context.value || "{}");
  } catch {
    toast("El contexto no es un JSON válido", true);
    return;
  }
  const payload = {
    rule: form.rule.value,
    operation: context.operation || "view",
    auth: context.auth || null,
    resource: context.resource || null,
    body: context.body || null,
    query: context.query || null,
  };
  try {
    const result = await api("/api/admin/rules/test", { method: "POST", body: payload });
    $("#rule-result").textContent = JSON.stringify(result, null, 2);
    if (result.allowed) {
      toast("Regla evaluada: PERMITIDA");
    } else {
      toast("Regla evaluada: DENEGADA", true);
    }
  } catch (err) {
    $("#rule-result").textContent = err.data ? JSON.stringify(err.data, null, 2) : err.message;
    toast(err.message, true);
  }
}

async function loadMetrics() {
  try {
    const data = await api("/api/health");
    $("#metric-uptime").textContent = formatUptime(data.uptime);
    $("#metric-dbsize").textContent = formatBytes(data.dbSize);
    $("#metric-cols").textContent = String(data.collections || 0);

    // Métricas Prometheus directas
    const metricsText = await fetch("/metrics").then((r) => r.text()).catch(() => "");
    const match = metricsText.match(/docpocket_realtime_clients\s+(\d+)/);
    if (match) {
      $("#metric-clients").textContent = match[1];
    } else {
      $("#metric-clients").textContent = "0";
    }
  } catch {
    toast("No se pudieron cargar las métricas", true);
  }
}

async function runCheck() {
  $("#maintenance-output").textContent = "Ejecutando auditoría de integridad…";
  try {
    const report = await api("/api/admin/check", { method: "POST" });
    $("#maintenance-output").textContent = JSON.stringify(report, null, 2);
    const hasIssues = report.issues && report.issues.length > 0;
    toast(hasIssues ? `Se encontraron ${report.issues.length} problemas` : "Base de datos 100% consistente", hasIssues);
  } catch (err) {
    $("#maintenance-output").textContent = err.message;
    toast(err.message, true);
  }
}

async function downloadBackup() {
  try {
    toast("Generando copia de seguridad consistente…");
    const res = await fetch("/api/admin/backup", { headers: { Authorization: "Bearer " + state.token } });
    if (!res.ok) throw new Error("No se pudo generar el backup");
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const link = el("a", { href: url, download: `docpocket-backup-${new Date().toISOString().slice(0, 10)}.db` });
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
    toast("Copia de seguridad descargada correctamente");
  } catch (err) {
    toast(err.message, true);
  }
}

boot();

# DocPocket — Diseño del proyecto y guía de desarrollo

> Nombre tentativo. BaaS (Backend-as-a-Service) NoSQL, autoalojado, en un **único binario** escrito en Go, con panel de administración en Svelte y almacenamiento embebido sobre **Bbolt**. Es el equivalente "documental" de PocketBase y una alternativa autoalojable a Firebase/Firestore.

Este documento explica **qué se construye, cómo está diseñado y en qué orden desarrollarlo**. Las cifras de tiempo son estimaciones para una sola persona y los objetivos de rendimiento son metas a validar con benchmarks, no garantías. Verifica siempre las versiones de las librerías antes de fijarlas.

---

## Tabla de contenido

1. Resumen y objetivos
2. Diferenciadores
3. Arquitectura general
4. Stack y dependencias
5. Estructura del repositorio
6. Motor de almacenamiento (Bbolt)
7. Motor de consultas
8. API REST
9. Autenticación y usuarios
10. Reglas de seguridad (CEL)
11. Tiempo real (SSE)
12. Archivos
13. Panel de administración (Svelte)
14. SDK de cliente (TypeScript)
15. Hooks y extensibilidad
16. Operación: configuración, backups, migraciones, observabilidad
17. Seguridad
18. Estrategia de pruebas
19. Hoja de ruta por fases
20. Riesgos y mitigaciones
21. Decisiones abiertas
22. Primeros pasos concretos

---

## 1. Resumen y objetivos

### Qué es

Un servidor que se descarga como **un solo ejecutable**, se lanza con `./docpocket serve` y ofrece:

- Colecciones de **documentos JSON** sin esquema obligatorio (esquema opcional por colección).
- **API REST** para CRUD, filtros, orden y paginación.
- **Autenticación** (email/contraseña, OAuth2) con tokens.
- **Reglas de seguridad** por colección, al estilo Firestore.
- **Tiempo real**: los clientes reciben cambios en vivo.
- **Almacenamiento de archivos**.
- **Panel de administración** embebido en `/_/`.
- **SDK** de JavaScript/TypeScript.

### Objetivos

- Instalación en menos de un minuto: copiar el binario y ejecutarlo. Sin Docker, sin servidores de base de datos, sin configuración obligatoria.
- NoSQL real: documentos anidados, arrays, sin migraciones forzadas.
- Calidad de producción: consistencia ante cortes de energía, backups seguros, límites y protecciones por defecto.
- Un solo nodo bien afinado que cubra la gran mayoría de aplicaciones pequeñas y medianas.

### No objetivos (al menos para la v1)

- Clúster, sharding o multi-región (un solo nodo).
- Compatibilidad con el protocolo de MongoDB o con el SDK de Firebase.
- Un lenguaje SQL o consultas con joins.
- Soportar millones de escrituras concurrentes por segundo.

---

## 2. Diferenciadores

Ya existen proyectos parecidos (Appwrite, Parse Server, PocketBase, Supabase). DocPocket debe apostar por lo que ellos no ofrecen juntos:

1. **Binario único + NoSQL documental real.** PocketBase es un binario único pero relacional; Appwrite y Parse son documentales pero pesados de instalar.
2. **Reglas expresivas y seguras** con CEL, comprobables desde el panel.
3. **Consultas reactivas** (fase avanzada): el cliente se suscribe a una consulta y recibe solo las entradas, salidas y cambios de ese resultado.
4. **Sincronización offline-first en el SDK** (fase avanzada).

Elige uno o dos como bandera. El resto es "tabla de apuestas" que cualquier BaaS debe tener.

---

## 3. Arquitectura general

```
 Clientes (web / móvil)                 ┌───────────────────────────────────────────┐
   └─ SDK TypeScript ──── HTTP / SSE ──►│            Binario único (Go)             │
                                        │                                           │
 Panel Admin (Svelte, embebido) ───────►│  API REST        Realtime (SSE hub)       │
                                        │     │                  ▲                  │
                                        │  Auth · Reglas CEL · Hooks · Archivos     │
                                        │     │                  │ eventos          │
                                        │     ▼                  │ (tras commit)    │
                                        │  Motor de consultas (parser + planner)    │
                                        │     │                                     │
                                        │     ▼                                     │
                                        │  Interfaz Storage (Tx de lectura/escritura)│
                                        │     │                                     │
                                        │     ▼                                     │
                                        │  Bbolt: data.db    Bbolt: logs.db         │
                                        └───────────────────────────────────────────┘
                                                   Disco: ./dp_data/
```

Principios de diseño:

- **Todo lo que escribe pasa por el servidor.** Eso permite publicar eventos realtime sin depender del motor de datos.
- **La interfaz `Storage` aísla Bbolt.** Si algún día necesitas Pebble o MongoDB, solo cambia esa capa.
- **El índice solo genera candidatos; el documento manda.** Después de leer por índice, se vuelve a comprobar el filtro contra el documento real.
- **Índices y documentos se actualizan en la misma transacción.** Nunca quedan desincronizados tras un fallo.

---

## 4. Stack y dependencias

| Pieza | Elección | Notas |
|---|---|---|
| Lenguaje | Go (versión estable reciente) | Compilación cruzada a Linux/macOS/Windows, binario estático |
| Almacenamiento | `go.etcd.io/bbolt` | Un archivo, transacciones ACID, sin CGO |
| IDs | `github.com/oklog/ulid/v2` (o UUIDv7) | Ordenables por tiempo |
| HTTP | `net/http` de la librería estándar | Sirve de sobra con el enrutador de Go 1.22+; `chi` es una alternativa |
| Reglas | `github.com/google/cel-go` | Lenguaje de expresiones seguro, compilable una vez |
| Contraseñas | `golang.org/x/crypto/argon2` (argon2id) | Parámetros según la recomendación vigente de OWASP |
| Tokens | `github.com/golang-jwt/jwt/v5` | Tokens de acceso |
| OAuth2 | `golang.org/x/oauth2` | Google, GitHub, etc. |
| TLS automático | `golang.org/x/crypto/acme/autocert` | Let's Encrypt integrado |
| Validación opcional | JSON Schema (por ejemplo `santhosh-tekuri/jsonschema`) | Esquema opcional por colección |
| Hooks en JS | `github.com/dop251/goja` | Fase 5 |
| CLI | `flag` o `cobra` | Comandos `serve`, `backup`, `check`, etc. |
| Logs | `log/slog` | JSON estructurado |
| Panel admin | Svelte + Vite (SvelteKit con `adapter-static`) | Embebido con `//go:embed` |
| SDK | TypeScript, sin dependencias | `fetch` y `EventSource` |

---

## 5. Estructura del repositorio

```
docpocket/
├── cmd/docpocket/main.go        # punto de entrada y comandos CLI
├── internal/
│   ├── app/                     # ciclo de vida, configuración, conexión de módulos
│   ├── storage/                 # interfaz Storage + implementación Bbolt
│   ├── codec/                   # codificación de claves e índices (order-preserving)
│   ├── collections/             # definiciones, esquemas opcionales, índices
│   ├── query/                   # lexer, parser, AST, planner, ejecutor
│   ├── api/                     # handlers REST, middleware, errores
│   ├── auth/                    # usuarios, tokens, OAuth2, correos
│   ├── rules/                   # compilación y evaluación de reglas CEL
│   ├── realtime/                # hub SSE, suscripciones
│   ├── files/                   # almacenamiento de archivos
│   ├── hooks/                   # hooks Go y JS
│   ├── backup/                  # instantáneas y restauración
│   └── logs/                    # logs de peticiones en logs.db
├── ui/                          # panel de administración (Svelte)
├── sdk/ts/                      # SDK de cliente
├── docs/                        # especificación OpenAPI, guías
├── test/                        # benchmarks, e2e, pruebas de caos
├── go.mod
└── Makefile                     # build UI → build Go → binario
```

---

## 6. Motor de almacenamiento (Bbolt)

### 6.1 Qué es Bbolt y qué te da

Bbolt es una librería de clave-valor embebida: un árbol B+ en **un solo archivo**, con claves ordenadas bit a bit, buckets (similares a tablas), cursores para recorrer rangos y transacciones ACID. Lo usa etcd en producción.

Lo que **no** te da, y construyes tú: documentos, colecciones lógicas, índices secundarios, filtros, ordenamiento por campos, paginación.

### 6.2 Límites que debes conocer

- **Un solo escritor a la vez** (muchos lectores concurrentes). Las escrituras se serializan; cada commit hace `fsync`, así que el rendimiento de escritura depende del disco. Mitiga con endpoint de lotes y agrupando transacciones pequeñas.
- **El archivo no se reduce solo.** El espacio liberado se reutiliza, pero para recuperar tamaño en disco hay que compactar (copiar a un archivo nuevo).
- **Un solo proceso** puede abrir el archivo (bloqueo de archivo). Usa la opción `Timeout` al abrir para dar un error claro si hay otra instancia.
- **Transacciones de lectura largas** impiden reutilizar páginas y hacen crecer el archivo. Nunca mantengas una transacción abierta mientras esperas a un cliente (por ejemplo en un stream SSE).
- **Tamaño máximo de clave: ~32 KB.** Importante para claves de índice con strings largos (ver 6.7).

### 6.3 Archivos en disco

```
dp_data/
├── data.db          # base de datos principal (Bbolt)
├── logs.db          # logs de peticiones (Bbolt), separado para no inflar data.db
├── storage/         # archivos subidos por usuarios
├── backups/         # copias generadas por el comando backup
└── secret.key       # clave secreta del servidor (permisos 0600) — o dentro de _meta
```

### 6.4 Disposición de buckets en `data.db`

Buckets de primer nivel con prefijo (más simple y rápido que anidar):

```
_meta                    → schema_version, server_secret, ajustes
_collections             → nombre de colección → definición JSON (campos, índices, reglas, tipo)
d:<colección>            → id → documento JSON
i:<colección>:<índice>   → clave de índice codificada → (vacío | id si es único)
_refresh_tokens          → hash(refresh token) → JSON {userId, exp, ...}
_admins                  → id → superusuario
```

Las definiciones de colección se cachean en memoria y se invalidan al cambiar.

### 6.5 Identificadores

- **ULID** (128 bits, ordenable por tiempo). En el API se muestra como cadena de 26 caracteres; en las claves de índice se usa la forma binaria de 16 bytes.
- Permite recorrer en orden de creación y paginar por cursor sin `offset`.
- Los IDs pueden ser provistos por el cliente si cumplen un formato válido y no existen.

### 6.6 Formato del documento

JSON con campos reservados `id`, `created`, `updated` (marcas de tiempo en milisegundos o RFC 3339, decide una y mantenla). Ejemplo:

```json
{
  "id": "01JC8Z3X7Q0N5K9M2R4T6V8WYB",
  "created": "2026-11-15T13:30:00Z",
  "updated": "2026-11-15T13:30:00Z",
  "title": "Hola mundo",
  "views": 120,
  "author": { "id": "01JC...", "name": "Ana" },
  "tags": ["go", "svelte"]
}
```

Empieza con JSON (`encoding/json` con `UseNumber`). Si el perfil de rendimiento lo exige, evalúa MessagePack o un formato binario más adelante, ocultándolo tras el mismo módulo de serialización. Límite de tamaño por documento configurable (por ejemplo 1 MB por defecto).

### 6.7 La interfaz `Storage`

Todo el resto del proyecto solo conoce esta interfaz. Está pensada en términos de transacciones para que un futuro adaptador (Pebble, MongoDB) encaje.

```go
type Storage interface {
    View(fn func(ReadTx) error) error
    Update(fn func(WriteTx) error) error   // lo que emita eventos los publica solo si hay commit
    Backup(w io.Writer) error              // instantánea consistente
    Close() error
}

type ReadTx interface {
    Collection(name string) (*CollectionDef, bool)
    GetDoc(col, id string) (Doc, bool, error)
    ScanDocs(col string, o ScanOpts) Iterator                       // recorrido por id
    ScanIndex(col, index string, r KeyRange, o ScanOpts) Iterator   // recorrido por índice
}

type WriteTx interface {
    ReadTx
    PutDoc(col string, d Doc) error        // inserta/reemplaza y mantiene índices
    DeleteDoc(col, id string) error        // borra documento y entradas de índice
    SaveCollection(def CollectionDef) error
    DeleteCollection(name string) error
}
```

### 6.8 Codificación de claves de índice (la pieza más delicada)

Las claves de Bbolt se comparan byte a byte, así que el valor del campo debe codificarse de forma que **el orden de los bytes coincida con el orden de los valores**.

Una entrada de índice no único tiene la forma:

```
[ etiqueta de tipo (1 byte) ][ valor codificado ][ id (16 bytes) ]
```

Etiquetas de tipo (orden entre tipos): `null < false < true < número < string`.

- **null / ausente:** un campo ausente se indexa igual que `null`, para que las consultas sean coherentes.
- **booleanos:** solo la etiqueta.
- **números (float64):** tomar los 8 bytes IEEE-754 en big-endian; si el signo es 1, invertir todos los bits; si es 0, invertir solo el bit de signo. Así los bytes ordenan como los números. Limitación a documentar: enteros mayores a 2^53 pierden precisión (igual que JSON en JavaScript).
- **strings:** los bytes UTF-8 con `0x00` escapado como `0x00 0xFF`, y terminados con `0x00 0x01` para que "ab" ordene antes que "abc" y no se mezcle con el id que viene después.
- **arrays:** índice *multi-clave*: una entrada por elemento.
- **strings muy largos:** indexar solo un prefijo (por ejemplo 512 bytes) y volver a comprobar el predicado contra el documento (ya lo haces siempre).
- **índice único:** la clave es `etiqueta + valor` (sin id) y el valor almacenado es el id; así `Put` detecta violaciones con un `Get`.
- **orden descendente:** se recorre el índice hacia atrás (`Last` / `Prev`).

Esta codificación debe tener pruebas basadas en propiedades: para valores aleatorios `a < b` implica `enc(a) < enc(b)` byte a byte.

### 6.9 Mantenimiento de índices dentro de la transacción

```go
func (tx *writeTx) PutDoc(col string, d Doc) error {
    def := tx.def(col)
    docs := tx.bucket("d:" + col)

    old := docs.Get(idBytes(d.ID)) // nil si es una inserción
    if old != nil {
        for _, idx := range def.Indexes {
            tx.deleteIndexEntries(col, idx, oldDoc)
        }
    }

    if err := def.Validate(d); err != nil { return err }       // esquema opcional
    for _, idx := range def.Indexes {
        if err := tx.putIndexEntries(col, idx, d); err != nil { // comprueba unicidad
            return err
        }
    }
    if err := docs.Put(idBytes(d.ID), marshal(d)); err != nil { return err }

    tx.queueEvent(Event{Action: actionFor(old), Collection: col, Record: d, Old: oldDoc})
    return nil
}
```

Si cualquier paso falla, la transacción se revierte entera y los índices no quedan a medias. Los eventos de tiempo real **solo se publican si el commit termina bien**.

### 6.10 Concurrencia y rendimiento de escritura

- Lecturas: usan `db.View` y escalan con los núcleos.
- Escrituras: se serializan. Para operaciones pequeñas y concurrentes puedes usar `db.Batch` (agrupa varias en un commit), pero su función puede ejecutarse más de una vez, así que solo debe usarse con operaciones idempotentes.
- Ofrece un endpoint de lotes (`POST /api/batch`) para insertar muchas cosas en una sola transacción.
- No desactives `fsync` (`NoSync`) en producción.

### 6.11 Índices creados con datos existentes (en línea)

1. Marcar el índice como `building` en la definición de la colección.
2. A partir de ese momento, **toda escritura nueva** ya mantiene el índice.
3. Un proceso en segundo plano recorre los documentos existentes en lotes (transacciones pequeñas) y añade las entradas que faltan.
4. Al terminar, el índice pasa a `ready`. Mientras está en `building`, el planificador **no lo usa**.

Comando de apoyo: `docpocket reindex <colección> [índice]`.

### 6.12 Mantenimiento, backups y recuperación

- **Backup consistente:** dentro de una transacción de lectura, `tx.WriteTo(w)` escribe una instantánea coherente sin parar las escrituras. Exponlo como `docpocket backup` y como acción del panel. Opciones: copia local rotativa y, más adelante, destino compatible con S3.
- **Restauración:** `docpocket restore <archivo>` (con el servidor detenido).
- **Compactación:** copiar todo a un archivo nuevo con la utilidad/función de compactación de Bbolt (verifica la API de la versión que uses) y reemplazar. Ofrécelo como `docpocket compact`, con el servidor detenido.
- **Recuperación tras fallo:** Bbolt es copy-on-write y se reabre de forma consistente tras un corte de energía. Añade un comando `docpocket check` que verifique que cada documento tiene sus entradas de índice y que no hay entradas huérfanas.

---

## 7. Motor de consultas

### 7.1 Sintaxis del filtro

Cadena de texto en el parámetro `filter`. Ejemplos:

```
status = "active"
views >= 100 && author.name = "Ana"
tags contains "go" || tags contains "svelte"
status in ["draft", "review"] && !(archived = true)
created > @now
owner = @auth.id
```

Operadores: `=`, `!=`, `>`, `>=`, `<`, `<=`, `in`, `contains`, `~` (coincidencia de texto, fase posterior), `&&`, `||`, `!`, paréntesis. Rutas anidadas con punto (`author.name`). Variables: `@now`, `@auth.id`, `@auth.<campo>`.

### 7.2 Gramática (EBNF simplificada)

```
expr        = or ;
or          = and { "||" and } ;
and         = unary { "&&" unary } ;
unary       = [ "!" ] primary ;
primary     = comparison | "(" expr ")" ;
comparison  = path op value | path "in" list | path "contains" value ;
op          = "=" | "!=" | ">" | ">=" | "<" | "<=" | "~" ;
path        = ident { "." ident } ;
value       = string | number | "true" | "false" | "null" | variable ;
list        = "[" value { "," value } "]" ;
variable    = "@now" | "@auth" { "." ident } ;
```

Pipeline: **lexer → parser → AST → normalización → planificador → ejecutor**.

### 7.3 Planificador

Entrada: AST del filtro, `sort`, `limit`, `cursor` y los índices `ready` de la colección. Salida: un plan.

Reglas simples para la v1:

1. Separar el filtro en conjunciones de nivel superior (`A && B && C`).
2. Entre las condiciones que tienen índice, elegir la más selectiva: igualdad sobre índice único > igualdad > rango > `in`.
3. Si el `sort` coincide con un índice, recorrer **ese índice en orden** y aplicar el resto del filtro sobre los candidatos (evita ordenar en memoria).
4. Si no hay índice utilizable: recorrido completo de la colección. Configurable: permitirlo, advertir en logs, o **rechazar** consultas sin índice (modo estricto, útil en producción).
5. `OR` con ambos lados indexables: unión de resultados deduplicando por id (fase posterior).
6. Siempre **volver a comprobar el filtro completo** contra el documento leído.

Estadísticas: empieza sin ellas (heurística fija). Más adelante, guarda contadores por índice para mejorar la elección.

### 7.4 Ejecución, orden y paginación

- **Paginación por cursor**, no por `offset`: el cursor es la última clave de índice (o el último id) codificada en base64. La siguiente página continúa justo después de esa clave. Esto mantiene el coste estable aunque la colección sea enorme.
- Si hace falta ordenar en memoria (sin índice), impón un tope de documentos para evitar consumir toda la RAM.
- Límites: `limit` máximo configurable (por ejemplo 500), tiempo máximo por consulta con `context`, máximo de condiciones y de elementos en `in`.
- Respuesta: `{ "items": [...], "nextCursor": "..." }` (el cursor es `null` en la última página).

---

## 8. API REST

Base: `/api`. Formato JSON. Autenticación con `Authorization: Bearer <token>`.

| Método y ruta | Descripción |
|---|---|
| `GET /api/health` | Estado del servidor |
| `GET /api/collections/{col}/records` | Listar (parámetros: `filter`, `sort`, `limit`, `cursor`, `fields`) |
| `GET /api/collections/{col}/records/{id}` | Obtener uno |
| `POST /api/collections/{col}/records` | Crear |
| `PATCH /api/collections/{col}/records/{id}` | Actualización parcial |
| `PUT /api/collections/{col}/records/{id}` | Reemplazo completo |
| `DELETE /api/collections/{col}/records/{id}` | Eliminar |
| `POST /api/batch` | Varias operaciones en una sola transacción |
| `POST /api/collections/users/auth-with-password` | Iniciar sesión |
| `POST /api/collections/users/auth-with-oauth2` | Iniciar sesión con OAuth2 |
| `POST /api/collections/users/auth-refresh` | Renovar el token |
| `GET /api/realtime` · `POST /api/realtime` | Stream SSE y gestión de suscripciones |
| `GET /api/files/{col}/{id}/{archivo}` | Descargar archivo |
| `/api/admin/...` | Gestión de colecciones, índices, reglas, ajustes (solo superusuarios) |

Reglas de diseño del API:

- **Formato de error uniforme:** `{ "error": { "code": "validation_failed", "message": "...", "details": { "campo": "..." } } }` con códigos HTTP correctos (400, 401, 403, 404, 409, 422, 429).
- **Colecciones implícitas opcionales:** puede crearse la colección al insertar el primer documento solo en modo desarrollo; en producción conviene crearlas de forma explícita (desde el panel o con migraciones) para evitar errores por typos.
- Documenta todo con una especificación **OpenAPI** que se mantenga junto al código; el SDK se contrasta contra ella.
- Cuerpo máximo configurable, tiempos de espera del servidor y compresión gzip.

---

## 9. Autenticación y usuarios

### Modelo

- Colección de tipo `auth` (por defecto `users`) con campos de sistema: `email` (índice único), `passwordHash`, `verified`, `tokenKey` y los campos propios que defina la aplicación.
- **Superusuarios** en `_admins`, separados de los usuarios finales. El primero se crea con `docpocket superuser create <email> <clave>` o con un enlace de configuración inicial mostrado en consola.

### Contraseñas

`argon2id` con sal aleatoria y parámetros ajustables. Política mínima de longitud y limitación de intentos de inicio de sesión.

### Tokens

- **Token de acceso (JWT, corta duración):** firmado con HS256 usando una clave derivada del secreto del servidor y del `tokenKey` del usuario. Rotar el `tokenKey` invalida todos los tokens de ese usuario (cambio de contraseña, "cerrar todas las sesiones").
- **Token de refresco (opaco):** 32 bytes aleatorios, se guarda solo su hash (SHA-256) y su expiración. Con **rotación** en cada uso y detección de reutilización (si un token ya usado reaparece, se revoca la familia).

### OAuth2 y correo

- OAuth2 con PKCE (Google y GitHub primero); configurable desde el panel.
- Verificación de correo y recuperación de contraseña con tokens de un solo propósito; SMTP configurable.

---

## 10. Reglas de seguridad (CEL)

Cada colección define una regla por operación: `list`, `view`, `create`, `update`, `delete`.

Semántica:

- `null` → bloqueado salvo para superusuarios.
- `""` o `"true"` → público.
- Cualquier otra cosa → expresión CEL que debe evaluarse a `true`.
- Los superusuarios ignoran las reglas.

Variables disponibles:

| Variable | Contenido |
|---|---|
| `request.auth` | Usuario autenticado (mapa) o `null` |
| `request.method` | Operación |
| `request.body` | Cuerpo enviado (en `create`/`update`) |
| `request.query` | Parámetros de la petición |
| `resource` | Documento actual (en `create` es `null`) |
| `now` | Fecha y hora del servidor |

Ejemplos:

```
view / list : request.auth != null && resource.owner == request.auth.id
create      : request.auth != null && request.body.owner == request.auth.id
update      : resource.owner == request.auth.id && !('owner' in request.body)
delete      : request.auth.role == 'admin'
```

Implementación:

- Compilar cada expresión **una sola vez** al cargar la colección y cachear el programa; revalidar sintaxis y tipos al guardar la regla desde el panel.
- En `list`, la regla se evalúa sobre cada documento candidato. Optimización posterior (*rule pushdown*): traducir condiciones simples como `owner = @auth.id` al filtro para que el planificador use el índice en lugar de leer y descartar.
- Costo y seguridad: CEL no ejecuta código arbitrario; aun así, limita la complejidad de las expresiones y registra los rechazos.
- El panel debe incluir un **probador de reglas** ("¿este usuario puede leer este documento?").

---

## 11. Tiempo real (SSE)

Se usa **Server-Sent Events** en lugar de WebSockets: más simple, reconexión automática y compatible con proxies.

Protocolo:

1. El cliente abre `GET /api/realtime`. El servidor responde con un evento `connect` que incluye un `clientId`.
2. El cliente envía `POST /api/realtime` con `{ clientId, subscriptions: ["posts", "posts/<id>"] }` y su cabecera `Authorization`. Así el token no viaja en la URL y funciona con el `EventSource` del navegador.
3. El servidor emite eventos `event: posts` con `{ action: "create|update|delete", record: {...} }`.

Diseño interno:

- **Hub** con un canal con búfer por cliente. Si un cliente es demasiado lento, se le desconecta (para no acumular memoria).
- Los eventos se publican **después del commit**, nunca dentro de la transacción de escritura.
- Antes de enviar a cada cliente, se evalúa la regla `view` (o `list` para la colección) con su identidad.
- *Heartbeat* cada 15–25 segundos para mantener viva la conexión.
- Reanudación: un búfer circular de los últimos N eventos permite reconectar con `Last-Event-ID`; si el cliente quedó muy atrás, el SDK vuelve a cargar los datos.
- No mantengas ninguna transacción Bbolt abierta durante el stream.

**Consultas reactivas (fase avanzada):** al suscribirse con `posts?filter=views > 100`, para cada evento se evalúa el filtro contra el documento nuevo y el anterior (`Old` va en el evento). Si pasa de no cumplir a cumplir se emite como alta en esa vista; de cumplir a no cumplir, como baja; si cumple en ambos, como actualización.

Límite conocido: al ser un solo nodo, el hub vive en memoria de ese proceso. Documéntalo.

---

## 12. Archivos

- Tipo de campo `file` en el esquema de la colección (con restricciones de tamaño y tipo MIME).
- Subida `multipart/form-data`; los archivos se guardan en `storage/<colección>/<id>/<nombre>` con nombre saneado y sufijo aleatorio.
- Descarga en `/api/files/...`. Para archivos protegidos, token de corta duración en la URL (`?token=`) generado por un endpoint autenticado.
- Al borrar un documento, los archivos se eliminan **después del commit**, de forma asíncrona.
- Abstracción `FileSystem` desde el principio (disco local en la v1; S3 compatible más adelante).
- Miniaturas de imágenes: fase posterior.

---

## 13. Panel de administración (Svelte)

Proyecto en `ui/`, compilado a archivos estáticos y embebido con `//go:embed all:ui/build`; servido en `/_/` con *fallback* de SPA. En desarrollo: `vite dev` con proxy al puerto del servidor Go.

Pantallas:

1. **Inicio de sesión / configuración inicial** del primer superusuario.
2. **Colecciones:** crear/editar, esquema opcional, índices (con estado `building/ready`), reglas con probador.
3. **Explorador de documentos:** tabla con filtros y orden, vista de árbol y JSON crudo, edición con validación.
4. **Usuarios** y sesiones.
5. **Logs** de peticiones (con filtros por estado, duración, ruta).
6. **Ajustes:** SMTP, OAuth2, CORS, límites, copia de seguridad (crear, descargar, restaurar).

Calidad mínima: accesible por teclado, modo oscuro, estados de carga y error claros.

---

## 14. SDK de cliente (TypeScript)

Paquete ligero, sin dependencias externas.

```ts
import { DocPocket } from 'docpocket-sdk';

const dp = new DocPocket('http://localhost:8090');

await dp.collection('users').authWithPassword('ana@correo.com', 'clave-segura');

const page = await dp.collection('posts').getList({
  filter: 'views > 100',
  sort: '-created',
  limit: 20,
});

await dp.collection('posts').create({ title: 'Hola', views: 0 });

const unsubscribe = await dp.collection('posts').subscribe('*', (e) => {
  console.log(e.action, e.record);
});
```

Debe incluir: almacén de autenticación (memoria / `localStorage`), renovación automática del token, reconexión del stream con reanudación, tipos generados a partir de la especificación OpenAPI. Fase avanzada: caché local y cola de escrituras offline con resolución de conflictos.

---

## 15. Hooks y extensibilidad

- **Modo librería de Go:** el proyecto también se puede importar como paquete para construir una aplicación propia con hooks.

```go
app := docpocket.New()
app.OnRecordBeforeCreate("posts").Add(func(e *docpocket.RecordEvent) error {
    e.Record.Set("slug", slugify(e.Record.GetString("title")))
    return nil
})
app.Start()
```

- **Hooks en JavaScript** (`goja`), cargados desde una carpeta `hooks/`, para quien no quiera compilar Go.
- Puntos de enganche: antes/después de crear, actualizar, eliminar y listar; antes de autenticar; al arrancar y al cerrar.
- Los hooks "antes" pueden abortar la operación devolviendo un error y se ejecutan **dentro** de la transacción cuando modifican datos; los "después" se ejecutan tras el commit.
- Rutas personalizadas y tareas programadas (cron) en una fase posterior.

---

## 16. Operación: configuración, backups, migraciones, observabilidad

### Configuración

Valores por defecto que funcionan sin tocar nada; flags y variables de entorno para cambiarlos:

```
docpocket serve --dir=./dp_data --http=0.0.0.0:8090 --https=midominio.com
docpocket backup [--out=./dp_data/backups/]
docpocket restore <archivo>
docpocket compact
docpocket check
docpocket reindex <colección> [índice]
docpocket superuser create <email> <clave>
docpocket version
```

### Migraciones

- **Internas:** un número `schema_version` guardado en `_meta` y funciones de migración ordenadas, aplicadas al arrancar dentro de una transacción.
- **De usuario:** archivos de migración (colecciones, índices, reglas) en una carpeta `migrations/`, aplicados en orden; el panel puede generarlos automáticamente al cambiar la estructura, para llevar el mismo cambio de desarrollo a producción.

### Observabilidad

- Logs estructurados con `slog`; registro de peticiones en `logs.db` con retención configurable.
- Endpoint opcional `/metrics` (Prometheus) con latencias, tamaño de la base de datos, clientes realtime conectados y escrituras por segundo.
- `/api/health` para balanceadores y monitorización.

### Despliegue

Un servicio de systemd que ejecuta el binario con `Restart=always`, detrás de TLS automático (`autocert`) o de un proxy inverso (Caddy/Nginx). Documenta una guía de despliegue y de actualización (parar, reemplazar binario, arrancar; las migraciones corren solas).

---

## 17. Seguridad

- Límites de tamaño de cuerpo, profundidad máxima del JSON, tamaño máximo de documento y tiempos de espera (`ReadHeaderTimeout`, etc.).
- Limitación de peticiones por IP y por usuario (cubo de tokens), con límites más estrictos en autenticación.
- CORS con lista de orígenes permitidos; cabeceras de seguridad por defecto.
- `argon2id` para contraseñas; tokens de refresco guardados como hash; secretos nunca en logs.
- Permisos del directorio y archivos de datos (`0700`/`0600`).
- Límites de complejidad en filtros y reglas (nº de condiciones, tamaño de listas `in`) y tiempo máximo de ejecución por consulta.
- Validación de nombres de archivo y tipos MIME en subidas; archivos servidos con tipo de contenido controlado.
- Dependencias fijadas, `govulncheck` en la integración continua y actualizaciones periódicas.
- Política de divulgación de vulnerabilidades (`SECURITY.md`).

---

## 18. Estrategia de pruebas

1. **Unitarias:** lexer, parser, codificación de claves, reglas.
2. **Propiedades:** orden de la codificación (`a < b ⇒ enc(a) < enc(b)`), idempotencia de operaciones, serialización ida y vuelta.
3. **Fuzzing** (el de Go nativo) del parser de filtros y del decodificador de claves.
4. **Pruebas diferenciales del motor de consultas:** para filtros aleatorios, comparar el resultado del plan con índices contra un evaluador ingenuo de recorrido completo. Deben coincidir siempre. Es la mejor red de seguridad para el planificador.
5. **Pruebas de caos:** matar el proceso (`kill -9`) en mitad de escrituras intensas, reabrir y ejecutar `docpocket check` para verificar consistencia entre documentos e índices.
6. **Carrera de datos:** `go test -race` en toda la integración continua.
7. **Benchmarks:** 100 mil y 1 millón de documentos; medir inserción (con y sin índices), lectura por id, consultas por índice, paginación profunda, y uso de memoria y disco. Guarda los resultados para detectar regresiones.
8. **End-to-end:** SDK contra un servidor real, incluidos escenarios de tiempo real y reglas.
9. **Carga:** herramientas como k6 o vegeta contra los endpoints principales.

---

## 19. Hoja de ruta por fases

Cada fase termina con algo que funciona y con criterios de aceptación claros. Las duraciones son orientativas para una persona.

### Fase 0 — Prototipo de riesgo (1–2 semanas)

Validar lo más difícil antes de construir nada más.

- Abrir Bbolt, crear colecciones y documentos con ULID.
- Módulo `codec` con pruebas de propiedades.
- Un índice sobre un campo, un filtro de igualdad/rango y paginación por cursor.
- Benchmark con 1 millón de documentos.

**Aceptación:** consultas por índice con latencia estable aunque la colección crezca; pruebas de orden en verde; decisión informada de seguir con Bbolt (o cambiar a Pebble).

### Fase 1 — Núcleo (4–6 semanas)

- Interfaz `Storage` completa; definiciones de colección; índices simples, únicos y multi-clave.
- Parser y planificador (AND, rangos, `in`, `contains`, rutas anidadas, orden).
- API REST de registros y colecciones, errores uniformes, límites básicos.
- Comandos `serve`, `backup`, `check`.
- Pruebas diferenciales y de caos.

**Aceptación:** CRUD estable con índices; `check` limpio tras pruebas de caos; especificación OpenAPI inicial.

### Fase 2 — Autenticación y reglas (3–4 semanas)

- Colección `users`, argon2id, tokens de acceso y refresco con rotación.
- Superusuarios y configuración inicial.
- Reglas CEL por operación, con caché de programas compilados.
- Limitación de peticiones y CORS.

**Aceptación:** matriz de pruebas de permisos (anónimo, usuario, propietario, superusuario) pasando en todas las operaciones.

### Fase 3 — Tiempo real y SDK (3–4 semanas)

- Hub SSE con suscripciones por colección y por documento, reglas aplicadas por cliente, *heartbeat* y reanudación.
- SDK TypeScript con autenticación, CRUD y suscripciones.

**Aceptación:** miles de conexiones SSE simultáneas estables en una máquina modesta (objetivo a medir); sin fugas de memoria en pruebas de larga duración.

### Fase 4 — Panel de administración (4–5 semanas)

- Todas las pantallas de la sección 13, incluido el probador de reglas y la gestión de índices.
- Embebido en el binario; flujo de construcción en el `Makefile`.

**Aceptación:** un usuario nuevo puede instalar, crear colección, definir reglas y ver datos solo con el panel.

### Fase 5 — Archivos, hooks, backups avanzados (3–4 semanas)

- Subida y descarga de archivos con tokens.
- Hooks en Go (modo librería) y en JS.
- Migraciones de usuario, restauración, compactación, destino S3 para backups.
- Correo (verificación, recuperación) y OAuth2.

**Aceptación:** ciclo completo de backup → restauración verificado; ejemplos de hooks documentados.

### Fase 6 — Endurecimiento y lanzamiento (3–4 semanas)

- Revisión de seguridad, `govulncheck`, límites por defecto razonables.
- Documentación (guía de inicio, referencia de API, despliegue), ejemplos y plantillas.
- Binarios para Linux, macOS y Windows con versionado semántico y notas de versión.

**Aceptación:** versión 1.0 publicada, probada en un despliegue real de larga duración.

### Fase 7 — Diferenciadores (continuo)

Consultas reactivas, sincronización offline-first en el SDK, *rule pushdown*, `OR` con unión de índices, estadísticas para el planificador, adaptador opcional de MongoDB o Pebble para escenarios con más escala.

---

## 20. Riesgos y mitigaciones

| Riesgo | Impacto | Mitigación |
|---|---|---|
| El motor de consultas es complejo y propenso a errores | Alto | Alcance acotado en la v1, pruebas diferenciales, fuzzing, comprobación final contra el documento |
| Escritor único de Bbolt limita el volumen de escritura | Medio | Endpoint de lotes, `Batch`, documentar el límite, interfaz `Storage` que permita Pebble |
| Crecimiento del archivo de datos | Medio | Compactación documentada, logs en archivo separado, monitorización del tamaño |
| Inconsistencia entre documentos e índices | Alto | Misma transacción, `docpocket check`, pruebas de caos |
| Fugas de memoria o clientes lentos en tiempo real | Medio | Canales con búfer y desconexión de clientes lentos, pruebas de larga duración |
| Reglas de seguridad mal configuradas | Alto | Probador en el panel, valores por defecto cerrados (`null`), registro de rechazos |
| Alcance excesivo del proyecto | Alto | Fases con criterios de aceptación, lanzar la v1 sin los diferenciadores |
| Un solo mantenedor | Medio | Documentación, pruebas automatizadas, diseño modular y licencia clara |

---

## 21. Decisiones abiertas

- **Nombre y licencia** (MIT o Apache-2.0 son opciones habituales).
- **Formato interno del documento:** JSON puro o uno binario (MessagePack) tras medir.
- **Marcas de tiempo:** RFC 3339 o milisegundos desde la época.
- **Relaciones entre documentos** (`expand` de referencias): ¿v1 o posterior?
- **Transacciones multi-colección** expuestas al cliente más allá de `/api/batch`.
- **Modo estricto de índices por defecto:** rechazar consultas sin índice, ¿sí o no?
- **Camino hacia más escala:** Pebble como segundo motor o adaptador de MongoDB para quienes necesiten alta disponibilidad.

---

## 22. Primeros pasos concretos

```bash
mkdir docpocket && cd docpocket
go mod init github.com/tu-usuario/docpocket
go get go.etcd.io/bbolt@latest
go get github.com/oklog/ulid/v2@latest
mkdir -p cmd/docpocket internal/{storage,codec,query,api,collections}
```

Orden sugerido para la primera semana:

1. `internal/codec`: codificación de claves + pruebas de propiedades (orden).
2. `internal/storage`: abrir `data.db` (con `Timeout`), buckets `_meta`, `d:<col>` e `i:<col>:<índice>`.
3. `PutDoc` / `GetDoc` / `DeleteDoc` con mantenimiento de un índice dentro de la transacción.
4. `ScanIndex` con rangos, orden ascendente/descendente y cursor.
5. Un `main.go` mínimo que inserte N documentos y mida los tiempos.
6. Benchmark con 100 mil y luego 1 millón de documentos; anota resultados en `docs/benchmarks.md`.
7. Decidir con datos: seguir con Bbolt o evaluar Pebble antes de pasar a la Fase 1.

Un ejemplo mínimo para abrir el archivo y asegurar que un segundo proceso falle con un mensaje claro:

```go
db, err := bolt.Open(filepath.Join(dir, "data.db"), 0o600,
    &bolt.Options{Timeout: 2 * time.Second})
if err != nil {
    return fmt.Errorf("no se pudo abrir la base de datos (¿hay otra instancia en ejecución?): %w", err)
}
```

---

*Fin del documento. Mantenlo vivo: actualiza las decisiones abiertas y los resultados de benchmarks conforme avances.*




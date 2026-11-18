# DocPocket

BaaS documental autoalojado en **un único binario Go** con **Bbolt** como motor embebido. Incluye API REST, autenticación JWT con refresh tokens, reglas de seguridad CEL, tiempo real por SSE, archivos locales y un panel de administración en `/_/`.

Sin servicios externos: una base de datos, un proceso y un panel.

## Características

- Colecciones con esquemas e índices configurables.
- API REST con `filter`, `sort`, `fields`, `include`, operaciones `batch` y `transactions`, y agregaciones en servidor.
- Reglas CEL para controlar el acceso a registros.
- SSE para suscribirse a cambios en tiempo real.
- Superusuarios, sesiones de actualización y revocación por familia o identidad.
- Panel web embebido, backup/restauración y compactación desde CLI o API.

## Inicio rápido

```bash
go build -o docpocket ./cmd/docpocket
./docpocket serve --dir=./dp_data --http=127.0.0.1:8090
```

Abre el panel en `http://127.0.0.1:8090/_/` y crea el primer superusuario. También puedes hacerlo por CLI o con `POST /api/admin/setup` (solo funciona mientras no exista ningún superusuario):

```bash
./docpocket superuser create --dir=./dp_data admin@example.com 'una-clave-larga'
```

## Comandos

```
docpocket serve [flags]
docpocket backup   [--dir=DIR] [--out=DIR]
docpocket restore  [--dir=DIR] <archivo>
docpocket compact  [--dir=DIR]
docpocket check    [--dir=DIR]
docpocket reindex  [--dir=DIR] <coleccion> [indice]
docpocket superuser create [--dir=DIR] <email> <clave>
docpocket version
```

## Estructura

| Ruta | Contenido |
| --- | --- |
| `cmd/docpocket` | Punto de entrada del binario |
| `internal/api` | API REST y endpoints del panel |
| `internal/app` | Arranque del servidor y mantenimiento |
| `internal/auth` | JWT, refresh tokens y superusuarios |
| `ui/` | Panel Svelte 5 + Vite |
| `sdk/ts/` | SDK TypeScript |
| `examples/nexo-desk` | Aplicación de ejemplo |

## Panel de administración

El panel de `/_/` está escrito en **Svelte 5 + Vite**, se compila a `ui/build/` y se embebe en el binario con `//go:embed`. El contenido de `ui/build/` es generado: no se edita a mano. Para reconstruirlo necesitas [Bun](https://bun.sh), que no hace falta para ejecutar DocPocket:

```bash
cd ui
bun install
bun test
bun run build
cd ..
go build -o docpocket ./cmd/docpocket
```

Tiene 11 secciones: Resumen, Colecciones, Usuarios, Archivos, Reglas CEL, Tiempo real, Registro, Mantenimiento, Ajustes, Superusuarios y Sesiones. Incluye editor de esquemas e índices, explorador de registros, vistas guardadas, operaciones `batch` y `transactions`, agregaciones en servidor, SSE en vivo, carga y borrado de archivos, probador de reglas CEL, backup/restauración/compactación y gestión de ajustes, superusuarios y sesiones.

Los tokens de sesión se guardan en `sessionStorage`, no en `localStorage`. El panel respeta la CSP estricta que sirve el servidor en `/_/`: sin estilos ni scripts en línea, CDN ni `eval`.

## API de administración

Además de la gestión de colecciones, reglas y backups, el panel usa estos endpoints. Todos requieren token de superusuario:

| Método | Ruta | Uso |
| --- | --- | --- |
| `GET` | `/api/admin/logs?status=&q=&limit=` | Registro de peticiones en memoria, con filtros |
| `GET` / `PATCH` | `/api/admin/settings` | Título, CORS, límite de auth y retención de logs |
| `GET` / `POST` | `/api/admin/admins` | Listar y crear superusuarios |
| `DELETE` | `/api/admin/admins/{id}` | Eliminar superusuario (ni el propio ni el último) |
| `GET` | `/api/admin/sessions?kind=&limit=` | Listar sesiones de actualización, sin exponer tokens |
| `POST` | `/api/admin/sessions/revoke` | Revocar por familia, identidad o todas |
| `POST` | `/api/admin/compact` | Compactar la base de datos |
| `POST` | `/api/admin/restore` | Restaurar un `.db` (multipart `file`) |

Los cambios de CORS y del límite de autenticación se aplican sin reiniciar. `compact` y `restore` bloquean el resto de peticiones mientras se sustituye la base de datos; las streams SSE activas pueden retrasar la operación.

## SDK TypeScript

```ts
import { DocPocket } from "docpocket-sdk";

const dp = new DocPocket("http://localhost:8090", {
  storage: window.localStorage,
});

await dp.collection("users").authWithPassword("ana@correo.com", "clave-segura");

const page = await dp.collection("posts").getList({
  filter: "views > 100",
  sort: "-created",
  limit: 20,
});

await dp.collection("posts").create({ title: "Hola", views: 0 });

const unsubscribe = await dp.collection("posts").subscribe("*", (event) => {
  console.log(event.action, event.record);
});
```

El SDK guarda tokens, los renueva automáticamente en respuestas 401, reconecta SSE y vuelve a registrar las suscripciones con el `clientId` nuevo. Ver [`sdk/ts/README.md`](sdk/ts/README.md).

## Ejemplo: Nexo Desk

`examples/nexo-desk` es una suite de agencia construida sobre DocPocket: proyectos, CRM, tareas, tiempo, facturación, SSE, batch, archivos, consultas en servidor y exportación. Usa Bun + Vite + Svelte 5 + TypeScript.

```bash
cd examples/nexo-desk
bun run bootstrap   # datos de demostración (idempotente)
bun run dev         # http://127.0.0.1:5173
```

Las carencias encontradas al construir el ejemplo están en [`examples/nexo-desk/LIMITATIONS.md`](examples/nexo-desk/LIMITATIONS.md).

## Pruebas

```bash
go test ./...
cd ui && bun test
```

## Despliegue

La guía completa (systemd, TLS con ACME o proxy inverso, copias de seguridad, restauración, compactación y actualización) está en [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

El `Dockerfile` incluido compila un binario estático y monta `/data` como volumen:

```bash
docker build -t docpocket .
docker run -d --name docpocket -p 8090:8090 -v docpocket-data:/data docpocket
docker exec -it docpocket docpocket superuser create --dir=/data admin@example.com 'clave-larga'
```

Crea el superusuario antes de exponer el panel a internet.

# DocPocket

BaaS documental autoalojado en **un único binario Go** con **Bbolt** como motor
embebido. Incluye API REST, autenticación con JWT + refresh tokens, reglas de
seguridad CEL, tiempo real por SSE, archivos locales y un panel de administración
embebido en `/_/`.

un servidor para ejecutar la suite.

## Inicio rápido

```bash
go build -o docpocket ./cmd/docpocket

# Servidor (datos en ./dp_data)
./docpocket serve --dir=./dp_data --http=127.0.0.1:8090

# Panel de administración
# http://127.0.0.1:8090/_/
```

El primer superusuario se crea desde el panel o por CLI:

```bash
./docpocket superuser create --dir=./dp_data admin@example.com 'una-clave-larga'
```

También se puede usar `POST /api/admin/setup` la primera vez; el endpoint se
bloquea en cuanto existe un superusuario.

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

El SDK guarda tokens, renueva automáticamente en respuestas 401, reconecta SSE y
vuelve a registrar las suscripciones con el `clientId` nuevo. Véase
[`sdk/ts/README.md`](sdk/ts/README.md).


## Ejemplo operativo: Nexo Desk

`examples/nexo-desk` es una suite completa de agencia construida sobre DocPocket
(proyectos, CRM, tareas, tiempo, facturación, SSE, batch, archivos, consultas en
servidor y exportación). Construida con Bun + Vite + Svelte 5 + TypeScript.

```bash
cd examples/nexo-desk
bun run bootstrap   # sembrar datos de demostración (idempotente)
bun run dev         # interfaz en http://127.0.0.1:5173
```

Las carencias reales encontradas durante la construcción del ejemplo —tanto en DocPocket
como en la base de datos Bbolt— están documentadas en
[`examples/nexo-desk/LIMITATIONS.md`](examples/nexo-desk/LIMITATIONS.md).


## Despliegue

La guía completa (systemd, TLS con ACME o proxy inverso, copias de seguridad,
restauración, compactación y actualización) está en
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md). El `Dockerfile` incluido compila un
binario estático y monta `/data` como volumen:

```bash
docker build -t docpocket .
docker run -d --name docpocket -p 8090:8090 -v docpocket-data:/data docpocket
```

Antes de exponer el panel, crea el superusuario:

```bash
docker exec -it docpocket docpocket superuser create --dir=/data admin@example.com 'clave-larga'
```


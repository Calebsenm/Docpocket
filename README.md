# DocPocket

Self-hosted document BaaS in a **single Go binary** with **Bbolt** as the embedded engine. It includes a REST API, JWT authentication with refresh tokens, CEL security rules, real-time updates over SSE, local file storage, and an admin panel at `/_/`.

No external services required: one database, one process, and one panel.

## Features

- Collections with configurable schemas and indexes.
- REST API with `filter`, `sort`, `fields`, `include`, `batch` and `transactions` operations, and server-side aggregations.
- CEL rules to control record access.
- SSE subscriptions for real-time changes.
- Superusers, refresh sessions, and revocation by family or identity.
- Embedded web panel, backup/restore, and compaction from the CLI or API.

## Quick start

```bash
go build -o docpocket ./cmd/docpocket
./docpocket serve --dir=./dp_data --http=127.0.0.1:8090
```

Open the panel at `http://127.0.0.1:8090/_/` and create the first superuser. You can also do it from the CLI or with `POST /api/admin/setup` (which only works while no superuser exists):

```bash
./docpocket superuser create --dir=./dp_data admin@example.com 'a-long-password'
```

## Commands

```
docpocket serve [flags]
docpocket backup   [--dir=DIR] [--out=DIR]
docpocket restore  [--dir=DIR] <file>
docpocket compact  [--dir=DIR]
docpocket check    [--dir=DIR]
docpocket reindex  [--dir=DIR] <collection> [index]
docpocket superuser create [--dir=DIR] <email> <password>
docpocket version
```

## Repository layout

| Path | Contents |
| --- | --- |
| `cmd/docpocket` | Binary entry point |
| `internal/api` | REST API and panel endpoints |
| `internal/app` | Server startup and maintenance |
| `internal/auth` | JWT, refresh tokens, and superusers |
| `ui/` | Svelte 5 + Vite panel |
| `sdk/ts/` | TypeScript SDK |
| `examples/nexo-desk` | Example application |

## Admin panel

The `/_/` panel is built with **Svelte 5 + Vite**, compiled into `ui/build/`, and embedded in the binary with `//go:embed`. The contents of `ui/build/` are generated: do not edit them by hand. Rebuilding requires [Bun](https://bun.sh), which is not needed to run DocPocket:

```bash
cd ui
bun install
bun test
bun run build
cd ..
go build -o docpocket ./cmd/docpocket
```

It has 11 sections: Overview, Collections, Users, Files, CEL Rules, Realtime, Logs, Maintenance, Settings, Admins, and Sessions. It includes a schema and index editor, a record browser, saved views, `batch` and `transactions` operations, server-side aggregations, live SSE, file upload and deletion, a CEL rule tester, backup/restore/compaction, and management of settings, superusers, and sessions.

Session tokens are stored in `sessionStorage`, not `localStorage`. The panel respects the strict CSP served by the server for `/_/`: no inline styles or scripts, no CDNs, and no `eval`.

## Admin API

In addition to collection, rule, and backup management, the panel uses these endpoints. All of them require a superuser token:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/admin/logs?status=&q=&limit=` | In-memory request log with filters |
| `GET` / `PATCH` | `/api/admin/settings` | Title, CORS, auth limit, and log retention |
| `GET` / `POST` | `/api/admin/admins` | List and create superusers |
| `DELETE` | `/api/admin/admins/{id}` | Delete a superuser (not yourself or the last one) |
| `GET` | `/api/admin/sessions?kind=&limit=` | List refresh sessions without exposing tokens |
| `POST` | `/api/admin/sessions/revoke` | Revoke by family, identity, or all |
| `POST` | `/api/admin/compact` | Compact the database |
| `POST` | `/api/admin/restore` | Restore a `.db` file (multipart `file`) |

CORS and auth limit changes take effect without a restart. `compact` and `restore` block other requests while the database is replaced; active SSE streams can delay the operation.

## TypeScript SDK

```ts
import { DocPocket } from "docpocket-sdk";

const dp = new DocPocket("http://localhost:8090", {
  storage: window.localStorage,
});

await dp.collection("users").authWithPassword("ana@example.com", "strong-password");

const page = await dp.collection("posts").getList({
  filter: "views > 100",
  sort: "-created",
  limit: 20,
});

await dp.collection("posts").create({ title: "Hello", views: 0 });

const unsubscribe = await dp.collection("posts").subscribe("*", (event) => {
  console.log(event.action, event.record);
});
```

The SDK stores tokens, renews them automatically on 401 responses, reconnects SSE, and re-registers subscriptions with the new `clientId`. See [`sdk/ts/README.md`](sdk/ts/README.md).

## Example: Nexo Desk

`examples/nexo-desk` is an agency suite built on DocPocket: projects, CRM, tasks, time tracking, invoicing, SSE, batch operations, files, server-side queries, and exports. It uses Bun + Vite + Svelte 5 + TypeScript.

```bash
cd examples/nexo-desk
bun run bootstrap   # seed demo data (idempotent)
bun run dev         # http://127.0.0.1:5173
```

The gaps found while building the example are documented in [`examples/nexo-desk/LIMITATIONS.md`](examples/nexo-desk/LIMITATIONS.md).

## Tests

```bash
go test ./...
cd ui && bun test
```

## Deployment

The full guide (systemd, TLS with ACME or a reverse proxy, backups, restore, compaction, and upgrades) is in [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

The included `Dockerfile` builds a static binary and mounts `/data` as a volume:

```bash
docker build -t docpocket .
docker run -d --name docpocket -p 8090:8090 -v docpocket-data:/data docpocket
docker exec -it docpocket docpocket superuser create --dir=/data admin@example.com 'a-long-password'
```

Create the superuser before exposing the panel to the internet.

# AGENTS.md

This file is guidance for future coding agents working in this repository or building quick prototypes with a running Flink server.

## What Flink Is

Flink is a zero-config internal platform for live HTML/JS prototypes. The server hosts the dashboard, tenant-aware static sites, browser storage APIs, uploads, realtime WebSocket rooms, optional AI calls, and tenant auth. The user CLI is separate from the server and talks to a running server over HTTP.

Keep the product bias simple: optimize for developer joy, fast iteration, and a magical "save HTML, get a live URL" loop. Avoid adding heavyweight platform, auth, moderation, or scalability machinery unless the user explicitly asks for it.

## Repository Map

- `go.work`: Go workspace tying together separate Go modules.
- `server/`: server module and operator binary.
- `server/main.go`: single server entrypoint.
- `server/cmd/`: Cobra server commands: serve, config init, tenant management.
- `server/app/`: HTTP app wiring, routing, sessions, auth, embedded frontend.
- `server/api/`: tenant-aware site APIs, storage APIs, uploads, realtime hub, AI endpoints.
- `server/storage/`: storage abstraction and backends. Use this abstraction for Flink state and offered storage APIs.
- `server/frontend/`: React + Vite + Tailwind dashboard. All frontend code must be TypeScript.
- `server/frontend/static/`: static embedded files such as login HTML and shared browser library output.
- `server/extras/`: systemd unit and install/update script.
- `cli/`: separate user CLI module.
- `cli/main.go`: single CLI entrypoint.
- `cli/cmd/`: Cobra user commands and HTTP client code.
- `client/`: TypeScript browser SDK package for Flink APIs.
- `.ona/automations.yaml`: Ona tasks/services for building, testing, and running Flink.

The server and user CLI are intentionally separate products. The server CLI is for operators. The `flink` CLI is for tenants who publish and manage their own sites.

## Build And Test

Use the Makefile unless you have a narrow reason not to:

```sh
make test
make build
make run
```

Focused checks:

```sh
go test ./cli/cmd ./server/cmd
go test ./server/app ./server/api ./server/storage
cd client && npm test
cd server/frontend && npm run typecheck && npm run build
```

Do not run `make test` and `make build` in parallel. Both install frontend dependencies and can race in `node_modules`.

Generated or local runtime output should not be committed:

- `bin/`
- `data/`
- `.flink/`
- `client/dist/`
- `server/frontend/dist/`
- `node_modules/`

## Architecture Rules

- Keep Go modules separate: `server` and `cli` are independent workspace modules.
- Keep Cobra entrypoints thin. Root files should wire commands; individual commands belong in their own files.
- Keep server runtime configuration in YAML config, not environment variables or ad hoc flags.
- All user-facing state must be tenant-scoped.
- All tenant interaction must be authenticated unless the endpoint is explicitly login, logout, registration, or static public serving.
- Use `server/storage.Backend` for durable state. Do not bypass it with direct filesystem/database calls for Flink state.
- The frontend must be React, Vite, Tailwind, and TypeScript. Do not add JavaScript files.
- The browser SDK in `client/` should expose pleasant APIs for storage, uploads, realtime, and AI. Keep it small and dependency-light.

## Tenant Model

Everything meaningful happens inside a tenant:

- dashboard access
- site CRUD
- hosted site viewing
- JSON/key-value storage APIs
- file upload APIs
- WebSocket realtime rooms
- AI calls
- CLI publishing

Tenant web sessions use cookies. The user CLI uses HTTP Basic Auth with tenant username and password.

Useful operator commands:

```sh
go run ./server init --config flink.yaml
go run ./server --config flink.yaml
go run ./server tenants pending --config flink.yaml
go run ./server tenants approve <username> --config flink.yaml
go run ./server tenants create demo flink --config flink.yaml
go run ./server tenants bootstrap demo flink --config flink.yaml
```

For local automation, bootstrap a tenant in config:

```yaml
bootstrapTenants:
  - username: demo
    password: flink
```

or enable immediate registration approval in high-trust environments:

```yaml
autoApproveTenants: true
```

## Building Prototypes With Flink

When asked to build a prototype on Flink, prefer using Flink itself instead of adding a new app framework:

1. Sign in as an approved tenant.
2. Create or update a site with the user CLI.
3. Publish a single `index.html` first, or publish a static directory when the prototype has assets/routes.
4. Use the browser SDK from the hosted shared library:

```html
<script src="/flink.js"></script>
```

5. Store prototype state with the Flink storage API instead of inventing a backend.
6. Use uploads for binary/user files and store returned URLs in JSON state.
7. Use realtime rooms for chat, cursors, collaboration, multiplayer, or presence.
8. Use AI endpoints only when the server config has AI credentials.

The fastest CLI loop for a tenant is:

```sh
flink --server https://flink.internal --tenant demo --password flink site create my-prototype
flink --server https://flink.internal --tenant demo --password flink site write my-prototype ./dist
```

On a shared Flink server with `baseHost` and wildcard DNS configured, open the domain-based site URL:

```text
https://demo--my-prototype.flink.internal/
```

Domain-based hosting is the preferred shape for Flink. Path-based hosting is only the fallback for localhost or servers without `baseHost` configured:

```text
http://localhost:8080/t/demo/s/my-prototype/
```

Prototype HTML can be self-contained or published as a directory tree. Nested files are served below the same site base, and directory indexes resolve through `index.html`. Keep interactions obvious, avoid build steps inside hosted sites, and make the first saved version usable.

## Ona Environment

Prefer the checked-in Ona automations when available:

```sh
gitpod automations service start flink
gitpod automations task start build
gitpod automations task start test
```

The Flink service exposes port `8080`, runs the real server, and bootstraps:

```text
username: demo
password: flink
```

Use Ona port commands for user-facing previews when running inside an Ona environment.

## Editing Discipline

- Preserve user changes. Do not reset or checkout files unless the user explicitly asks.
- Keep edits scoped to the requested behavior.
- Update tests when behavior changes.
- Run focused tests first, then broader Make targets when practical.
- For frontend changes, verify `cd server/frontend && npm run typecheck && npm run build`.
- For SDK changes, verify `cd client && npm test && npm run build`.
- For server API/storage changes, verify relevant Go package tests and `make test` when feasible.

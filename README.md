# Flink

Flink is a tiny internal platform for live HTML/JS prototypes. The server hosts the dashboard, static sites, browser APIs, uploads, WebSocket rooms, and approved tenant accounts. The separate user CLI publishes to a running server over HTTP.

## Layout

```text
go.work              workspace linking the separate modules
client/              TypeScript browser SDK package for Flink APIs
cli/                 user CLI module and HTTP client
cli/main.go          user CLI binary entrypoint
cli/cmd/             user CLI commands and HTTP client
server/              server module, HTTP routing, APIs, frontend embedding
server/main.go       operator server binary entrypoint
server/app/          HTTP app package, tenant auth, routing, storage wiring
server/api/          site storage, JSON data, uploads, realtime hub
server/extras/       systemd unit and curlable install/update script
server/storage/      backend abstraction plus file and bbolt implementations
server/frontend/     React/Vite/Tailwind dashboard plus embedded static assets
.ona/automations.yaml Ona service and build/test tasks
```

## Getting Started

For local development, generate a config and run the server:

```sh
go run ./server init --config flink.yaml
go run ./server --config flink.yaml
```

The generated config is intentionally small:

```yaml
addr: :8080
dataDir: ./data
storage: file
baseHost: ""
autoApproveTenants: false
ai:
  apiKey: ""
  baseURL: https://api.openai.com/v1
  model: gpt-4.1-mini
bootstrapTenants: []
```

You can also use Make during development:

```sh
make run
```

Open `http://localhost:8080/_flink`, register a tenant, then approve it from the server binary:

```sh
go run ./server tenants pending --config flink.yaml
go run ./server tenants approve <tenant> --config flink.yaml
```

After approval, sign in on the web, create a site, save `index.html`, then open `http://localhost:8080/t/<tenant>/s/<site>/`.

Pick the server storage backend by editing `flink.yaml`:

```yaml
dataDir: ./data
storage: bbolt
```

On a VPS, install or update the systemd service with the curlable installer:

```sh
curl -fsSL https://raw.githubusercontent.com/csweichel/flink/main/server/extras/install.sh | sudo sh
```

The installer writes:

```text
/opt/flink/flink-server       server binary
/etc/flink/flink.yaml         server config, created only if missing
/var/lib/flink                default data directory
/etc/systemd/system/flink.service
```

By default it initializes `/etc/flink/flink.yaml` with:

```yaml
addr: :8080
dataDir: /var/lib/flink
storage: bbolt
baseHost: ""
autoApproveTenants: false
ai:
  apiKey: ""
  baseURL: https://api.openai.com/v1
  model: gpt-4.1-mini
bootstrapTenants: []
```

Run the same command again to update the binary and restart the service. For unreleased builds or private artifacts, pass an explicit binary or tarball URL:

```sh
curl -fsSL https://raw.githubusercontent.com/csweichel/flink/main/server/extras/install.sh | sudo FLINK_DOWNLOAD_URL=https://example.com/flink-server_linux_amd64.tar.gz sh
```

Useful installer env vars:

```text
FLINK_DOWNLOAD_URL  exact binary, .gz, or .tar.gz URL to install
FLINK_VERSION       GitHub release tag, defaults to latest
FLINK_REPO          GitHub repo for releases, defaults to csweichel/flink
FLINK_INSTALL_DATA_DIR  data directory written by initial config, defaults to /var/lib/flink
```

Ona can run the same server with:

```sh
gitpod automations service start flink
```

That service opens port `8080` as `flink`, starts the real Flink server, and bootstraps an approved demo tenant before serving traffic:

```text
username: demo
password: flink
```

The service writes those credentials into `.flink/ona.yaml`. Its readiness check fetches `/flink.js` and calls `/api/sites` with the same credentials, so a ready service is also ready to sign in and use.

## Tenants And Auth

Everything user-facing happens inside an approved tenant:

- dashboard access
- site CRUD
- hosted site viewing
- browser storage/upload/AI APIs
- WebSocket rooms
- CLI publishing

Users register at `/_flink/register`. By default, new registrations are `pending` until a server operator approves or denies them:

```sh
flink-server tenants list
flink-server tenants list pending
flink-server tenants pending
flink-server tenants get alice
flink-server tenants approve alice
flink-server tenants deny alice
flink-server tenants reset-password alice new-secret
flink-server tenants delete alice
```

Tenant operator commands read storage settings from the same YAML config:

```sh
flink-server tenants pending --config /etc/flink/flink.yaml
flink-server tenants approve alice --config /etc/flink/flink.yaml
```

In a high-trust environment, registrations can be approved immediately:

```yaml
autoApproveTenants: true
```

When that is enabled, registration signs the tenant into the dashboard immediately.

For local demos and automation, either add an approved bootstrap tenant to the config:

```yaml
bootstrapTenants:
  - username: demo
    password: flink
```

or create an approved tenant directly against the configured storage:

```sh
flink-server tenants create demo flink --config flink.yaml
flink-server tenants bootstrap demo flink --config flink.yaml
```

The web app uses a tenant session cookie. The user CLI uses HTTP Basic Auth with the tenant username and password.

## Server Config

Generate a YAML config file:

```sh
go run ./server init --config flink.yaml
```

Example `flink.yaml`:

```yaml
addr: :8080
dataDir: ./data
storage: file
baseHost: ""
autoApproveTenants: false
ai:
  apiKey: ""
  baseURL: https://api.openai.com/v1
  model: gpt-4.1-mini
bootstrapTenants: []
```

Run with the config:

```sh
go run ./server --config flink.yaml
```

The server loads defaults and then the YAML config. It does not read server runtime settings from environment variables, and `--config` is the only server flag outside `init`. If `flink.yaml` exists in the working directory, it is loaded automatically. Tenant operator commands accept the same config:

```sh
go run ./server tenants pending --config flink.yaml
```

## Build And Test

```sh
make build
make test
```

Both commands run the Vite frontend build first, then compile or test Go. `make build` writes `bin/flink` and `bin/flink-server`.

`make build`, `make test`, and `make run` also build `client/` and copy its browser bundle into `server/frontend/static/flink.js`, so the script tag and npm package use the same implementation.

## Storage

The server uses one storage abstraction for its own Flink state and for user-facing APIs:

- tenant registrations, approvals, and sessions
- site metadata
- hosted site files
- JSON document/key-value data
- uploaded files

Supported drivers:

```text
file    default, directory-backed storage under dataDir
bbolt   single-file embedded database at dataDir/flink.db
```

The adapter boundary is in `server/storage`. DynamoDB and Firebase can be added by implementing the same `storage.Backend` interface, without changing `server/api` or the browser API.

Configure storage in `flink.yaml`:

```yaml
dataDir: /opt/flink/data
storage: bbolt
```

## Browser API

For instant prototypes, add this to any Flink page:

```html
<script src="/flink.js"></script>
```

Use the zero-config APIs after signing into the tenant in the browser:

```ts
await flink.set("note", { text: "hello" });
const note = await flink.get("note");

const uploaded = await flink.upload(fileInput.files[0]);

const room = flink.room("chat", console.log);
room.send({ text: "hi" });

const ai = await flink.ai("Give me an idea");
```

For TypeScript projects, use the client package:

```ts
import { createFlinkClient } from "@flink/client";

const flink = createFlinkClient({ site: "demo", baseUrl: "http://localhost:8080" });

await flink.storage.set("note", { text: "hello" });
const note = await flink.storage.get<{ text: string }>("note");

const uploaded = await flink.upload(file);
const text = await flink.uploads.text(uploaded);

const room = flink.realtime.room<{ text: string }>("chat");
room.send({ text: "hi" });

const idea = await flink.ai({ prompt: "Give me a prototype idea", maxOutputTokens: 80 });
```

The client package exposes:

```text
storage   JSON key-value/document APIs
files     hosted site file read/write helpers
uploads   file upload plus URL/fetch/text/json/blob helpers
realtime  WebSocket room messaging
ai        optional server-side LLM endpoint
```

If `ai.apiKey` is empty, `flink.ai()` returns a stable "not configured" response. To enable real calls, edit `flink.yaml`:

```yaml
ai:
  apiKey: sk-...
  baseURL: https://api.openai.com/v1
  model: gpt-4.1-mini
```

## CLI

```sh
go run ./cli --server http://localhost:8080 --tenant alice --password secret site create demo
go run ./cli --server http://localhost:8080 --tenant alice --password secret site write demo ./index.html
go run ./cli --server http://localhost:8080 --tenant alice --password secret site list
```

The CLI is for Flink users and only talks to the server API. It does not import or manage server internals. Use `FLINK_SERVER`, `FLINK_TENANT`, and `FLINK_PASSWORD` to avoid repeating flags.

Server operators run `flink-server --config flink.yaml` directly or use `make run`, which creates `.flink/dev.yaml` on first run. Edit the YAML file to configure hosting.

## Deploy

Build and copy the binary to a small VPS:

```sh
make build
scp bin/flink-server vm:/opt/flink/flink-server
```

Run it behind Caddy or nginx:

```sh
/opt/flink/flink-server init --config /opt/flink/flink.yaml
$EDITOR /opt/flink/flink.yaml
/opt/flink/flink-server --config /opt/flink/flink.yaml
```

Caddy example:

```caddyfile
flink.internal, *.flink.internal {
  reverse_proxy 127.0.0.1:8080
}
```

Sites work at `/t/alice/s/demo/`. Signed-in users can also use `/s/demo/` as a shorthand for their current tenant. With wildcard DNS, tenant subdomains use `https://alice--demo.flink.internal/`.

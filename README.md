# Flink

Flink is a small internal platform for hosting live HTML/TypeScript prototypes and simple websites. It gives every approved tenant a dashboard, instant site hosting, a user CLI, browser-callable storage APIs, uploads, realtime rooms, and optional AI calls.

Use Flink when you want to go from `index.html` to a live shareable internal URL without setting up a backend, database, object storage bucket, or websocket service.

## Use Flink To Host A Website

You need three things from your Flink server operator:

```text
server URL
tenant username
tenant password
```

Open the dashboard:

```text
https://<flink-server>/_flink
```

Sign in, create a site, edit `index.html`, save it, and open:

```text
https://<flink-server>/t/<tenant>/s/<site>/
```

Signed-in users can also use the shorthand:

```text
https://<flink-server>/s/<site>/
```

## Publish With The User CLI

The `flink` CLI is for website authors and agents. It only talks to a running Flink server over HTTP.

Build the CLI from this repo:

```sh
make build
```

Create and publish a site:

```sh
bin/flink --server http://localhost:8080 --tenant demo --password flink site create hello
bin/flink --server http://localhost:8080 --tenant demo --password flink site write hello ./index.html
```

Publish a whole directory tree:

```sh
bin/flink --server http://localhost:8080 --tenant demo --password flink site write hello ./dist
```

Files are served from the same site base. For example, `./dist/assets/app.css` is available at:

```text
http://localhost:8080/t/demo/s/hello/assets/app.css
```

Directory indexes are served as expected, so `./dist/docs/index.html` is available at:

```text
http://localhost:8080/t/demo/s/hello/docs/
```

List sites:

```sh
bin/flink --server http://localhost:8080 --tenant demo --password flink site list
```

List or remove published files:

```sh
bin/flink --server http://localhost:8080 --tenant demo --password flink site files hello
bin/flink --server http://localhost:8080 --tenant demo --password flink site files hello assets/
bin/flink --server http://localhost:8080 --tenant demo --password flink site delete-file hello assets/app.css
```

To avoid repeating flags:

```sh
export FLINK_SERVER=http://localhost:8080
export FLINK_TENANT=demo
export FLINK_PASSWORD=flink

bin/flink site create hello
bin/flink site write hello ./index.html
```

## Guidance For AI Agents Building Sites

When an agent is asked to build and deploy a Flink-hosted website:

1. Get or infer the Flink server URL and tenant credentials.
2. Build the website as plain static files first, usually starting with `index.html`.
3. Use the Flink CLI to create the site and publish files.
4. Use `/flink.js` for backend features instead of creating a separate backend.
5. Store JSON state with Flink storage, uploaded file URLs with Flink uploads, and realtime messages with Flink rooms.
6. Keep the first deployed version usable. Add more files only when the prototype needs them.

Minimal publish loop:

```sh
bin/flink site create my-site
bin/flink site write my-site ./dist
```

Then open:

```text
$FLINK_SERVER/t/$FLINK_TENANT/s/my-site/
```

## Browser APIs For Hosted Sites

Every Flink-hosted site can import the shared browser library:

```html
<script src="/flink.js"></script>
```

From the browser, use Flink as the backend for the current tenant and site:

```ts
await flink.set("note", { text: "hello" });
const note = await flink.get("note");

const uploaded = await flink.upload(fileInput.files[0]);

const room = flink.room("chat", console.log);
room.send({ text: "hi" });

const ai = await flink.ai("Give me a prototype idea");
```

For TypeScript projects, use the client package:

```ts
import { createFlinkClient } from "@flink/client";

const flink = createFlinkClient({
  baseUrl: "http://localhost:8080",
  tenant: "demo",
  site: "hello",
});

await flink.storage.set("note", { text: "hello" });
const note = await flink.storage.get<{ text: string }>("note");

const uploaded = await flink.upload(file);
const body = await flink.uploads.text(uploaded);

const files = await flink.files.list();
await flink.files.write("assets/app.css", "body { color: red; }");
await flink.files.delete("old.html");

const room = flink.realtime.room<{ text: string }>("chat");
room.send({ text: "hi" });

const idea = await flink.ai({ prompt: "Give me a prototype idea" });
```

Available API areas:

```text
storage   JSON key-value/document APIs
files     hosted site file helpers
uploads   file upload plus URL/fetch/text/json/blob helpers
realtime  WebSocket room messaging
ai        optional server-side LLM endpoint
```

If AI is not configured on the server, AI calls return a stable "not configured" response instead of failing unpredictably.

## Run A Local Flink Server

For development or local demos:

```sh
make run
```

That creates `.flink/dev.yaml` on first run and starts the server at:

```text
http://localhost:8080
```

Manual equivalent:

```sh
go run ./server init --config flink.yaml
go run ./server --config flink.yaml
```

Example config:

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

Register at:

```text
http://localhost:8080/_flink/register
```

Approve a pending tenant:

```sh
go run ./server tenants pending --config flink.yaml
go run ./server tenants approve <tenant> --config flink.yaml
```

For local automation, bootstrap a ready-to-use tenant:

```yaml
bootstrapTenants:
  - username: demo
    password: flink
```

In high-trust environments, new tenants can be approved automatically:

```yaml
autoApproveTenants: true
```

## Host A Flink Server

The server is a single Go binary. It serves the dashboard, hosted sites, APIs, uploads, websocket rooms, and tenant sessions.

Install or update it on a VPS with the curlable installer:

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

Default production-style config:

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

Run the same installer command again to update the binary and restart the service.

For unreleased builds or private artifacts:

```sh
curl -fsSL https://raw.githubusercontent.com/csweichel/flink/main/server/extras/install.sh | sudo FLINK_DOWNLOAD_URL=https://example.com/flink-server_linux_amd64.tar.gz sh
```

Useful installer variables:

```text
FLINK_DOWNLOAD_URL       exact binary, .gz, or .tar.gz URL to install
FLINK_VERSION            GitHub release tag, defaults to latest
FLINK_REPO               GitHub repo for releases, defaults to csweichel/flink
FLINK_INSTALL_DATA_DIR   initial config data directory, defaults to /var/lib/flink
```

Put Flink behind Caddy, nginx, or another reverse proxy:

```caddyfile
flink.internal, *.flink.internal {
  reverse_proxy 127.0.0.1:8080
}
```

Path-based hosting works without wildcard DNS:

```text
https://flink.internal/t/alice/s/demo/
```

With wildcard DNS and `baseHost` configured, tenant site subdomains can be served as:

```text
https://alice--demo.flink.internal/
```

## Tenant Administration

Server operators use `flink-server`, not the user CLI:

```sh
flink-server tenants list --config /etc/flink/flink.yaml
flink-server tenants pending --config /etc/flink/flink.yaml
flink-server tenants get alice --config /etc/flink/flink.yaml
flink-server tenants approve alice --config /etc/flink/flink.yaml
flink-server tenants deny alice --config /etc/flink/flink.yaml
flink-server tenants reset-password alice new-secret --config /etc/flink/flink.yaml
flink-server tenants delete alice --config /etc/flink/flink.yaml
flink-server tenants bootstrap demo flink --config /etc/flink/flink.yaml
```

The web app uses tenant session cookies. The user CLI uses HTTP Basic Auth with the tenant username and password.

## Storage

Flink uses one storage abstraction for its own state and for user-facing APIs:

- tenants, approvals, and sessions
- site metadata
- hosted site files
- JSON key-value/document data
- uploaded files

Supported storage drivers:

```text
file    directory-backed storage under dataDir
bbolt   single-file embedded database at dataDir/flink.db
```

Configure storage in YAML:

```yaml
dataDir: /var/lib/flink
storage: bbolt
```

Future backends such as DynamoDB or Firebase should implement `server/storage.Backend` without changing `server/api`.

## Ona Development Environment

In Ona, start a ready-to-use Flink server:

```sh
gitpod automations service start flink
```

The service opens port `8080`, starts the real server, and bootstraps:

```text
username: demo
password: flink
```

It writes the config to `.flink/ona.yaml` and checks readiness by fetching `/flink.js` and `/api/sites` with the demo credentials.

## Repository Layout

```text
go.work               Go workspace linking separate modules
shared/               shared Go packages used by server and CLI
client/               TypeScript browser SDK package
cli/                  user CLI module
cli/main.go           user CLI entrypoint
cli/cmd/              user CLI Cobra commands and HTTP client
server/               server module
server/main.go        server entrypoint
server/cmd/           server Cobra commands
server/app/           HTTP app, auth, routing, frontend embedding
server/api/           sites, storage APIs, uploads, realtime, AI
server/storage/       storage abstraction and backends
server/frontend/      React/Vite/Tailwind dashboard
server/extras/        systemd unit and install/update script
.ona/automations.yaml Ona service and build/test tasks
```

## Build And Test

```sh
make build
make test
```

`make build` builds the TypeScript client, copies the browser bundle to `server/frontend/static/flink.js`, builds the dashboard, and writes:

```text
bin/flink
bin/flink-server
```

Focused checks:

```sh
go test ./shared/... ./cli/... ./server/...
cd client && npm test && npm run build
cd server/frontend && npm run typecheck && npm run build
```

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

Sign in to list sites, inspect hosted files, inspect JSON state, download or delete uploads, visit sites, delete sites, and download a complete site archive.

On a normal Flink server with wildcard DNS configured, open a published site at its tenant-scoped domain:

```text
https://<tenant>--<site>.<flink-base-host>/
```

For example:

```text
https://demo--hello.flink.internal/
```

Path-based hosting is only the fallback for local development or servers without `baseHost` configured: `https://<flink-server>/t/<tenant>/s/<site>/`.

## Publish With The User CLI

The `flink` CLI is for website authors and agents. It only talks to a running Flink server over HTTP.

Download the CLI from the latest release:

```sh
curl -L -o flink.tar.gz https://github.com/csweichel/flink/releases/latest/download/flink_linux_amd64.tar.gz
tar -xzf flink.tar.gz
```

Create and publish a site:

```sh
bin/flink --server https://flink.internal --tenant demo --password flink site create hello
bin/flink --server https://flink.internal --tenant demo --password flink site write hello ./index.html
```

Publish a whole directory tree:

```sh
bin/flink --server https://flink.internal --tenant demo --password flink site write hello ./dist
```

Files are served from the same site base. For example, `./dist/assets/app.css` is available at:

```text
https://demo--hello.flink.internal/assets/app.css
```

Directory indexes are served as expected, so `./dist/docs/index.html` is available at:

```text
https://demo--hello.flink.internal/docs/
```

When running locally on `localhost` without wildcard DNS, use the fallback path form instead: `http://localhost:8080/t/demo/s/hello/`.

List sites:

```sh
bin/flink --server https://flink.internal --tenant demo --password flink site list
```

Publish a built-in example:

```sh
bin/flink --server https://flink.internal --tenant demo --password flink site example
bin/flink --server https://flink.internal --tenant demo --password flink site example hello chat
```

List or remove published files:

```sh
bin/flink --server https://flink.internal --tenant demo --password flink site files hello
bin/flink --server https://flink.internal --tenant demo --password flink site files hello assets/
bin/flink --server https://flink.internal --tenant demo --password flink site delete-file hello assets/app.css
```

To avoid repeating flags:

```sh
export FLINK_SERVER=https://flink.internal
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
https://$FLINK_TENANT--my-site.<flink-base-host>/
```

If the server has no `baseHost`, use the fallback URL printed by the CLI.

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
  baseUrl: "https://flink.internal",
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

For development or local demos, `localhost` usually has no wildcard DNS. That means local runs use path-based fallback URLs unless you add your own wildcard DNS entry.

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

For shared environments, set `baseHost` to the wildcard domain and route both the base domain and wildcard subdomains to the Flink server.

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

Install or update it as the current Unix user:

```sh
curl -fsSL https://raw.githubusercontent.com/csweichel/flink/main/server/extras/install.sh | sh
```

The installer writes:

```text
~/.local/bin/flink-server                 server binary
~/.config/flink/flink.yaml                server config, created only if missing
~/.local/share/flink/data                 default data directory
~/.config/systemd/user/flink.service      user systemd unit
```

Default production-style config:

```yaml
addr: :8080
dataDir: /home/alice/.local/share/flink/data
storage: bbolt
baseHost: flink.internal
autoApproveTenants: false
ai:
  apiKey: ""
  baseURL: https://api.openai.com/v1
  model: gpt-4.1-mini
bootstrapTenants: []
```

Run the same installer command again to update the binary and restart the user service.

For unreleased builds or private artifacts:

```sh
curl -fsSL https://raw.githubusercontent.com/csweichel/flink/main/server/extras/install.sh | FLINK_DOWNLOAD_URL=https://example.com/flink-server_linux_amd64.tar.gz sh
```

Useful installer variables:

```text
FLINK_DOWNLOAD_URL       exact binary, .gz, or .tar.gz URL to install
FLINK_VERSION            GitHub release tag, defaults to latest
FLINK_REPO               GitHub repo for releases, defaults to csweichel/flink
FLINK_INSTALL_BIN_DIR    binary directory, defaults to ~/.local/bin
FLINK_INSTALL_CONFIG_DIR config directory, defaults to ~/.config/flink
FLINK_INSTALL_DATA_DIR   initial config data directory, defaults to ~/.local/share/flink/data
FLINK_INSTALL_BASE_HOST  initial wildcard site domain, defaults to flink.internal
```

Control the service with user systemd:

```sh
systemctl --user status flink
systemctl --user restart flink
journalctl --user -u flink -f
```

If the server should start at boot without an active login session, enable lingering for the Flink Unix user if your host allows it:

```sh
loginctl enable-linger "$USER"
```

Put Flink behind Caddy, nginx, or another reverse proxy that routes both the base domain and wildcard subdomains to `127.0.0.1:8080`:

```caddyfile
flink.internal, *.flink.internal {
  reverse_proxy 127.0.0.1:8080
}
```

With wildcard DNS and `baseHost` configured, tenant site domains are served as:

```text
https://alice--demo.flink.internal/
```

Path-based hosting works without wildcard DNS, but treat it as a fallback:

```text
https://flink.internal/t/alice/s/demo/
```

## Tenant Administration

Server operators use `flink-server`, not the user CLI:

```sh
flink-server tenants list --config ~/.config/flink/flink.yaml
flink-server tenants pending --config ~/.config/flink/flink.yaml
flink-server tenants get alice --config ~/.config/flink/flink.yaml
flink-server tenants approve alice --config ~/.config/flink/flink.yaml
flink-server tenants deny alice --config ~/.config/flink/flink.yaml
flink-server tenants reset-password alice new-secret --config ~/.config/flink/flink.yaml
flink-server tenants delete alice --config ~/.config/flink/flink.yaml
flink-server tenants bootstrap demo flink --config ~/.config/flink/flink.yaml
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
dataDir: /home/alice/.local/share/flink/data
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
.goreleaser.yaml      release archives for the CLI and server
.github/workflows/    GitHub Actions release workflow
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

## Release

Tagged releases use GoReleaser:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The GitHub workflow builds `flink` and `flink-server` for Linux and macOS on amd64 and arm64. Server archives are named `flink-server_<os>_<arch>.tar.gz` and include `server/extras` as `extras/`, which keeps the curlable installer URL format stable.

Manual workflow runs build a local snapshot without publishing a GitHub release.

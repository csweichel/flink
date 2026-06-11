# Flink CLI Overhaul Spec

## Objective

Overhaul the Flink user CLI so it is simple, use-case oriented, and effective for agents and humans. The CLI should make the primary loop feel like: save local HTML/assets, run one command, get a working URL.

This plan covers the selected improvements:

1. Magic publishing command.
2. Site metadata and publish history with rollback.
3. Dashboard catalog and discovery.
4. First-class prototype templates.
5. Browser SDK as the product API center.
6. Capability indicators per site.
8. Static snapshots.

Usage and implementation should preserve Flink's current product bias: developer joy, fast iteration, tenant-aware safety, and no heavyweight platform machinery.

## Current State

The CLI currently exposes most user workflows under `flink site ...`:

- `flink site create <slug>`
- `flink site write <slug> <local-file-or-dir> [site-path]`
- `flink site list`
- `flink site auth ...`
- `flink site files ...`
- `flink site delete-file ...`
- `flink site delete ...`
- `flink site example ...`

The server already supports:

- tenant-authenticated site CRUD through `/api/sites`
- per-site file APIs through `/api/sites/{slug}/files`
- per-site archive download through `/api/sites/{slug}/archive`
- per-site auth policies
- browser APIs for data, uploads, realtime, and AI through `/flink.js`
- a basic dashboard at `/_flink`

Missing pieces for the requested overhaul:

- one-command create-or-update publishing
- local CLI config/profile persistence
- site ownership/publish metadata beyond timestamps
- publish history and rollback
- top-level use-case-oriented command names
- bundled templates
- catalog/discovery affordances
- site capability detection
- static snapshot export workflow

## Requirements

### R1: Use-Case-Oriented CLI

The CLI must make common workflows top-level and self-explanatory.

Required commands:

```sh
flink publish [path] [--site <slug>] [--title <title>] [--public|--owner|--tenants <tenant,...>]
flink init [template] [path] [--site <slug>]
flink open [site]
flink list
flink inspect [site]
flink history <site>
flink rollback <site> [version]
flink snapshot <site> [path]
flink auth <site> owner|none|tenants [tenant...]
flink config set server <url>
flink config set tenant <username>
flink config login
```

This is a clean-break redesign. The existing `flink site ...` namespace should be removed or replaced outright; no backward compatibility is required because the product is not yet in use.

### R2: Magic Publish

`flink publish` must be the default happy path.

Behavior:

- Default path is the current directory.
- If `--site` is omitted, infer the site slug in this order:
  - existing `.flink/site.json`
  - directory basename normalized to a valid slug
  - explicit prompt in interactive terminals
- If the site does not exist, create it.
- If the path is a file, publish it as `index.html` when the filename is HTML and no explicit target path is provided; otherwise publish to the file basename.
- If the path is a directory, publish all files recursively.
- Ignore generated/local directories by default: `.git`, `.flink`, `node_modules`, `dist` only when publishing a parent project unintentionally, and common OS junk files.
- Print a concise result with file count, site slug, auth mode, and canonical URL.
- Exit non-zero with actionable errors.

Example:

```sh
$ flink publish ./prototype
created site checkout-demo
published 8 files
url https://demo--checkout-demo.flink.internal/
```

### R3: Agent-Friendly Operation

All primary commands must work non-interactively.

Requirements:

- Every interactive prompt must have a flag/env/config equivalent.
- Commands must support `--json` for machine-readable output.
- Error messages must include the failing operation and the next action when possible.
- `flink init` templates must be fully usable after generation.
- `flink publish` must not require a build step.
- CLI output must be stable enough for agents to parse.

Environment variables remain supported:

- `FLINK_SERVER`
- `FLINK_TENANT` / `FLINK_USERNAME`
- `FLINK_PASSWORD`

Local config should reduce repeated flags for humans.

### R4: CLI Config And Login

Add local user config for server and tenant defaults.

Suggested locations:

- project config: `.flink/site.json`
- user config: OS config dir, e.g. `~/.config/flink/config.json`

Project config should include:

```json
{
  "site": "checkout-demo",
  "server": "https://flink.internal",
  "tenant": "demo"
}
```

User config should include server and tenant defaults. Password storage should be conservative:

- Do not write passwords to project config.
- Prefer environment variable or interactive prompt.
- If password persistence is added, keep it user-scoped only and document the storage behavior.

### R5: Site Metadata

Extend site metadata so users and operators can understand provenance.

Add fields to site metadata:

- `createdBy`
- `updatedBy`
- `lastPublishedBy`
- `lastPublishedAt`
- `lastPublishedFrom`
- `lastGitCommit`
- `fileCount`
- `totalBytes`
- `capabilities`

`createdBy`, `updatedBy`, and `lastPublishedBy` are tenant usernames for now. Do not introduce separate user identities unless the auth model changes later.

`lastPublishedFrom` may be a sanitized local path basename or declared source label; it must not leak sensitive absolute paths in shared metadata.

### R6: Publish History And Rollback

Add publish versions per site.

Each successful directory or file publish creates a publish record containing:

- version id
- timestamp
- publishing tenant
- source label
- optional git commit
- file manifest with path, size, and content hash
- site auth mode at publish time

Rollback must restore hosted files to a previous publish version.

Rollback behavior:

- `flink history <site>` lists versions newest first.
- `flink rollback <site>` rolls back to the previous version.
- `flink rollback <site> <version>` rolls back to a specific version.
- Rollback creates a new publish record with `rollbackOf`.
- Data, uploads, and realtime state are not rolled back.

Implementation should store history through `server/storage.Backend`, not direct filesystem paths.

### R7: Dashboard Catalog And Discovery

Improve `/_flink` from a tenant-only site table into a useful catalog.

Required views:

- My sites.
- Public/open sites visible to anonymous users.
- Tenant-visible sites when authenticated and allowed by auth policy.
- Recently updated sites.
- Template/examples area.

Each site row/card should show:

- slug/title
- owner tenant
- access mode
- updated/published time
- capability indicators
- file count/total size
- actions: visit, inspect, download archive, copy publish command

Catalog access must respect existing site auth policies. It may show metadata for public sites and tenant-visible sites, but owner-only sites from other tenants must not leak sensitive details.

### R8: First-Class Templates

Add built-in templates optimized for humans and agents.

Required templates:

- `blank`: minimal single-file HTML with `/flink.js`.
- `todo`: storage API example.
- `chat`: realtime room example.
- `dashboard`: JSON state and simple data display.
- `upload-gallery`: uploads API example.
- `ai-tool`: AI endpoint example with graceful unconfigured state.
- `multiplayer`: realtime presence/cursor or simple shared interaction.

Template requirements:

- Generated output works as static files without a build step.
- TypeScript is not required inside hosted prototype templates; the repository frontend remains TypeScript.
- Each template uses `/flink.js` where relevant.
- Templates must be visually usable, not placeholder-only.
- Templates should avoid external CDN dependencies unless clearly justified.

`flink init <template>` should create files locally and optionally write `.flink/site.json`.

### R9: Browser SDK Center Of Gravity

Keep `/flink.js` and the `client/` package as the primary API surface for hosted prototypes.

SDK improvements to plan alongside CLI changes:

- Keep friendly aliases: `get`, `set`, `upload`, `room`, `ai`.
- Document copy-paste examples in generated templates and dashboard snippets.
- Expose enough metadata helpers for templates to display current tenant/site.
- Preserve dependency-light TypeScript package.
- Maintain browser-global usage:

```html
<script src="/flink.js"></script>
```

The CLI should point users to SDK examples after `flink init` and `flink inspect`, without making the CLI itself a documentation wall.

### R10: Capability Indicators

Track and display what each site uses.

Capability values:

- `files`
- `storage`
- `uploads`
- `realtime`
- `ai`
- `public`
- `tenant-restricted`
- `owner-only`

Detection rules:

- `files`: site has hosted files.
- `storage`: site data collection has keys, or hosted files reference storage APIs.
- `uploads`: upload collection has files, or hosted files reference upload APIs.
- `realtime`: hosted files reference realtime/room/websocket APIs.
- `ai`: hosted files reference AI API.
- access capabilities derive from auth policy.

Static file scanning should be heuristic and cheap. It must not execute user code.

### R11: Static Snapshots

Add `flink snapshot`.

Use cases:

- export a static copy for demos
- archive a prototype
- share a frozen version without writable APIs

Behavior:

- `flink snapshot <site>` downloads hosted files into `./<site>-snapshot/` by default.
- `flink snapshot <site> <path>` writes to the provided directory or archive path.
- Snapshot exports hosted files only.
- Snapshot does not export site data, uploads, auth cookies, AI credentials, or live realtime behavior.
- Optional `--zip` may produce a zip archive.
- Snapshot output should include a small `flink-snapshot.json` manifest with source site, tenant, timestamp, and file list.

Do not add public internet publishing as part of this scope.

## Constraints

- Keep `server` and `cli` as separate Go modules in the workspace.
- Keep Cobra entrypoints thin; command behavior belongs in focused files/helpers.
- Use `server/storage.Backend` for all durable Flink state.
- Preserve tenant scoping for all user-facing state.
- Do not bypass existing auth policy checks.
- Do not introduce per-site backends, job runners, cron, custom databases, or platform orchestration.
- Do not make templates require npm, Vite, React, or a bundler.
- No compatibility with existing `flink site ...` commands is required.
- Do not run `make test` and `make build` in parallel.
- Frontend dashboard code must remain React + Vite + Tailwind + TypeScript.
- SDK changes must remain dependency-light.

## Proposed Architecture

### CLI Command Layer

Replace the current nested `flink site ...` command tree with top-level commands in `cli/cmd`:

- `publish.go`
- `init.go`
- `open.go`
- `list.go`
- `inspect.go`
- `history.go`
- `rollback.go`
- `snapshot.go`
- `auth.go`
- `config.go`

Remove `siteCommand` and the old command files once their behavior is represented by the new use-case-oriented commands. Tests should target the new command names only.

Shared CLI helpers:

- config loading/merging from flags, env, project config, and user config
- slug inference and normalization
- local file discovery and ignore rules
- publish manifest generation
- URL formatting using server discovery
- JSON output formatting

### Server API

Add or extend APIs:

- `GET /api/sites` returns richer metadata for the authenticated tenant.
- `GET /api/sites/{slug}` returns one site metadata object and computed details.
- `POST /api/sites/{slug}/publishes` records publish metadata.
- `GET /api/sites/{slug}/publishes` lists publish history.
- `POST /api/sites/{slug}/rollback` restores files from a publish version.
- `GET /api/catalog` lists discoverable sites according to auth policy.

Existing file upload APIs may continue to handle file writes. A later implementation can add a batch publish endpoint if performance becomes a problem, but the first implementation can keep the current per-file PUT behavior and record one publish at the end.

### Storage Model

Existing collections remain:

- `tenants/{tenant}/site-meta`
- `tenants/{tenant}/sites/{slug}/files`
- `tenants/{tenant}/sites/{slug}/data`
- `tenants/{tenant}/sites/{slug}/uploads`

Add collections:

- `tenants/{tenant}/sites/{slug}/publishes`
- `tenants/{tenant}/sites/{slug}/publish-files/{version}`

Publish records should store metadata and manifests. File snapshots for rollback can initially store full file bytes per version for simplicity. If storage pressure becomes a problem later, add content-addressed blobs as an optimization.

### Dashboard

Extend `server/frontend/src/main.tsx` or split it into components if the file becomes unwieldy.

Dashboard should consume catalog and details APIs rather than reconstructing everything from multiple low-level endpoints when possible.

### Templates

Store templates in the CLI module as embedded files.

Suggested path:

- `cli/templates/{template}/index.html`
- optional assets under each template directory

Use Go `embed` to keep the distributed CLI self-contained.

## Implementation Steps

1. Replace the CLI command tree with the new top-level command model.
   - Implement config precedence: flags, env, project config, user config, defaults.
   - Add `flink list`, `flink auth`, and `flink open` as direct primary commands.
   - Remove `flink site ...` commands and their compatibility tests.

2. Implement `flink publish`.
   - Add slug inference and `.flink/site.json`.
   - Add local file walking and ignore rules.
   - Create site if missing.
   - Publish files through existing file API.
   - Print canonical URL and support `--json`.

3. Add templates and `flink init`.
   - Embed required templates.
   - Generate local files and optional `.flink/site.json`.
   - Ensure every template works with a direct `flink publish`.

4. Extend site metadata and capability computation.
   - Add metadata fields to API/store types.
   - Compute file count and total bytes.
   - Add capability detection from auth policy, collections, and static file heuristics.
   - Update CLI list/inspect output.

5. Add publish history and rollback.
   - Store publish records and manifests.
   - Record publish history from `flink publish`.
   - Implement history and rollback APIs.
   - Add `flink history` and `flink rollback`.

6. Add catalog/dashboard improvements.
   - Add catalog API respecting auth policies.
   - Update dashboard views for My Sites, discoverable sites, recent sites, and templates.
   - Show capability indicators and publish metadata.

7. Add snapshots.
   - Implement `flink snapshot` using file list/read or archive API.
   - Write snapshot manifest.
   - Add optional zip output if straightforward.

8. Tighten SDK-centered examples and docs snippets.
   - Update templates to use the preferred SDK APIs.
   - Add inspect output snippets.
   - Verify `/flink.js` still works for all template examples.

## Test Plan

Focused CLI tests:

- config precedence
- slug inference
- file walking and ignore rules
- `publish` creates a missing site
- `publish` updates an existing site
- `publish --json` output shape
- `init` writes expected templates
- `history` and `rollback` command output
- `snapshot` directory and manifest output

Server tests:

- metadata fields are set and updated correctly
- publish history records are tenant-scoped
- rollback restores files and creates a new publish record
- catalog does not leak owner-only sites across tenants
- capability detection handles files/data/uploads/auth policies

Frontend checks:

```sh
cd server/frontend && npm run typecheck && npm run build
```

SDK checks:

```sh
cd client && npm test && npm run build
```

Go checks:

```sh
go test ./cli/cmd ./server/app ./server/api ./server/storage
```

Broader checks when practical:

```sh
make test
make build
```

Do not run `make test` and `make build` in parallel.

## Success Criteria

- A new user can run `flink init todo`, `flink publish`, and receive a working URL without reading server internals.
- An agent can publish non-interactively with flags/env/config and parse `--json` output.
- The old `flink site ...` namespace is gone, and the new top-level commands cover the same core user jobs with simpler names.
- Site list/inspect/dashboard views show provenance, access mode, URL, file count, size, and capabilities.
- Every successful publish records history.
- `flink rollback <site>` restores the previous hosted files without changing site data or uploads.
- `flink snapshot <site>` exports a static copy with a manifest.
- Templates are usable, visually coherent, and exercise storage, uploads, realtime, and AI where appropriate.
- All new durable state goes through `server/storage.Backend`.
- Tenant isolation and existing site auth semantics are preserved.

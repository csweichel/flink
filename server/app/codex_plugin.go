package app

import (
	"fmt"
	"net/http"
)

func (a *App) handleCodexPluginScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, codexPluginScript, requestOrigin(r))
}

const codexPluginScript = `#!/bin/sh
set -eu

: "${FLINK_SERVER:=%s}"

missing=""
if [ -z "${FLINK_SERVER:-}" ]; then missing="$missing FLINK_SERVER"; fi
if [ -z "${FLINK_TENANT:-}" ]; then missing="$missing FLINK_TENANT"; fi
if [ -z "${FLINK_PASSWORD:-}" ]; then missing="$missing FLINK_PASSWORD"; fi
if [ -z "${CODEX_HOME:-}" ] && [ -z "${HOME:-}" ]; then missing="$missing CODEX_HOME_or_HOME"; fi
if [ -n "$missing" ]; then
  printf 'Missing required environment variables:%%s\n' "$missing" >&2
  printf 'Example:\n' >&2
  printf '  export FLINK_TENANT=demo\n' >&2
  printf '  export FLINK_PASSWORD=<your-password>\n' >&2
  printf '  curl -fsSL %%s/_flink/codex-plugin.sh | sh\n' "$FLINK_SERVER" >&2
  exit 1
fi
if [ -z "${CODEX_HOME:-}" ]; then CODEX_HOME="$HOME/.codex"; fi

FLINK_MCP_URL="${FLINK_SERVER%%/}/mcp"
FLINK_AUTH="$(printf '%%s' "${FLINK_TENANT}:${FLINK_PASSWORD}" | base64 | tr -d '\n')"
PLUGIN_DIR="$CODEX_HOME/plugins/flink"

mkdir -p "$PLUGIN_DIR/.codex-plugin" "$PLUGIN_DIR/skills/flink"

cat > "$PLUGIN_DIR/.codex-plugin/plugin.json" <<'JSON'
{
  "name": "flink",
  "version": "0.1.0",
  "description": "Codex helpers for publishing and managing Flink sites."
}
JSON

cat > "$PLUGIN_DIR/skills/flink/SKILL.md" <<EOF
# Flink

Use this skill when the user asks to publish, inspect, update, configure, or rollback Flink sites on this server.

## Server

- Flink server: ${FLINK_SERVER}
- MCP endpoint: ${FLINK_MCP_URL}
- Tenant: ${FLINK_TENANT}
- Auth: HTTP Basic Auth with tenant username and password.

## Rules

- Use the configured Flink MCP server for site operations.
- Never put tenant passwords, Basic Auth headers, API keys, or other secrets into hosted browser files.
- Keep sites owner-only unless the user explicitly asks to share them.
- Use flink_publish_site for new publishes, then verify the returned URL.
- Use flink_get_site and flink_read_file before editing an existing site.
- Use flink_set_site_auth only when the user asks to change access.

## Common Flows

Publish a site:
1. Prepare static files.
2. Call flink_publish_site with the site slug and file list.
3. Open or fetch the returned URL to verify the page loads.

Update a site:
1. Call flink_get_site.
2. Read the files you need.
3. Write or publish updated files.
4. Verify the live URL.

Configure access:
- owner: only this tenant can view.
- none: anonymous viewers can view and use allowed browser APIs.
- tenants: approved tenants can view, optionally restricted to a tenant allow-list.
EOF

cat > "$PLUGIN_DIR/mcp.config.json" <<EOF
{
  "mcpServers": {
    "flink": {
      "type": "http",
      "url": "${FLINK_MCP_URL}",
      "headers": {
        "Authorization": "Basic ${FLINK_AUTH}"
      }
    }
  }
}
EOF

cat > "$PLUGIN_DIR/MCP.md" <<EOF
Flink MCP endpoint
${FLINK_MCP_URL}

Authentication
HTTP Basic Auth
username: ${FLINK_TENANT}
password: loaded from FLINK_PASSWORD when this plugin scaffold was generated

Local MCP config was written to:
${PLUGIN_DIR}/mcp.config.json

Available tools include:
- flink_list_sites
- flink_get_site
- flink_publish_site
- flink_read_file
- flink_write_file
- flink_delete_file
- flink_set_site_auth
- flink_get_site_data
- flink_set_site_data
- flink_delete_site_data
- flink_list_publishes
- flink_rollback_site
EOF

printf 'Installed Flink Codex plugin in %%s\n' "$PLUGIN_DIR"
printf 'MCP config written to %%s\n' "$PLUGIN_DIR/mcp.config.json"
`

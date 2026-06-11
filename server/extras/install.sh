#!/bin/sh
set -eu

SERVICE_NAME="${FLINK_INSTALL_SERVICE_NAME:-flink}"
XDG_BIN_HOME="${XDG_BIN_HOME:-$HOME/.local/bin}"
XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
BIN_DIR="${FLINK_INSTALL_BIN_DIR:-$XDG_BIN_HOME}"
INSTALL_DIR="${FLINK_INSTALL_DIR:-$XDG_DATA_HOME/flink}"
CONFIG_DIR="${FLINK_INSTALL_CONFIG_DIR:-$XDG_CONFIG_HOME/flink}"
DATA_DIR="${FLINK_INSTALL_DATA_DIR:-$XDG_DATA_HOME/flink/data}"
SYSTEMD_USER_DIR="${FLINK_INSTALL_SYSTEMD_USER_DIR:-$XDG_CONFIG_HOME/systemd/user}"
BASE_HOST="${FLINK_INSTALL_BASE_HOST:-flink.internal}"
CONFIG_FILE="${FLINK_INSTALL_CONFIG_FILE:-$CONFIG_DIR/flink.yaml}"
REPO="${FLINK_REPO:-csweichel/flink}"
VERSION="${FLINK_VERSION:-latest}"
DOWNLOAD_URL="${FLINK_DOWNLOAD_URL:-}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

detect_os() {
  uname -s | tr '[:upper:]' '[:lower:]'
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
  esac
}

download_url() {
  if [ -n "$DOWNLOAD_URL" ]; then
    echo "$DOWNLOAD_URL"
    return
  fi
  os="$(detect_os)"
  arch="$(detect_arch)"
  if [ "$VERSION" = "latest" ]; then
    version_path="latest"
  else
    version_path="download/$VERSION"
  fi
  echo "https://github.com/$REPO/releases/$version_path/download/flink-server_${os}_${arch}.tar.gz"
}

install_binary() {
  url="$(download_url)"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT INT TERM

  echo "downloading $url"
  curl -fsSL "$url" -o "$tmpdir/flink-server.download"

  case "$url" in
    *.tar.gz|*.tgz)
      tar -xzf "$tmpdir/flink-server.download" -C "$tmpdir"
      binary="$(find "$tmpdir" -type f -name flink-server | head -n 1)"
      ;;
    *.gz)
      gzip -dc "$tmpdir/flink-server.download" > "$tmpdir/flink-server"
      binary="$tmpdir/flink-server"
      ;;
    *)
      binary="$tmpdir/flink-server.download"
      ;;
  esac

  if [ ! -s "$binary" ]; then
    echo "download did not contain a flink-server binary" >&2
    exit 1
  fi

  install -d -m 0755 "$BIN_DIR" "$INSTALL_DIR"
  install -m 0755 "$binary" "$BIN_DIR/flink-server"
}

install_config() {
  install -d -m 0755 "$CONFIG_DIR" "$DATA_DIR"

  if [ ! -f "$CONFIG_FILE" ]; then
    cat > "$CONFIG_FILE" <<EOF
# Flink server configuration
addr: :8080
dataDir: $DATA_DIR
storage: bbolt
baseHost: "$BASE_HOST"
autoApproveTenants: false
ai:
  apiKey: ""
  baseURL: https://api.openai.com/v1
  model: gpt-4.1-mini
bootstrapTenants: []
EOF
    chmod 0600 "$CONFIG_FILE"
  else
    echo "keeping existing config $CONFIG_FILE"
  fi
}

install_unit() {
  install -d -m 0755 "$SYSTEMD_USER_DIR"
  cat > "$SYSTEMD_USER_DIR/$SERVICE_NAME.service" <<EOF
[Unit]
Description=Flink internal prototyping server
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$BIN_DIR/flink-server --config $CONFIG_FILE
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=default.target
EOF
}

start_service() {
  systemctl --user daemon-reload
  systemctl --user enable "$SERVICE_NAME.service" >/dev/null
  if [ "${FLINK_INSTALL_SKIP_START:-}" = "1" ]; then
    echo "installed user service $SERVICE_NAME.service; start skipped because FLINK_INSTALL_SKIP_START=1"
    return
  fi
  systemctl --user restart "$SERVICE_NAME.service"
  systemctl --user --no-pager --full status "$SERVICE_NAME.service" || true
}

main() {
  need curl
  need tar
  need gzip
  need find
  need systemctl
  install_binary
  install_config
  install_unit
  start_service
  echo "Flink installed for user $(id -un)."
  echo "Binary:  $BIN_DIR/flink-server"
  echo "Config:  $CONFIG_FILE"
  echo "Data:    $DATA_DIR"
  echo "Service: systemctl --user status $SERVICE_NAME.service"
  echo "For boot without an active login session, enable lingering for this user if your host allows it."
}

main "$@"

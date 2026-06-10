#!/bin/sh
set -eu

SERVICE_NAME="${FLINK_INSTALL_SERVICE_NAME:-flink}"
INSTALL_DIR="${FLINK_INSTALL_DIR:-/opt/flink}"
CONFIG_DIR="${FLINK_INSTALL_CONFIG_DIR:-/etc/flink}"
DATA_DIR="${FLINK_INSTALL_DATA_DIR:-/var/lib/flink}"
CONFIG_FILE="${FLINK_INSTALL_CONFIG_FILE:-$CONFIG_DIR/flink.yaml}"
USER_NAME="${FLINK_INSTALL_USER:-flink}"
GROUP_NAME="${FLINK_INSTALL_GROUP:-flink}"
REPO="${FLINK_REPO:-csweichel/flink}"
VERSION="${FLINK_VERSION:-latest}"
DOWNLOAD_URL="${FLINK_DOWNLOAD_URL:-}"

as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  else
    sudo "$@"
  fi
}

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

install_user() {
  if ! getent group "$GROUP_NAME" >/dev/null 2>&1; then
    as_root groupadd --system "$GROUP_NAME"
  fi
  if ! id "$USER_NAME" >/dev/null 2>&1; then
    as_root useradd --system --home "$INSTALL_DIR" --shell /usr/sbin/nologin --gid "$GROUP_NAME" "$USER_NAME"
  fi
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

  as_root install -d -m 0755 "$INSTALL_DIR"
  as_root install -m 0755 "$binary" "$INSTALL_DIR/flink-server"
  as_root chown root:root "$INSTALL_DIR/flink-server"
}

install_config() {
  as_root install -d -m 0755 "$CONFIG_DIR" "$DATA_DIR"
  as_root chown "$USER_NAME:$GROUP_NAME" "$DATA_DIR"

  if [ ! -f "$CONFIG_FILE" ]; then
    as_root sh -c "cat > '$CONFIG_FILE'" <<EOF
# Flink server configuration
addr: :8080
dataDir: $DATA_DIR
storage: bbolt
baseHost: ""
autoApproveTenants: false
ai:
  apiKey: ""
  baseURL: https://api.openai.com/v1
  model: gpt-4.1-mini
bootstrapTenants: []
EOF
    as_root chown root:"$GROUP_NAME" "$CONFIG_FILE"
    as_root chmod 0640 "$CONFIG_FILE"
  else
    echo "keeping existing config $CONFIG_FILE"
  fi
}

install_unit() {
  as_root sh -c "cat > '/etc/systemd/system/$SERVICE_NAME.service'" <<EOF
[Unit]
Description=Flink internal prototyping server
Documentation=https://github.com/$REPO
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$USER_NAME
Group=$GROUP_NAME
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/flink-server --config $CONFIG_FILE
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=$DATA_DIR $CONFIG_DIR $INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF
}

start_service() {
  as_root systemctl daemon-reload
  as_root systemctl enable "$SERVICE_NAME.service" >/dev/null
  if [ "${FLINK_INSTALL_SKIP_START:-}" = "1" ]; then
    echo "installed $SERVICE_NAME.service; start skipped because FLINK_INSTALL_SKIP_START=1"
    return
  fi
  as_root systemctl restart "$SERVICE_NAME.service"
  as_root systemctl --no-pager --full status "$SERVICE_NAME.service" || true
}

main() {
  need curl
  need tar
  need gzip
  need find
  need systemctl
  install_user
  install_binary
  install_config
  install_unit
  start_service
  echo "Flink installed."
  echo "Config: $CONFIG_FILE"
  echo "Data:   $DATA_DIR"
}

main "$@"

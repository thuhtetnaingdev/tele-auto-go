#!/usr/bin/env bash
set -Eeuo pipefail

APP_NAME="tele-auto"
APP_USER="${APP_USER:-teleauto}"
APP_GROUP="${APP_GROUP:-teleauto}"
INSTALL_DIR="${INSTALL_DIR:-/opt/tele-auto-go}"
SERVICE_NAME="${SERVICE_NAME:-tele-auto}"
REPO="${REPO:-thuhtetnaingdev/tele-auto-go}"
VERSION="${VERSION:-latest}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing required command: $1" >&2; exit 1; }
}

require_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    echo "Please run as root: sudo bash install.sh" >&2
    exit 1
  fi
}

detect_arch() {
  local machine
  machine="$(uname -m)"
  case "$machine" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "Unsupported architecture: $machine" >&2
      exit 1
      ;;
  esac
}

github_api_get() {
  local url="$1"
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" -H "Accept: application/vnd.github+json" "$url"
  else
    curl -fsSL -H "Accept: application/vnd.github+json" "$url"
  fi
}

get_release_asset_urls() {
  local tag="$1"
  local api_url="https://api.github.com/repos/${REPO}/releases/tags/${tag}"
  local body
  if ! body="$(github_api_get "$api_url" 2>/dev/null)"; then
    echo "Release tag not found: ${REPO}@${tag}" >&2
    echo "Create GitHub release and upload build artifacts first." >&2
    return 1
  fi

  echo "$body" | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

ensure_release_assets() {
  local tag="$1"
  local arch="$2"
  local expected_app expected_web urls
  expected_app="tele-auto-go_${tag}_linux_${arch}.tar.gz"
  expected_web="tele-auto-go-web_${tag}.tar.gz"

  urls="$(get_release_asset_urls "$tag")" || return 1
  if [[ -z "$urls" ]]; then
    echo "No assets found in release ${tag}." >&2
    echo "Expected assets:" >&2
    echo "  - ${expected_app}" >&2
    echo "  - ${expected_web}" >&2
    return 1
  fi

  if ! echo "$urls" | grep -q "/${expected_app}$"; then
    echo "Missing asset: ${expected_app}" >&2
    echo "Please upload it to release ${tag}." >&2
    return 1
  fi
  if ! echo "$urls" | grep -q "/${expected_web}$"; then
    echo "Missing asset: ${expected_web}" >&2
    echo "Please upload it to release ${tag}." >&2
    return 1
  fi
}

resolve_version() {
  if [[ "$VERSION" != "latest" ]]; then
    echo "$VERSION"
    return
  fi

  local latest_tag
  latest_tag="$(github_api_get "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  if [[ -z "$latest_tag" ]]; then
    echo "Unable to resolve latest release tag for ${REPO}" >&2
    echo "Set VERSION explicitly, e.g. VERSION=v1.0.0" >&2
    exit 1
  fi
  echo "$latest_tag"
}

resolve_cli_user_home() {
  local target_user target_home
  if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
    target_user="$SUDO_USER"
  else
    target_user="${USER:-root}"
  fi

  target_home="$(getent passwd "$target_user" | awk -F: '{print $6}')"
  if [[ -z "$target_home" ]]; then
    target_home="/root"
  fi
  echo "$target_home"
}

write_uninstall_script() {
  local cli_user_link="$1"
  cat > "$INSTALL_DIR/bin/uninstall.sh" <<UNINSTALL
#!/usr/bin/env bash
set -Eeuo pipefail

SERVICE_NAME="${SERVICE_NAME}"
INSTALL_DIR="${INSTALL_DIR}"
SYSTEM_LINK="/usr/local/bin/tele-auto"
USER_LINK="${cli_user_link}"

if [[ "\${1:-}" != "--yes" ]]; then
  echo "This will uninstall tele-auto from this server."
  echo "It will remove:"
  echo "  - systemd service: \${SERVICE_NAME}.service"
  echo "  - install dir: \${INSTALL_DIR}"
  echo "  - links: \${SYSTEM_LINK}, \${USER_LINK}"
  read -r -p "Type y to continue: " confirm
  if [[ "\${confirm}" != "y" ]]; then
    echo "Canceled."
    exit 1
  fi
fi

if [[ "\$(id -u)" -ne 0 ]]; then
  echo "Please run uninstall with sudo/root."
  exit 1
fi

systemctl stop "\${SERVICE_NAME}" 2>/dev/null || true
systemctl disable "\${SERVICE_NAME}" 2>/dev/null || true
rm -f "/etc/systemd/system/\${SERVICE_NAME}.service"
systemctl daemon-reload || true
systemctl reset-failed "\${SERVICE_NAME}" 2>/dev/null || true

rm -f "\${SYSTEM_LINK}" "\${USER_LINK}"
rm -rf "\${INSTALL_DIR}"

echo "tele-auto uninstalled."
UNINSTALL
  chmod +x "$INSTALL_DIR/bin/uninstall.sh"
}

write_cli_script() {
  cat > "$INSTALL_DIR/bin/tele-auto" <<CLI
#!/usr/bin/env bash
set -Eeuo pipefail

SERVICE_NAME="${SERVICE_NAME}"
INSTALL_DIR="${INSTALL_DIR}"

run_as_root() {
  if [[ "\$(id -u)" -eq 0 ]]; then
    "\$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "\$@"
    return
  fi
  echo "This command requires root. Please run with sudo."
  exit 1
}

usage() {
  cat <<EOF
Usage: tele-auto <command>

Commands:
  status       Show service status
  start        Start service
  stop         Stop service
  restart      Restart service
  logs         Tail service logs
  uninstall    Uninstall tele-auto (asks for confirmation)
EOF
}

cmd="\${1:-status}"
case "\${cmd}" in
  status)
    exec systemctl --no-pager --full status "\${SERVICE_NAME}"
    ;;
  restart)
    run_as_root systemctl restart "\${SERVICE_NAME}"
    ;;
  start)
    run_as_root systemctl start "\${SERVICE_NAME}"
    ;;
  stop)
    run_as_root systemctl stop "\${SERVICE_NAME}"
    ;;
  logs)
    exec journalctl -u "\${SERVICE_NAME}" -f
    ;;
  uninstall)
    shift || true
    run_as_root "\${INSTALL_DIR}/bin/uninstall.sh" "\$@"
    ;;
  *)
    usage
    exit 1
    ;;
esac
CLI
  chmod +x "$INSTALL_DIR/bin/tele-auto"
}

main() {
  require_root
  need_cmd curl
  need_cmd tar
  need_cmd systemctl
  need_cmd getent

  local arch release_tag base_url app_tar web_tar tmp_dir cli_user_home cli_user_link
  arch="$(detect_arch)"
  release_tag="$(resolve_version)"
  base_url="https://github.com/${REPO}/releases/download/${release_tag}"
  app_tar="tele-auto-go_${release_tag}_linux_${arch}.tar.gz"
  web_tar="tele-auto-go-web_${release_tag}.tar.gz"
  cli_user_home="$(resolve_cli_user_home)"
  cli_user_link="${cli_user_home}/.local/bin/tele-auto"

  echo "==> Installing ${APP_NAME}"
  echo "    repo: ${REPO}"
  echo "    version: ${release_tag}"
  echo "    arch: ${arch}"

  echo "==> Checking release assets"
  ensure_release_assets "$release_tag" "$arch"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  echo "==> Downloading artifacts"
  curl -fL "${base_url}/${app_tar}" -o "${tmp_dir}/${app_tar}"
  curl -fL "${base_url}/${web_tar}" -o "${tmp_dir}/${web_tar}"

  mkdir -p "${tmp_dir}/app"
  tar -xzf "${tmp_dir}/${app_tar}" -C "${tmp_dir}/app"

  getent group "$APP_GROUP" >/dev/null 2>&1 || groupadd --system "$APP_GROUP"
  id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --gid "$APP_GROUP" --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin "$APP_USER"

  echo "==> Preparing directories"
  mkdir -p "$INSTALL_DIR"/bin "$INSTALL_DIR"/web "$INSTALL_DIR"/etc "$INSTALL_DIR"/data "$INSTALL_DIR"/logs

  install -m 0755 "${tmp_dir}/app/bin/tele-auto-control" "$INSTALL_DIR/bin/tele-auto-control"
  rm -rf "$INSTALL_DIR/web"/*
  tar -xzf "${tmp_dir}/${web_tar}" -C "$INSTALL_DIR/web"
  write_uninstall_script "$cli_user_link"
  write_cli_script

  if [[ ! -f "$INSTALL_DIR/etc/tele-auto.env" ]]; then
    if [[ -f "${tmp_dir}/app/tele-auto.env.example" ]]; then
      cp "${tmp_dir}/app/tele-auto.env.example" "$INSTALL_DIR/etc/tele-auto.env"
    elif [[ -f "${tmp_dir}/app/.env.example" ]]; then
      cp "${tmp_dir}/app/.env.example" "$INSTALL_DIR/etc/tele-auto.env"
    else
      cat > "$INSTALL_DIR/etc/tele-auto.env" <<ENV
CONTROL_PORT=3000
PORT=3000
LOG_LEVEL=info
WEB_DIR=./web
TG_API_ID=
TG_API_HASH=
TG_PHONE=
TG_SESSION_FILE=./data/session.json
OPENAI_BASE_URL=
OPENAI_API_KEY=
OPENAI_MODEL=
AUTO_REPLY_ENABLED=true
SQLITE_PATH=./data/app.db
SOUL_PROMPT_PATH=./SOUL.md
ENV
    fi
    echo "==> Created $INSTALL_DIR/etc/tele-auto.env (please edit required values)"
  fi

  if [[ -f "${tmp_dir}/app/tele-auto.service" ]]; then
    sed \
      -e "s|__APP_USER__|${APP_USER}|g" \
      -e "s|__APP_GROUP__|${APP_GROUP}|g" \
      -e "s|__INSTALL_DIR__|${INSTALL_DIR}|g" \
      "${tmp_dir}/app/tele-auto.service" > "/etc/systemd/system/${SERVICE_NAME}.service"
  else
    cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<UNIT
[Unit]
Description=Tele Auto Control Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_GROUP}
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${INSTALL_DIR}/etc/tele-auto.env
ExecStart=${INSTALL_DIR}/bin/tele-auto-control
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
UNIT
  fi

  chown -R "$APP_USER:$APP_GROUP" "$INSTALL_DIR"

  echo "==> Linking tele-auto command"
  mkdir -p /usr/local/bin
  ln -sfn "$INSTALL_DIR/bin/tele-auto" /usr/local/bin/tele-auto
  mkdir -p "$(dirname "$cli_user_link")"
  ln -sfn "$INSTALL_DIR/bin/tele-auto" "$cli_user_link"
  if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
    chown -h "$SUDO_USER:$SUDO_USER" "$cli_user_link" || true
  fi

  echo "==> Reloading and starting systemd service"
  systemctl daemon-reload
  systemctl enable --now "${SERVICE_NAME}"

  echo "==> Service status"
  systemctl --no-pager --full status "${SERVICE_NAME}" || true

  echo
  echo "Install complete."
  echo "1) Edit env: ${INSTALL_DIR}/etc/tele-auto.env"
  echo "2) Restart: systemctl restart ${SERVICE_NAME}"
  echo "3) Health: curl http://127.0.0.1:3000/health"
  echo "4) CLI: tele-auto status | tele-auto uninstall"
}

main "$@"

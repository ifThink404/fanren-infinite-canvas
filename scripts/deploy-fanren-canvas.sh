#!/usr/bin/env bash
set -euo pipefail

# Fanren canvas deployment topology:
#   aliyun-8 (build) -> fr-netcup-new (container) -> DMIT (SSH tunnel/Nginx)
# The application is exposed at https://fanrenapi.com/creative/.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_HOST="${CANVAS_BUILD_HOST:-aliyun-8}"
BUILD_DIR="${CANVAS_BUILD_DIR:-/opt/docker/fanren-infinite-canvas-build}"
TARGET_HOST="${CANVAS_TARGET_HOST:-fr-netcup-new}"
TARGET_DIR="${CANVAS_TARGET_DIR:-/opt/ai-stack/apps/fanren/infinite-canvas}"
TARGET_PORT="${CANVAS_TARGET_PORT:-13011}"
GATEWAY_HOST="${CANVAS_GATEWAY_HOST:-fanren-dmit}"
GATEWAY_PORT="${CANVAS_GATEWAY_PORT:-23011}"
GATEWAY_TUNNEL_SERVICE="${CANVAS_GATEWAY_TUNNEL_SERVICE:-netcup-fanren-canvas-tunnel.service}"
GATEWAY_TUNNEL_TARGET="${CANVAS_GATEWAY_TUNNEL_TARGET:-root@159.195.17.9}"
GATEWAY_TUNNEL_TARGET_PORT="${CANVAS_GATEWAY_TUNNEL_TARGET_PORT:-2222}"
GATEWAY_TUNNEL_IDENTITY="${CANVAS_GATEWAY_TUNNEL_IDENTITY:-/root/.ssh/netcup_tunnel_ed25519}"
IMAGE_TRANSFER_TARGET="${CANVAS_IMAGE_TRANSFER_TARGET:-root@159.195.17.9}"
IMAGE_TRANSFER_PORT="${CANVAS_IMAGE_TRANSFER_PORT:-2222}"
IMAGE_TRANSFER_IDENTITY="${CANVAS_IMAGE_TRANSFER_IDENTITY:-/root/.ssh/fanren_netcup_image_transfer_ed25519}"
NGINX_CONF="${CANVAS_NGINX_CONF:-/etc/nginx/conf.d/fanrenapi.com.conf}"
RESTORE_DIR="${CANVAS_GATEWAY_RESTORE_DIR:-/opt/ai-stack-gateway/restore}"
PUBLIC_URL="${CANVAS_PUBLIC_URL:-https://fanrenapi.com/creative/}"
BASE_PATH="${CANVAS_BASE_PATH:-/creative}"
timestamp="$(date +%Y%m%d%H%M%S)"
IMAGE="${CANVAS_IMAGE:-fanren-infinite-canvas:${timestamp}}"
BUILD_RELEASE_DIR="${BUILD_DIR}/${timestamp}"

shell_quote() {
  printf "%q" "$1"
}

build_ssh() {
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=12 "$BUILD_HOST" "$@"
}

target_ssh() {
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=12 "$TARGET_HOST" "$@"
}

gateway_ssh() {
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=12 "$GATEWAY_HOST" "$@"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || { echo "缺少命令: $1" >&2; exit 1; }
}

validate_number() {
  [[ "$2" =~ ^[0-9]+$ ]] || { echo "$1 必须是数字: $2" >&2; exit 1; }
}

require_command git
require_command ssh
require_command date
validate_number CANVAS_TARGET_PORT "$TARGET_PORT"
validate_number CANVAS_GATEWAY_PORT "$GATEWAY_PORT"
validate_number CANVAS_GATEWAY_TUNNEL_TARGET_PORT "$GATEWAY_TUNNEL_TARGET_PORT"
validate_number CANVAS_IMAGE_TRANSFER_PORT "$IMAGE_TRANSFER_PORT"

cd "$REPO_ROOT"
if [[ "${CANVAS_ALLOW_DIRTY:-0}" != "1" ]] && [[ -n "$(git status --short)" ]]; then
  echo "工作树存在未提交修改；确认后可设置 CANVAS_ALLOW_DIRTY=1。" >&2
  git status --short >&2
  exit 1
fi

echo "[1/8] 检查构建机与目标服务器"
build_ssh "docker version >/dev/null"
target_ssh "docker version >/dev/null"
gateway_ssh "nginx -t >/dev/null"

echo "[2/8] 传输已提交源码到构建机 ${BUILD_HOST}:${BUILD_RELEASE_DIR}"
build_ssh "mkdir -p $(shell_quote "$BUILD_RELEASE_DIR")"
git archive --format=tar HEAD | gzip -1 | build_ssh "gzip -d | tar -xf - -C $(shell_quote "$BUILD_RELEASE_DIR")"

echo "[3/8] 在构建机生成镜像 ${IMAGE}"
build_ssh "set -e; cd $(shell_quote "$BUILD_RELEASE_DIR"); docker build --build-arg NEXT_PUBLIC_BASE_PATH=$(shell_quote "$BASE_PATH") -t $(shell_quote "$IMAGE") ."

echo "[4/8] 构建机直传镜像到 ${TARGET_HOST}"
build_ssh "set -o pipefail; docker save $(shell_quote "$IMAGE") | gzip -1 | ssh -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=12 -i $(shell_quote "$IMAGE_TRANSFER_IDENTITY") -p $(shell_quote "$IMAGE_TRANSFER_PORT") $(shell_quote "$IMAGE_TRANSFER_TARGET") fanren-image-transfer-load"
target_ssh "docker image inspect $(shell_quote "$IMAGE") >/dev/null"

echo "[5/8] 更新 Netcup 独立画布容器"
admin_password="${FANREN_CANVAS_ADMIN_PASSWORD:-}"
if [[ -z "$admin_password" ]]; then
  admin_password="$(openssl rand -base64 24 | tr -dc 'A-Za-z0-9@#%+=' | cut -c1-24)"
  [[ -n "$admin_password" ]] || { echo "无法生成画布管理员密码" >&2; exit 1; }
  echo "画布管理员账号: admin"
  echo "画布管理员初始密码: ${admin_password}"
  echo "该密码仅写入 Netcup 的 ${TARGET_DIR}/.env，不会写入 Git。"
fi
jwt_secret="${FANREN_CANVAS_JWT_SECRET:-$(openssl rand -hex 32)}"
env_content=$(cat <<EOF
ADMIN_USERNAME=admin
ADMIN_PASSWORD=${admin_password}
JWT_SECRET=${jwt_secret}
JWT_EXPIRE_HOURS=168
PUBLIC_BASE_URL=${PUBLIC_URL%/}
STORAGE_DRIVER=sqlite
DATABASE_DSN=/app/data/infinite-canvas.db
FANREN_IMAGE_JOB_POLL_SECONDS=${FANREN_IMAGE_JOB_POLL_SECONDS:-5}
FANREN_IMAGE_JOB_TIMEOUT_SECONDS=${FANREN_IMAGE_JOB_TIMEOUT_SECONDS:-1800}
EOF
)
compose_content=$(cat <<EOF
services:
  app:
    image: ${IMAGE}
    container_name: fanren-infinite-canvas
    restart: always
    env_file:
      - .env
    volumes:
      - ${TARGET_DIR}/data:/app/data
    ports:
      - 127.0.0.1:${TARGET_PORT}:3000
    healthcheck:
      test: ["CMD", "node", "-e", "fetch('http://127.0.0.1:3000/api/health').then(r => process.exit(r.ok ? 0 : 1)).catch(() => process.exit(1))"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF
)
target_ssh "mkdir -p $(shell_quote "$TARGET_DIR/data")"
printf '%s\n' "$env_content" | target_ssh "umask 077; cat > $(shell_quote "$TARGET_DIR/.env")"
printf '%s\n' "$compose_content" | target_ssh "cat > $(shell_quote "$TARGET_DIR/docker-compose.yml")"
target_ssh "cd $(shell_quote "$TARGET_DIR") && docker compose up -d --remove-orphans"
target_ssh "for i in \$(seq 1 30); do curl -fsS http://127.0.0.1:${TARGET_PORT}/api/health >/dev/null && exit 0; sleep 2; done; docker compose logs --tail=100; exit 1"

echo "[6/8] 建立 DMIT -> Netcup 持久隧道"
tunnel_content=$(cat <<EOF
[Unit]
Description=Persistent SSH tunnel from DMIT gateway to Fanren infinite canvas
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/ssh -NT -i ${GATEWAY_TUNNEL_IDENTITY} -p ${GATEWAY_TUNNEL_TARGET_PORT} -o IdentitiesOnly=yes -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -L 127.0.0.1:${GATEWAY_PORT}:127.0.0.1:${TARGET_PORT} ${GATEWAY_TUNNEL_TARGET}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
)
printf '%s\n' "$tunnel_content" | gateway_ssh "cat > /tmp/${GATEWAY_TUNNEL_SERVICE}"
gateway_ssh "install -m 0644 /tmp/${GATEWAY_TUNNEL_SERVICE} /etc/systemd/system/${GATEWAY_TUNNEL_SERVICE}; systemctl daemon-reload; systemctl enable --now ${GATEWAY_TUNNEL_SERVICE}; systemctl is-active --quiet ${GATEWAY_TUNNEL_SERVICE}"
gateway_ssh "for i in \$(seq 1 20); do curl -fsS http://127.0.0.1:${GATEWAY_PORT}/api/health >/dev/null && exit 0; sleep 2; done; systemctl status ${GATEWAY_TUNNEL_SERVICE} --no-pager; exit 1"

echo "[7/8] 在 DMIT 的 fanrenapi.com HTTPS server 增加独立 /creative/ 路由"
backup_path="${RESTORE_DIR}/fanren-canvas-${timestamp}.nginx.conf"
gateway_ssh "mkdir -p $(shell_quote "$RESTORE_DIR"); cp -p $(shell_quote "$NGINX_CONF") $(shell_quote "$backup_path")"
gateway_ssh bash -s -- "$NGINX_CONF" "$GATEWAY_PORT" "$backup_path" <<'REMOTE_NGINX'
set -euo pipefail
conf="$1"
port="$2"
backup="$3"
python3 - "$conf" "$port" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
port = sys.argv[2]
marker = "# BEGIN FANREN INFINITE CANVAS"
text = path.read_text()
if marker in text:
    raise SystemExit(0)

server_name = "server_name fanrenapi.com;"
server_start = text.find(server_name)
if server_start < 0:
    raise SystemExit("fanrenapi.com HTTPS server block not found")
location = text.find("    location / {", server_start)
if location < 0:
    raise SystemExit("fanrenapi.com HTTPS location not found")

block = f'''    {marker}
    location = /creative {{
        return 301 /creative/;
    }}
    location ^~ /creative/ {{
        proxy_pass http://127.0.0.1:{port}/;
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $fanrenapi_forwarded_proto;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Prefix /creative;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }}
'''
path.write_text(text[:location] + block + text[location:])
PY
if ! nginx -t; then
    cp -p "$backup" "$conf"
    nginx -t
    exit 1
fi
systemctl reload nginx
REMOTE_NGINX

echo "[8/8] 验证主站、CDN 与画布入口"
curl_bin="$(command -v curl || printf '/usr/bin/curl')"
"$curl_bin" -fsS --max-time 20 https://fanrenapi.com/api/status >/dev/null
"$curl_bin" -fsS --max-time 20 https://cdn.fanrenapi.com/api/status >/dev/null
"$curl_bin" -fsS --max-time 20 "${PUBLIC_URL}api/health" >/dev/null
"$curl_bin" -fsS --max-time 20 "${PUBLIC_URL}login" | grep -q "无限画布"

echo "部署完成: ${PUBLIC_URL}"
echo "镜像: ${IMAGE}"
echo "Netcup 容器: fanren-infinite-canvas (127.0.0.1:${TARGET_PORT})"
echo "DMIT 隧道: 127.0.0.1:${GATEWAY_PORT} -> Netcup:${TARGET_PORT}"
echo "Nginx 备份: ${backup_path}"

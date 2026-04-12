#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP_NAME="ibnu-go-backend"
BIN_DIR="$REPO_ROOT/bin"
BIN_PATH="$BIN_DIR/$APP_NAME"
RUNTIME_DIR="${CPANEL_RUNTIME_DIR:-$HOME/.local/$APP_NAME}"
ENV_FILE="${CPANEL_BACKEND_ENV_FILE:-$RUNTIME_DIR/.env.production}"
LOG_DIR="${CPANEL_BACKEND_LOG_DIR:-$HOME/logs/$APP_NAME}"
PID_FILE="$RUNTIME_DIR/$APP_NAME.pid"

mkdir -p "$BIN_DIR" "$RUNTIME_DIR" "$LOG_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "[deploy] File env tidak ditemukan: $ENV_FILE"
  echo "[deploy] Salin deploy/cpanel/.env.production.example ke $ENV_FILE lalu isi nilai sebenarnya."
  exit 1
fi

cd "$REPO_ROOT"

echo "[deploy] Sedang build binary backend..."
go mod download
go build -o "$BIN_PATH" ./cmd/server

echo "[deploy] Menghentikan proses sebelumnya (jika ada)..."
if [[ -f "$PID_FILE" ]]; then
  OLD_PID="$(cat "$PID_FILE" || true)"
  if [[ -n "${OLD_PID:-}" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
    kill "$OLD_PID" || true
    sleep 1
  fi
fi

echo "[deploy] Menjalankan backend..."
set -a
source "$ENV_FILE"
set +a

nohup "$BIN_PATH" >> "$LOG_DIR/app.log" 2>&1 &
NEW_PID=$!
printf "%s" "$NEW_PID" > "$PID_FILE"

echo "[deploy] Backend berjalan dengan PID $NEW_PID"
echo "[deploy] File log: $LOG_DIR/app.log"

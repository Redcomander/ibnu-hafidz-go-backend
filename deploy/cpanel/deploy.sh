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

find_go_bin() {
  if [[ -n "${CPANEL_GO_BIN:-}" ]] && [[ -x "${CPANEL_GO_BIN}" ]]; then
    echo "${CPANEL_GO_BIN}"
    return 0
  fi

  if command -v go >/dev/null 2>&1; then
    command -v go
    return 0
  fi

  local candidates=(
    $HOME/.local/go/bin/go
    /usr/local/go/bin/go
    /opt/go/bin/go
    /usr/bin/go
    /usr/local/bin/go
  )

  local path
  for path in "${candidates[@]}"; do
    if [[ -x "$path" ]]; then
      echo "$path"
      return 0
    fi
  done

  return 1
}

mkdir -p "$BIN_DIR" "$RUNTIME_DIR" "$LOG_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "[deploy] File env tidak ditemukan: $ENV_FILE"
  echo "[deploy] Salin deploy/cpanel/.env.production.example ke $ENV_FILE lalu isi nilai sebenarnya."
  exit 1
fi

cd "$REPO_ROOT"

GO_BIN="$(find_go_bin || true)"
if [[ -z "$GO_BIN" ]]; then
  echo "[deploy] go tidak ditemukan di server."
  echo "[deploy] Pasang Go atau tambahkan binary go ke PATH shell cPanel."
  exit 1
fi

export PATH="$(dirname "$GO_BIN"):$PATH"
echo "[deploy] Menggunakan go di: $GO_BIN"

GO_VERSION="$($GO_BIN version | awk '{print $3}')"
echo "[deploy] Versi Go: $GO_VERSION"
if [[ "$GO_VERSION" =~ ^go1\.([0-9]+) ]]; then
  GO_MINOR="${BASH_REMATCH[1]}"
  if (( GO_MINOR < 22 )); then
    echo "[deploy] Versi Go terlalu lama. Minimal go1.22 diperlukan oleh dependency proyek."
    echo "[deploy] Solusi cepat: pasang Go terbaru ke \$HOME/.local/go lalu set CPANEL_GO_BIN=\$HOME/.local/go/bin/go"
    exit 1
  fi
fi

echo "[deploy] Sedang build binary backend..."
"$GO_BIN" mod download
"$GO_BIN" build -o "$BIN_PATH" ./cmd/server

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

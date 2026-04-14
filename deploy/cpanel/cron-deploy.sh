#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BRANCH="${CPANEL_DEPLOY_BRANCH:-main}"
LOG_DIR="${CPANEL_BACKEND_LOG_DIR:-$HOME/logs/ibnu-go-backend}"
LOCK_FILE="${CPANEL_CRON_LOCK_FILE:-$HOME/.local/ibnu-go-backend/cron-deploy.lock}"
MAX_LOCK_AGE="${CPANEL_CRON_MAX_LOCK_AGE_SECONDS:-900}"

mkdir -p "$LOG_DIR" "$(dirname "$LOCK_FILE")"

log() {
  printf '[cron-deploy] %s\n' "$*"
}

now_epoch() {
  date +%s
}

acquire_lock() {
  if [[ -f "$LOCK_FILE" ]]; then
    local lock_pid lock_time age
    lock_pid="$(awk -F: '{print $1}' "$LOCK_FILE" 2>/dev/null || true)"
    lock_time="$(awk -F: '{print $2}' "$LOCK_FILE" 2>/dev/null || true)"

    if [[ -n "${lock_pid:-}" ]] && kill -0 "$lock_pid" 2>/dev/null; then
      log "Deploy lain sedang berjalan (PID $lock_pid), skip."
      exit 0
    fi

    if [[ -n "${lock_time:-}" ]]; then
      age="$(( $(now_epoch) - lock_time ))"
      if (( age < MAX_LOCK_AGE )); then
        log "Lock baru ($age detik), skip."
        exit 0
      fi
    fi
  fi

  printf '%s:%s\n' "$$" "$(now_epoch)" > "$LOCK_FILE"
}

release_lock() {
  rm -f "$LOCK_FILE"
}

trap release_lock EXIT
acquire_lock

cd "$REPO_ROOT"

if ! command -v git >/dev/null 2>&1; then
  log "git tidak ditemukan di PATH."
  exit 1
fi

log "Fetch origin/$BRANCH"
git fetch --prune origin "$BRANCH"

LOCAL_SHA="$(git rev-parse HEAD)"
REMOTE_SHA="$(git rev-parse "origin/$BRANCH")"

if [[ "$LOCAL_SHA" == "$REMOTE_SHA" ]]; then
  log "Tidak ada perubahan."
  exit 0
fi

log "Perubahan terdeteksi: $LOCAL_SHA -> $REMOTE_SHA"
log "Reset ke origin/$BRANCH"
git reset --hard "origin/$BRANCH"

log "Menjalankan deploy/cpanel/deploy.sh"
/bin/bash "$REPO_ROOT/deploy/cpanel/deploy.sh" >> "$LOG_DIR/cron-deploy.log" 2>&1

log "Selesai deploy ke commit $REMOTE_SHA"

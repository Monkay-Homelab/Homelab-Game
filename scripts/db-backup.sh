#!/usr/bin/env bash
# db-backup.sh -- Automated PostgreSQL backup for Homelab the Game
#
# Produces both a custom-format dump (for pg_restore) and a plain SQL dump
# (for human-readable inspection). Supports daily and weekly retention.
#
# This script is designed to run as:
#   - A systemd timer (see scripts/systemd/db-backup.timer)
#   - A cron job: 0 3 * * * /root/project/scripts/db-backup.sh
#   - Manually: ./scripts/db-backup.sh
#
# Environment variables (all have sensible defaults for this single-VM setup):
#   BACKUP_DIR       -- Where backups are stored (default: /var/backups/homelab-game)
#   DB_NAME          -- Database name (default: homelab_game)
#   DB_USER          -- Database user (default: homelab_game)
#   DB_HOST          -- Database host (default: /var/run/postgresql, i.e. local socket)
#   DB_PORT          -- Database port (default: 5432)
#   DAILY_RETENTION  -- Number of daily backups to keep (default: 7)
#   WEEKLY_RETENTION -- Number of weekly backups to keep (default: 4)
#   PGPASSWORD       -- PostgreSQL password (not needed for local socket auth)
#   PG_OS_USER       -- OS user to run pg_dump as (default: postgres)
#                       Local socket auth (peer) requires pg_dump to run as the
#                       postgres OS user. Set to empty string to skip sudo.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

BACKUP_DIR="${BACKUP_DIR:-/var/backups/homelab-game}"
DB_NAME="${DB_NAME:-homelab_game}"
DB_USER="${DB_USER:-homelab_game}"
DB_HOST="${DB_HOST:-/var/run/postgresql}"
DB_PORT="${DB_PORT:-5432}"
DAILY_RETENTION="${DAILY_RETENTION:-7}"
WEEKLY_RETENTION="${WEEKLY_RETENTION:-4}"
PG_OS_USER="${PG_OS_USER:-postgres}"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DAY_OF_WEEK=$(date +%u)  # 1=Monday, 7=Sunday

DAILY_DIR="${BACKUP_DIR}/daily"
WEEKLY_DIR="${BACKUP_DIR}/weekly"

# ---------------------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------------------

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [backup] $*"
}

log_error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [backup] ERROR: $*" >&2
}

# Run a command as the PG_OS_USER (for peer auth on local socket).
# If PG_OS_USER is empty or matches the current user, run directly.
run_as_pg() {
    if [ -n "$PG_OS_USER" ] && [ "$(id -un)" != "$PG_OS_USER" ]; then
        sudo -u "$PG_OS_USER" "$@"
    else
        "$@"
    fi
}

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------

# Verify pg_dump is available
if ! command -v pg_dump &>/dev/null; then
    log_error "pg_dump not found in PATH"
    exit 1
fi

# Verify pg_isready (database is accepting connections)
if ! pg_isready -h "$DB_HOST" -p "$DB_PORT" -d "$DB_NAME" -q 2>/dev/null; then
    log_error "Database is not ready (host=$DB_HOST port=$DB_PORT db=$DB_NAME)"
    exit 1
fi

# Create backup directories if they do not exist
mkdir -p "$DAILY_DIR" "$WEEKLY_DIR"

# ---------------------------------------------------------------------------
# Backup
# ---------------------------------------------------------------------------

DUMP_FILE="${DAILY_DIR}/${DB_NAME}_${TIMESTAMP}.dump"
SQL_FILE="${DAILY_DIR}/${DB_NAME}_${TIMESTAMP}.sql"

log "Starting backup of database '${DB_NAME}' to ${DAILY_DIR}/"

# Custom-format dump (compressed, supports pg_restore with selective restore)
# --no-owner: Omit ownership commands (restore as any user)
# --no-privileges: Omit GRANT/REVOKE (avoid privilege conflicts on restore)
# --format=custom: Binary format, compressed, supports parallel restore
#
# We pipe stdout to the output file (instead of -f) so that pg_dump runs as
# the postgres OS user for peer auth, while the file is written by the
# calling user (root) who owns the backup directory.
if run_as_pg pg_dump \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -d "$DB_NAME" \
    --no-owner \
    --no-privileges \
    --format=custom \
    > "$DUMP_FILE" 2>/dev/null; then
    log "Custom-format dump complete: ${DUMP_FILE} ($(du -h "$DUMP_FILE" | cut -f1))"
else
    log_error "Custom-format dump failed"
    rm -f "$DUMP_FILE"
    exit 1
fi

# Plain SQL dump (human-readable, useful for inspection and simple restores)
if run_as_pg pg_dump \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -d "$DB_NAME" \
    --no-owner \
    --no-privileges \
    --format=plain \
    > "$SQL_FILE" 2>/dev/null; then
    log "Plain SQL dump complete: ${SQL_FILE} ($(du -h "$SQL_FILE" | cut -f1))"
else
    log_error "Plain SQL dump failed"
    rm -f "$SQL_FILE"
    exit 1
fi

# ---------------------------------------------------------------------------
# Weekly promotion: copy Sunday's backup to weekly directory
# ---------------------------------------------------------------------------

if [ "$DAY_OF_WEEK" = "7" ]; then
    WEEKLY_DUMP="${WEEKLY_DIR}/${DB_NAME}_weekly_${TIMESTAMP}.dump"
    WEEKLY_SQL="${WEEKLY_DIR}/${DB_NAME}_weekly_${TIMESTAMP}.sql"
    cp "$DUMP_FILE" "$WEEKLY_DUMP"
    cp "$SQL_FILE" "$WEEKLY_SQL"
    log "Sunday backup promoted to weekly: ${WEEKLY_DIR}/"
fi

# ---------------------------------------------------------------------------
# Retention: prune old backups
# ---------------------------------------------------------------------------

prune_old_backups() {
    local dir="$1"
    local keep="$2"
    local label="$3"

    # Count .dump files (each backup produces a .dump and .sql pair)
    local count
    count=$(find "$dir" -maxdepth 1 -name "*.dump" -type f | wc -l)

    if [ "$count" -gt "$keep" ]; then
        local to_remove=$((count - keep))
        log "Pruning ${to_remove} old ${label} backup(s) (keeping ${keep})"

        # Remove oldest .dump files and their matching .sql files
        find "$dir" -maxdepth 1 -name "*.dump" -type f -printf '%T@ %p\n' \
            | sort -n \
            | head -n "$to_remove" \
            | while read -r _ filepath; do
                local base
                base=$(basename "$filepath" .dump)
                log "  Removing: ${base}.dump + ${base}.sql"
                rm -f "$filepath"
                rm -f "${dir}/${base}.sql"
            done
    fi
}

prune_old_backups "$DAILY_DIR" "$DAILY_RETENTION" "daily"
prune_old_backups "$WEEKLY_DIR" "$WEEKLY_RETENTION" "weekly"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

DAILY_COUNT=$(find "$DAILY_DIR" -maxdepth 1 -name "*.dump" -type f | wc -l)
WEEKLY_COUNT=$(find "$WEEKLY_DIR" -maxdepth 1 -name "*.dump" -type f | wc -l)
TOTAL_SIZE=$(du -sh "$BACKUP_DIR" 2>/dev/null | cut -f1)

log "Backup complete. Daily: ${DAILY_COUNT}/${DAILY_RETENTION}, Weekly: ${WEEKLY_COUNT}/${WEEKLY_RETENTION}, Total size: ${TOTAL_SIZE}"

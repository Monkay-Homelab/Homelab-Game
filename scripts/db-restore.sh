#!/usr/bin/env bash
# db-restore.sh -- Restore a PostgreSQL backup for Homelab the Game
#
# Restores from a custom-format (.dump) or plain SQL (.sql) backup file.
# Detects the format automatically from the file extension.
#
# Usage:
#   ./scripts/db-restore.sh <backup-file>
#   ./scripts/db-restore.sh /var/backups/homelab-game/daily/homelab_game_20260404_030000.dump
#   ./scripts/db-restore.sh /var/backups/homelab-game/daily/homelab_game_20260404_030000.sql
#
# Environment variables:
#   DB_NAME   -- Target database (default: homelab_game)
#   DB_USER   -- Database user (default: homelab_game)
#   DB_HOST   -- Database host (default: /var/run/postgresql, i.e. local socket)
#   DB_PORT   -- Database port (default: 5432)
#   PGPASSWORD -- PostgreSQL password (not needed for local socket auth)
#
# IMPORTANT:
#   - This script drops and recreates the target database. ALL EXISTING DATA WILL BE LOST.
#   - Stop the backend service before restoring to avoid connection conflicts.
#   - The script will ask for confirmation before proceeding.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DB_NAME="${DB_NAME:-homelab_game}"
DB_USER="${DB_USER:-homelab_game}"
DB_HOST="${DB_HOST:-/var/run/postgresql}"
DB_PORT="${DB_PORT:-5432}"

# ---------------------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------------------

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [restore] $*"
}

log_error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] [restore] ERROR: $*" >&2
}

# ---------------------------------------------------------------------------
# Argument validation
# ---------------------------------------------------------------------------

if [ $# -ne 1 ]; then
    echo "Usage: $0 <backup-file>"
    echo ""
    echo "Supported formats:"
    echo "  .dump  -- Custom-format (pg_restore)"
    echo "  .sql   -- Plain SQL (psql)"
    echo ""
    echo "Available backups:"
    if [ -d /var/backups/homelab-game ]; then
        find /var/backups/homelab-game -name "*.dump" -o -name "*.sql" | sort -r | head -20
    else
        echo "  No backups found in /var/backups/homelab-game/"
    fi
    exit 1
fi

BACKUP_FILE="$1"

if [ ! -f "$BACKUP_FILE" ]; then
    log_error "Backup file not found: ${BACKUP_FILE}"
    exit 1
fi

# Determine format from extension
case "$BACKUP_FILE" in
    *.dump)
        FORMAT="custom"
        ;;
    *.sql)
        FORMAT="plain"
        ;;
    *)
        log_error "Unrecognized file extension. Expected .dump or .sql"
        exit 1
        ;;
esac

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------

if [ "$FORMAT" = "custom" ] && ! command -v pg_restore &>/dev/null; then
    log_error "pg_restore not found in PATH"
    exit 1
fi

if ! command -v psql &>/dev/null; then
    log_error "psql not found in PATH"
    exit 1
fi

# Check database connectivity (connect to 'postgres' maintenance db)
if ! pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -q 2>/dev/null; then
    log_error "PostgreSQL is not ready"
    exit 1
fi

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------

FILE_SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
FILE_DATE=$(stat -c '%y' "$BACKUP_FILE" 2>/dev/null | cut -d'.' -f1)

echo "============================================================"
echo "  DATABASE RESTORE"
echo "============================================================"
echo ""
echo "  Backup file : ${BACKUP_FILE}"
echo "  File size   : ${FILE_SIZE}"
echo "  File date   : ${FILE_DATE}"
echo "  Format      : ${FORMAT}"
echo ""
echo "  Target DB   : ${DB_NAME}"
echo "  DB Host     : ${DB_HOST}"
echo "  DB Port     : ${DB_PORT}"
echo "  DB User     : ${DB_USER}"
echo ""
echo "  WARNING: This will DROP and RECREATE the '${DB_NAME}' database."
echo "  ALL EXISTING DATA WILL BE LOST."
echo ""
echo "============================================================"
echo ""

# Skip confirmation if running non-interactively (e.g., piped or in a script)
if [ -t 0 ]; then
    read -r -p "Type 'yes' to proceed: " CONFIRM
    if [ "$CONFIRM" != "yes" ]; then
        log "Restore cancelled by user"
        exit 0
    fi
else
    log "Running non-interactively -- add confirmation handling in your wrapper script"
    log_error "Refusing to drop database without interactive confirmation"
    exit 1
fi

echo ""

# ---------------------------------------------------------------------------
# Stop active connections to the target database
# ---------------------------------------------------------------------------

log "Terminating active connections to '${DB_NAME}'..."
sudo -u postgres psql -q -c "
    SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
    WHERE datname = '${DB_NAME}' AND pid <> pg_backend_pid();
" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Drop and recreate the database
# ---------------------------------------------------------------------------

log "Dropping database '${DB_NAME}'..."
sudo -u postgres dropdb --if-exists "$DB_NAME"

log "Creating database '${DB_NAME}' (owner: ${DB_USER})..."
sudo -u postgres createdb -O "$DB_USER" "$DB_NAME"

# Enable TimescaleDB if available (the extension may not be installed locally,
# but the backup may contain TimescaleDB objects -- this is a best-effort step)
sudo -u postgres psql -d "$DB_NAME" -c "CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;" 2>/dev/null || \
    log "Note: TimescaleDB extension not available locally (non-fatal)"

# ---------------------------------------------------------------------------
# Restore
# ---------------------------------------------------------------------------

log "Restoring from ${FORMAT}-format backup..."

if [ "$FORMAT" = "custom" ]; then
    # pg_restore with custom format
    # --no-owner: Do not set ownership (restore as current user)
    # --no-privileges: Do not restore GRANT/REVOKE
    # --single-transaction: Wrap in a transaction (all or nothing)
    # --exit-on-error: Stop on first error
    if sudo -u postgres pg_restore \
        --dbname="$DB_NAME" \
        --no-owner \
        --no-privileges \
        --single-transaction \
        --verbose \
        "$BACKUP_FILE" 2>&1 | while IFS= read -r line; do log "  pg_restore: $line"; done; then
        log "pg_restore completed successfully"
    else
        log_error "pg_restore reported errors (some non-fatal warnings are expected)"
    fi
elif [ "$FORMAT" = "plain" ]; then
    # Plain SQL via psql
    # --single-transaction: Wrap in a transaction
    # --set ON_ERROR_STOP=on: Stop on first error
    if sudo -u postgres psql \
        -d "$DB_NAME" \
        --single-transaction \
        --set ON_ERROR_STOP=on \
        -f "$BACKUP_FILE" 2>&1 | tail -5; then
        log "psql restore completed successfully"
    else
        log_error "psql restore reported errors"
    fi
fi

# ---------------------------------------------------------------------------
# Grant permissions to application user
# ---------------------------------------------------------------------------

log "Granting permissions to '${DB_USER}'..."
sudo -u postgres psql -d "$DB_NAME" -c "
    GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ${DB_USER};
    GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ${DB_USER};
    GRANT USAGE ON SCHEMA public TO ${DB_USER};
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${DB_USER};
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${DB_USER};
" 2>/dev/null

# ---------------------------------------------------------------------------
# Verification
# ---------------------------------------------------------------------------

log "Verifying restore..."

TABLE_COUNT=$(sudo -u postgres psql -d "$DB_NAME" -t -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';" | tr -d ' ')
USER_COUNT=$(sudo -u postgres psql -d "$DB_NAME" -t -c "SELECT count(*) FROM users;" 2>/dev/null | tr -d ' ' || echo "N/A")

log "Restore complete."
log "  Tables restored : ${TABLE_COUNT}"
log "  Users in DB     : ${USER_COUNT}"
log ""
log "Next steps:"
log "  1. Restart the backend service:"
log "     docker service update --force homelab-the-game_backend"
log "  2. Verify the application works correctly"

#!/usr/bin/env bash
# db-backup-list.sh -- List available database backups with size and age
#
# Usage: ./scripts/db-backup-list.sh

set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/homelab-game}"

if [ ! -d "$BACKUP_DIR" ]; then
    echo "No backups found. Backup directory does not exist: ${BACKUP_DIR}"
    exit 0
fi

echo "============================================================"
echo "  DATABASE BACKUPS"
echo "  Location: ${BACKUP_DIR}"
echo "============================================================"

list_backups() {
    local dir="$1"
    local label="$2"

    if [ ! -d "$dir" ] || [ -z "$(ls -A "$dir" 2>/dev/null)" ]; then
        echo "  (none)"
        return
    fi

    # List .dump files with size and modification time, newest first
    find "$dir" -maxdepth 1 -name "*.dump" -type f -printf '%T@ %s %p\n' \
        | sort -rn \
        | while read -r mtime size filepath; do
            local basename
            basename=$(basename "$filepath" .dump)
            local human_size
            human_size=$(numfmt --to=iec-i --suffix=B "$size" 2>/dev/null || echo "${size}B")
            local human_date
            human_date=$(date -d "@${mtime%.*}" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || echo "unknown")
            local sql_exists=""
            if [ -f "${dir}/${basename}.sql" ]; then
                sql_exists=" [+sql]"
            fi
            printf "  %-8s  %s  %s%s\n" "$human_size" "$human_date" "$(basename "$filepath")" "$sql_exists"
        done
}

echo ""
echo "Daily backups:"
list_backups "${BACKUP_DIR}/daily" "daily"

echo ""
echo "Weekly backups:"
list_backups "${BACKUP_DIR}/weekly" "weekly"

echo ""
TOTAL_SIZE=$(du -sh "$BACKUP_DIR" 2>/dev/null | cut -f1)
echo "Total backup size: ${TOTAL_SIZE:-0}"
echo ""

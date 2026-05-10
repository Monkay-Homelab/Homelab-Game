# Database Backup & Restore

Automated PostgreSQL backup scripts for Homelab the Game.

## Overview

| What | Details |
|---|---|
| Database | PostgreSQL 16 (`homelab_game`) |
| Backup method | `pg_dump` (custom-format + plain SQL) |
| Schedule | Daily at 03:00 (systemd timer or cron) |
| Retention | 7 daily + 4 weekly (Sunday) |
| Backup location | `/var/backups/homelab-game/` |
| Restore method | `pg_restore` (custom-format) or `psql` (plain SQL) |

## Scripts

| Script | Purpose |
|---|---|
| `scripts/db-backup.sh` | Run a backup (called by timer/cron) |
| `scripts/db-restore.sh` | Restore from a backup file |
| `scripts/db-backup-list.sh` | List available backups with size/age |

## Backup Format

Each backup produces two files:

- **`.dump`** -- Custom-format (binary, compressed). Use with `pg_restore`. Supports
  selective table restore and parallel restore jobs.
- **`.sql`** -- Plain SQL (human-readable). Use with `psql`. Useful for inspection,
  grepping data, or restoring on systems without `pg_restore`.

## Retention Policy

- **Daily**: Keep the last 7 backups. Older dailies are pruned automatically.
- **Weekly**: Every Sunday backup is copied to the weekly directory. Keep the last 4 weeklies.
- At the default database size (~9 MB), this uses approximately 100-200 MB of disk.

## Setup

### Option A: systemd timer (recommended)

```bash
# Copy unit files
sudo cp scripts/systemd/homelab-db-backup.service /etc/systemd/system/
sudo cp scripts/systemd/homelab-db-backup.timer /etc/systemd/system/

# Make backup script executable
chmod +x scripts/db-backup.sh scripts/db-restore.sh scripts/db-backup-list.sh

# Create backup directory
sudo mkdir -p /var/backups/homelab-game

# Enable and start the timer
sudo systemctl daemon-reload
sudo systemctl enable homelab-db-backup.timer
sudo systemctl start homelab-db-backup.timer

# Verify the timer is active
sudo systemctl list-timers homelab-db-backup.timer
```

### Option B: cron

```bash
# Make scripts executable
chmod +x scripts/db-backup.sh scripts/db-restore.sh scripts/db-backup-list.sh

# Create backup directory
sudo mkdir -p /var/backups/homelab-game

# Add cron entry (daily at 03:00)
(crontab -l 2>/dev/null; echo "0 3 * * * /root/project/scripts/db-backup.sh >> /var/log/homelab-db-backup.log 2>&1") | crontab -
```

### Manual test run

```bash
# Run a backup now to verify everything works
sudo ./scripts/db-backup.sh

# List backups
./scripts/db-backup-list.sh
```

## Restoring a Backup

**IMPORTANT**: Restoring drops and recreates the database. All existing data is lost.

### 1. Stop the backend

```bash
# If running in Docker Swarm:
docker service scale homelab-the-game_backend=0

# If running directly:
lsof -ti:8080 | xargs kill -9
```

### 2. List available backups

```bash
./scripts/db-backup-list.sh
```

### 3. Restore

```bash
# From custom-format dump (recommended -- faster, supports selective restore):
sudo ./scripts/db-restore.sh /var/backups/homelab-game/daily/homelab_game_20260404_030000.dump

# From plain SQL:
sudo ./scripts/db-restore.sh /var/backups/homelab-game/daily/homelab_game_20260404_030000.sql
```

### 4. Restart the backend

```bash
# If running in Docker Swarm:
docker service scale homelab-the-game_backend=2

# If running directly:
cd /root/project/apps/backend && go run ./cmd/server/ &
```

### 5. Verify

- Check the game loads and player data is present
- Check logs for database connection errors

## Selective Table Restore (custom-format only)

To restore a single table without dropping the whole database:

```bash
# List contents of a dump file:
pg_restore --list /var/backups/homelab-game/daily/homelab_game_20260404_030000.dump

# Restore only the 'users' table (data only, into existing database):
pg_restore \
  --dbname=homelab_game \
  --data-only \
  --table=users \
  --no-owner \
  --no-privileges \
  /var/backups/homelab-game/daily/homelab_game_20260404_030000.dump
```

## Monitoring

### Check timer status

```bash
# systemd timer:
systemctl status homelab-db-backup.timer
systemctl list-timers homelab-db-backup.timer

# Last run result:
journalctl -u homelab-db-backup.service --since today
```

### Check cron logs

```bash
# If using cron:
tail -50 /var/log/homelab-db-backup.log
```

### Verify backups exist

```bash
./scripts/db-backup-list.sh
```

## Configuration

All scripts accept environment variables for customization:

| Variable | Default | Description |
|---|---|---|
| `BACKUP_DIR` | `/var/backups/homelab-game` | Root directory for backups |
| `DB_NAME` | `homelab_game` | Database name |
| `DB_USER` | `homelab_game` | Database user |
| `DB_HOST` | `/var/run/postgresql` | Database host (local socket) |
| `DB_PORT` | `5432` | Database port |
| `DAILY_RETENTION` | `7` | Number of daily backups to keep |
| `WEEKLY_RETENTION` | `4` | Number of weekly backups to keep |
| `PGPASSWORD` | (unset) | Password (not needed for local socket auth) |
| `PG_OS_USER` | `postgres` | OS user for pg_dump (peer auth). Set empty to skip sudo |

## Disaster Recovery Scenarios

| Scenario | Procedure |
|---|---|
| Bad migration corrupted data | Restore from most recent backup before the migration |
| Accidental data deletion | Restore from most recent backup |
| VM disk failure | Backups are on the same disk -- for off-site recovery, copy `/var/backups/homelab-game/` to external storage periodically |
| Need to inspect old data | Use the `.sql` dump and grep for the data you need |

### Known limitation: single-disk backups

Both the database and backups reside on the same VM disk. If the disk fails, both are lost.
For true disaster recovery, copy backups off-machine periodically:

```bash
# Example: rsync to a NAS or remote server
rsync -avz /var/backups/homelab-game/ user@nas:/backups/homelab-game/

# Example: upload to S3-compatible storage
aws s3 sync /var/backups/homelab-game/ s3://my-bucket/homelab-game-backups/
```

This is documented as a known limitation, not automated, because the project runs on a single
homelab VM and adding off-site backup infrastructure is a separate decision.

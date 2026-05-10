#!/bin/sh
set -e

# Read DB_PASSWORD from Docker secret file if env var not set
if [ -z "$DB_PASSWORD" ] && [ -f /run/secrets/db_password ]; then
  DB_PASSWORD=$(cat /run/secrets/db_password)
fi

export PGHOST="$DB_HOST"
export PGPORT="$DB_PORT"
export PGDATABASE="$DB_NAME"
export PGUSER="$DB_USER"
export PGPASSWORD="$DB_PASSWORD"

echo "Waiting for PostgreSQL to be ready..."
until pg_isready -q; do
  sleep 1
done

echo "Ensuring TimescaleDB extension..."
psql -c "CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;" 2>/dev/null || true

echo "Creating schema_migrations tracking table..."
psql -c "
  CREATE TABLE IF NOT EXISTS schema_migrations (
    version INT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
"

# Run each numbered migration if not already applied
for f in /migrations/[0-9]*.sql; do
  version=$(basename "$f" | grep -o '^[0-9]*')
  version=$((10#$version))  # strip leading zeros

  already=$(psql -tAc "SELECT 1 FROM schema_migrations WHERE version = $version;" 2>/dev/null || echo "")
  if [ "$already" = "1" ]; then
    echo "Skipping migration $version (already applied)"
    continue
  fi

  echo "Applying migration $version: $(basename "$f")"
  psql -f "$f"
  psql -c "INSERT INTO schema_migrations (version) VALUES ($version);"
done

echo "All migrations applied."

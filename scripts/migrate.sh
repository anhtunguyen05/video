#!/bin/sh
set -eu

direction="${1:-}"
case "$direction" in
  up) migration_file="migrations/000001_create_schema_migrations.up.sql" ;;
  down) migration_file="migrations/000001_create_schema_migrations.down.sql" ;;
  *) echo "usage: $0 {up|down}" >&2; exit 2 ;;
esac

docker compose -f "${COMPOSE_FILE:-compose.yaml}" \
  exec -T postgres \
  sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -' < "$migration_file"

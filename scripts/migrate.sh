#!/bin/sh
set -eu

direction="${1:-}"
case "$direction" in
  up)
    for migration_file in migrations/*.up.sql; do
      docker compose -f "${COMPOSE_FILE:-compose.yaml}" \
        exec -T postgres \
        sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -' < "$migration_file"
    done
    exit 0
    ;;
  down)
    migration_files=$(find migrations -maxdepth 1 -type f -name '*.down.sql' | sort -r)
    for migration_file in $migration_files; do
      docker compose -f "${COMPOSE_FILE:-compose.yaml}" \
        exec -T postgres \
        sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -' < "$migration_file"
    done
    exit 0
    ;;
  *) echo "usage: $0 {up|down}" >&2; exit 2 ;;
esac

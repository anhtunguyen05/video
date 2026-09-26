#!/bin/sh
set -eu

migration_is_applied() {
  version_number="$1"
  query="SELECT 1 FROM schema_migrations WHERE version = ${version_number};"
  printf '%s\n' "$query" |
    docker compose -f "${COMPOSE_FILE:-compose.yaml}" \
      exec -T postgres sh -c 'psql -At -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -' |
    grep -q '^1$'
}

case "${1:-}" in
  up)
    for migration_file in migrations/*.up.sql; do
      version="$(basename "$migration_file" | cut -d_ -f1)"
      version_number="$(printf '%s' "$version" | sed 's/^0*//')"
      [ -n "$version_number" ] || version_number=0
      if [ "$version" != "000001" ] && migration_is_applied "$version_number"; then
        echo "Skipping migration $version (up): already applied"
        continue
      fi
      echo "Applying migration $version (up): $(basename "$migration_file")"
      docker compose -f "${COMPOSE_FILE:-compose.yaml}" \
        exec -T postgres \
        sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -' < "$migration_file"
    done
    exit 0
    ;;
  down)
    migration_files=$(find migrations -maxdepth 1 -type f -name '*.down.sql' | sort -r)
    for migration_file in $migration_files; do
      version="$(basename "$migration_file" | cut -d_ -f1)"
      version_number="$(printf '%s' "$version" | sed 's/^0*//')"
      [ -n "$version_number" ] || version_number=0
      if ! migration_is_applied "$version_number"; then
        echo "Skipping migration $version (down): not applied"
        continue
      fi
      echo "Applying migration $version (down): $(basename "$migration_file")"
      docker compose -f "${COMPOSE_FILE:-compose.yaml}" \
        exec -T postgres \
        sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -' < "$migration_file"
    done
    exit 0
    ;;
  *) echo "usage: $0 {up|down}" >&2; exit 2 ;;
esac

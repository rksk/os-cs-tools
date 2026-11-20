#!/usr/bin/env bash
# Builds the schema from an EMPTY PostgreSQL database with `make migrate`, then
# runs it again and requires the second run to apply nothing.
#
# Usage: scripts/check_migrations_from_empty.sh   (needs docker and psql)
# Env:   PG_IMAGE (default postgres:18), PG_PORT (default 55499)
#
# The Makefile target runs from a temporary directory holding only the Makefile
# and a link to migrations/, so a local .env (which `make migrate` sources and
# which would override DATABASE_URL) can never point this check at a real DB.
set -euo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
image="${PG_IMAGE:-postgres:18}"; port="${PG_PORT:-55499}"
name="migcheck-$$"; work="$(mktemp -d)"
trap 'docker rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

docker run -d --name "$name" -e POSTGRES_PASSWORD=check -p "$port:5432" "$image" >/dev/null
for _ in $(seq 1 60); do pg_isready -q -h localhost -p "$port" && break; sleep 1; done
sleep 2 # the image restarts the server once after initdb
for _ in $(seq 1 60); do pg_isready -q -h localhost -p "$port" && break; sleep 1; done

cp "$here/Makefile" "$work/" && ln -s "$here/migrations" "$work/migrations"
export DATABASE_URL="postgresql://postgres:check@localhost:$port/postgres"

make -s -C "$work" migrate > "$work/run1.log" 2>&1 || { tail -20 "$work/run1.log"; echo "FAIL: first run"; exit 1; }
make -s -C "$work" migrate > "$work/run2.log" 2>&1 || { tail -20 "$work/run2.log"; echo "FAIL: second run"; exit 1; }
if grep -q '^applying' "$work/run2.log"; then grep '^applying' "$work/run2.log"; echo "FAIL: second run was not a no-op"; exit 1; fi

echo "applied: $(grep -c '^applying' "$work/run1.log"), second run skipped: $(grep -c 'already applied' "$work/run2.log")"
psql "$DATABASE_URL" -tAc "SELECT 'tracked=' || (SELECT count(*) FROM csm_migration_applied_migration)
  || ' tables=' || (SELECT count(*) FROM pg_tables WHERE schemaname = 'public')
  || ' policies=' || (SELECT count(*) FROM pg_policies)"
echo "OK"

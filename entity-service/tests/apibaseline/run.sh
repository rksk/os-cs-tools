#!/usr/bin/env bash
# Black-box API baseline driver for the entity-service.
#
#   run.sh run --tree <path-to-repo-or-entity-service> --out <results.json> [--keep]
#   run.sh compare <a.json> <b.json>
#
# `run` builds the server binary from the given source tree, starts a throwaway
# PostgreSQL cluster, applies THAT tree's migrations to it, seeds fictional
# data, starts the server on random ports, waits for readiness, runs the HTTP
# scenarios in this directory, writes normalized results JSON, and tears
# everything down. It never touches a shared or live database.
#
# Requirements: bash, go, and PostgreSQL 18 client/server binaries
# (PG_BIN, default /opt/homebrew/opt/postgresql@18/bin; falls back to PATH).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PG_BIN="${PG_BIN:-/opt/homebrew/opt/postgresql@18/bin}"
[ -x "$PG_BIN/initdb" ] || PG_BIN="$(dirname "$(command -v initdb)")"

cmd="${1:-}"; shift || true

build_tool() {
  (cd "$HERE" && go build -o "$1" .)
}

if [ "$cmd" = "compare" ]; then
  T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
  build_tool "$T/apibaseline"
  exec "$T/apibaseline" compare "$@"
fi

[ "$cmd" = "run" ] || { echo "usage: run.sh run --tree <path> --out <file> [--keep] | run.sh compare <a> <b>" >&2; exit 2; }

TREE=""; OUT=""; KEEP=0; HOLD=0
while [ $# -gt 0 ]; do
  case "$1" in
    --tree) TREE="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --keep) KEEP=1; shift ;;
    --hold) HOLD=1; shift ;;
    *) echo "unknown arg $1" >&2; exit 2 ;;
  esac
done
[ -n "$TREE" ] && [ -n "$OUT" ] || { echo "--tree and --out are required" >&2; exit 2; }
if [ -d "$TREE/entity-service/migrations" ]; then TREE="$TREE/entity-service"; fi
[ -d "$TREE/migrations" ] && [ -f "$TREE/cmd/api/main.go" ] || { echo "$TREE is not an entity-service source tree" >&2; exit 2; }
TREE="$(cd "$TREE" && pwd)"
case "$OUT" in /*) ;; *) OUT="$PWD/$OUT" ;; esac

W="$(mktemp -d)"
PIDS=()
cleanup() {
  for p in "${PIDS[@]:-}"; do if [ -n "$p" ]; then kill "$p" 2>/dev/null || true; wait "$p" 2>/dev/null || true; fi; done
  "$PG_BIN/pg_ctl" -D "$W/pg" -m immediate stop >/dev/null 2>&1 || true
  if [ "$KEEP" = 1 ]; then echo "kept work dir: $W" >&2; else rm -rf "$W"; fi
}
trap cleanup EXIT

free_port() {
  local p
  while :; do
    p=$((20000 + RANDOM % 30000))
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then echo "$p"; return; fi
  done
}

PGPORT_="$(free_port)"; SRV_PORT="$(free_port)"; HEALTH_PORT="$(free_port)"; JWKS_PORT="$(free_port)"
PGDB=apibaseline
SUPER="postgres://postgres@127.0.0.1:$PGPORT_/$PGDB?sslmode=disable"
APPROLE=ab_app; APPPW=ab_app_pw

echo "[apibaseline] tree: $TREE" >&2
echo "[apibaseline] commit: $(git -C "$TREE" rev-parse --short HEAD 2>/dev/null || echo unknown)" >&2

# 1. Ephemeral PostgreSQL.
"$PG_BIN/initdb" -D "$W/pg" -U postgres --auth=trust -E UTF8 >/dev/null
"$PG_BIN/pg_ctl" -D "$W/pg" -l "$W/pg.log" -w \
  -o "-p $PGPORT_ -k $W -c listen_addresses=127.0.0.1 -c fsync=off -c synchronous_commit=off -c full_page_writes=off" start >/dev/null
"$PG_BIN/createdb" -h 127.0.0.1 -p "$PGPORT_" -U postgres "$PGDB"
echo "[apibaseline] postgres: $("$PG_BIN/psql" "$SUPER" -tAc 'show server_version')" >&2

# 2. Migrations of THIS tree, in the canonical order (4-digit ascending, then
# 1NNNNN RLS track, then legacy 0NNNNN *.up.sql), mirroring the Makefile.
PSQL=("$PG_BIN/psql" "$SUPER" -v ON_ERROR_STOP=1 -q)
"${PSQL[@]}" -c "CREATE TABLE IF NOT EXISTS csm_migration_applied_migration (filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());"
n=0
files=$( { cd "$TREE/migrations"
  ls [0-9][0-9][0-9][0-9]_*.sql 2>/dev/null | sort
  ls 1[0-9][0-9][0-9][0-9][0-9]_*.sql 2>/dev/null | sort
  ls 0[0-9][0-9][0-9][0-9][0-9]_*.up.sql 2>/dev/null | sort; } )
apply_one() {
  "${PSQL[@]}" -o /dev/null -f "$TREE/migrations/$1" 2>>"$W/migrate.err" || return 1
  "${PSQL[@]}" -c "INSERT INTO csm_migration_applied_migration (filename) VALUES ('$1')"
  # Migration 0130 backfills named project_type rows that a real environment
  # syncs in and a fresh database lacks; supply them as soon as the table exists.
  case "$1" in 0031_*) "${PSQL[@]}" -o /dev/null -f "$HERE/fixture-0031.sql" ;; esac
}
deferred=()
for f in $files; do
  if apply_one "$f"; then n=$((n+1)); else deferred+=("$f"); fi
done
# A schema migration that depends on a table a legacy-phase file creates
# (e.g. a 4-digit file altering a table only a 6-digit file creates) fails the
# first time on an empty database. Retry the failures after the whole pass until
# no more progress; whatever still fails is fatal. The deferred names are
# recorded so the order difference from a plain run is visible.
: > "$W/deferred.txt"
while [ "${#deferred[@]}" -gt 0 ]; do
  progress=0; still=()
  for f in "${deferred[@]}"; do
    if apply_one "$f"; then n=$((n+1)); progress=1; echo "$f" >> "$W/deferred.txt"; else still+=("$f"); fi
  done
  deferred=("${still[@]:-}")
  [ "${deferred[0]:-}" = "" ] && deferred=()
  [ "$progress" = 0 ] && break
done
# Files that still fail cannot be applied to an empty database at all (a
# legacy file for a dropped table, a policy on a table a later migration
# dropped). They are skipped, recorded, and reported: a database built here is
# not byte-identical to one built over time. APIBASELINE_STRICT=1 makes them fatal.
: > "$W/skipped.txt"
for f in "${deferred[@]:-}"; do [ -n "$f" ] && echo "$f" >> "$W/skipped.txt"; done
if [ -s "$W/skipped.txt" ]; then
  echo "[apibaseline] migrations that cannot be applied to an empty database (skipped): $(tr '\n' ' ' < "$W/skipped.txt")" >&2
  cp "$W/skipped.txt" "${OUT%.json}.skipped-migrations.txt" 2>/dev/null || true
  if [ "${APIBASELINE_STRICT:-0}" = 1 ]; then exit 3; fi
fi
cp "$W/deferred.txt" "${OUT%.json}.deferred-migrations.txt" 2>/dev/null || true
[ -s "$W/deferred.txt" ] && echo "[apibaseline] migrations that needed a retry after the legacy phase: $(tr '\n' ' ' < "$W/deferred.txt")" >&2
echo "[apibaseline] migrations applied: $n" >&2

# 3. Application role: NOSUPERUSER NOBYPASSRLS, so row-level security applies
# to the server exactly as it does in a deployment.
"${PSQL[@]}" <<SQL
CREATE ROLE $APPROLE LOGIN PASSWORD '$APPPW' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
GRANT CONNECT ON DATABASE $PGDB TO $APPROLE;
GRANT USAGE ON SCHEMA public TO $APPROLE;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO $APPROLE;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO $APPROLE;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO $APPROLE;
SQL

# 4. Seed (fictional data, applied as the superuser).
"${PSQL[@]}" -o /dev/null -f "$HERE/seed.sql"
echo "[apibaseline] seed applied" >&2

# 5. Build the server and the suite tool.
(cd "$TREE" && go build -o "$W/entity-service" ./cmd/api)
build_tool "$W/apibaseline"

# 6. Token-signing key and a local key-set endpoint (the server fetches it at startup).
"$W/apibaseline" keygen --dir "$W/keys"
"$W/apibaseline" jwks --dir "$W/keys" --port "$JWKS_PORT" >"$W/jwks.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 50); do curl -fs "http://127.0.0.1:$JWKS_PORT/jwks.json" >/dev/null 2>&1 && break; sleep 0.1; done

# 7. Start the server from a scratch cwd (so no .env is picked up).
mkdir -p "$W/cwd"
(
  cd "$W/cwd"
  env -i PATH="$PATH" HOME="$W" \
    DATA_SOURCE=postgres \
    DB_HOST=127.0.0.1 DB_PORT="$PGPORT_" DB_USER="$APPROLE" DB_PASSWORD="$APPPW" DB_NAME="$PGDB" DB_SSLMODE=disable \
    SERVER_PORT="$SRV_PORT" HEALTH_PORT="$HEALTH_PORT" \
    AUTH_ISSUER="https://issuer.example.invalid/token" AUTH_JWKS_URL="http://127.0.0.1:$JWKS_PORT/jwks.json" \
    AUTH_USER_TOKEN_AUDIENCES="baseline-spa" \
    CSM_PORTAL_BACKEND_CLIENT_ID="baseline-csm-backend" CSM_PORTAL_USER_DOMAIN="example.com" \
    CUSTOMER_PORTAL_BACKEND_CLIENT_ID="baseline-customer-backend" \
    M2M_CLIENT_IDS="baseline-m2m" \
    "$W/entity-service" >"$W/server.log" 2>&1
) &
PIDS+=($!)
ready=0
for _ in $(seq 1 150); do
  if curl -fs "http://127.0.0.1:$HEALTH_PORT/health" >/dev/null 2>&1; then ready=1; break; fi
  if ! kill -0 "${PIDS[1]}" 2>/dev/null; then break; fi
  sleep 0.2
done
if [ "$ready" != 1 ]; then echo "[apibaseline] server did not become ready" >&2; tail -20 "$W/server.log" >&2; KEEP=1; exit 4; fi
echo "[apibaseline] server ready on :$SRV_PORT" >&2

if [ "$HOLD" = 1 ]; then
  echo "[apibaseline] HOLD: base=http://127.0.0.1:$SRV_PORT keys=$W/keys db=$SUPER" >&2
  echo "$W $SRV_PORT $PGPORT_" > "${OUT:-/tmp/ab-hold}.hold"
  while :; do sleep 3600; done
fi

# 8. Run the scenarios.
rc=0
"$W/apibaseline" run --base-url "http://127.0.0.1:$SRV_PORT" --dir "$W/keys" \
  --issuer "https://issuer.example.invalid/token" --db "$SUPER" --psql "$PG_BIN/psql" \
  --tree "$TREE" --out "$OUT" || rc=$?
cp "$W/server.log" "${OUT%.json}.server.log" 2>/dev/null || true
exit $rc

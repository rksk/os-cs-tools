#!/usr/bin/env bash
# Migration upgrade test: applies this tree's migrations to a throwaway PostgreSQL
# cluster, either on top of an existing schema (an upgrade) or on an empty
# database, and records the outcome and duration of every file.
#
# The apply loop reproduces the `migrate` target in entity-service/Makefile:
#   - files in this order: migrations/NNNN_*.sql ascending, then the RLS track
#     migrations/1NNNNN_*.sql ascending, then legacy migrations/0NNNNN_*.up.sql
#   - a file is skipped when its basename is already in
#     csm_migration_applied_migration
#   - each file runs through `psql -v ON_ERROR_STOP=1 -f`, and the first failure
#     stops the run (unless --keep-going is given, which is a diagnostic mode the
#     Makefile does not have)
#
# Usage:
#   migration-upgrade-test.sh --migrations DIR --out DIR [options]
#
# Options:
#   --migrations DIR     directory holding the migration files (required)
#   --out DIR            where results go: results.tsv, run.log, errors/,
#                        schema-after.sql (required; created if missing)
#   --base-schema FILE   schema-only dump to load first (an upgrade test). Omit it
#                        to start from an empty database.
#   --applied-list FILE  newline-separated migration filenames to record as
#                        already applied before the run (only with --base-schema)
#   --schema NAME        schema the migrations run in, put first on the
#                        database search_path (default: csm_platform). Created
#                        when it does not exist.
#   --seed FILE          SQL to load after the base schema and before the
#                        migrations. Repeatable; loaded in the order given.
#   --keep-going         on failure, record it and continue with the next file
#                        (the failed file is not recorded as applied)
#   --pg-bin DIR         directory with initdb, pg_ctl, psql, pg_dump
#                        (default: from PATH)
#   --port N             port for the throwaway cluster (default: random free)
#   --keep               leave the cluster running and its data dir in place
#
# Exit code: 0 when every file applied or was skipped, 1 when a migration failed,
# 2 for a setup problem.
#
# The inputs (--base-schema, --applied-list) describe a real database; keep them
# outside the repository. The seed files next to this script are fictional.

set -euo pipefail

MIG_DIR="" OUT="" BASE_SCHEMA="" APPLIED_LIST="" SCHEMA="csm_platform" PG_BIN="" PORT="" KEEP=0 KEEP_GOING=0
SEEDS=()

die() { echo "error: $*" >&2; exit 2; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --migrations)   MIG_DIR=$2; shift 2 ;;
    --out)          OUT=$2; shift 2 ;;
    --base-schema)  BASE_SCHEMA=$2; shift 2 ;;
    --applied-list) APPLIED_LIST=$2; shift 2 ;;
    --schema)       SCHEMA=$2; shift 2 ;;
    --seed)         SEEDS+=("$2"); shift 2 ;;
    --keep-going)   KEEP_GOING=1; shift ;;
    --pg-bin)       PG_BIN=$2; shift 2 ;;
    --port)         PORT=$2; shift 2 ;;
    --keep)         KEEP=1; shift ;;
    -h|--help)      sed -n '2,40p' "$0"; exit 0 ;;
    *)              die "unknown argument: $1" ;;
  esac
done

[[ -n "$MIG_DIR" && -d "$MIG_DIR" ]] || die "--migrations DIR is required and must exist"
[[ -n "$OUT" ]] || die "--out DIR is required"
[[ -z "$BASE_SCHEMA" || -f "$BASE_SCHEMA" ]] || die "base schema not found: $BASE_SCHEMA"
[[ -z "$APPLIED_LIST" || -f "$APPLIED_LIST" ]] || die "applied list not found: $APPLIED_LIST"
[[ -z "$APPLIED_LIST" || -n "$BASE_SCHEMA" ]] || die "--applied-list needs --base-schema"
[[ "$SCHEMA" =~ ^[a-z_][a-z0-9_]*$ ]] || die "--schema must be a lowercase identifier"
for s in ${SEEDS[@]+"${SEEDS[@]}"}; do [[ -f "$s" ]] || die "seed not found: $s"; done

bin() { if [[ -n "$PG_BIN" ]]; then echo "$PG_BIN/$1"; else command -v "$1" || die "$1 not on PATH"; fi; }
INITDB=$(bin initdb); PG_CTL=$(bin pg_ctl); PSQL=$(bin psql); PG_DUMP=$(bin pg_dump)

now_ms() { perl -MTime::HiRes=time -e 'printf "%d\n", time()*1000'; }

MIG_DIR=$(cd "$MIG_DIR" && pwd)
mkdir -p "$OUT/errors"
OUT=$(cd "$OUT" && pwd)
: > "$OUT/run.log"
printf 'order\tfile\toutcome\tms\n' > "$OUT/results.tsv"
log() { echo "$*" | tee -a "$OUT/run.log"; }

if [[ -z "$PORT" ]]; then
  PORT=$(perl -MIO::Socket::INET -e 'my $s=IO::Socket::INET->new(Listen=>1,LocalAddr=>"127.0.0.1",LocalPort=>0) or die; print $s->sockport')
fi
DATA=$(mktemp -d "${TMPDIR:-/tmp}/migup-pg.XXXXXX")
cleanup() {
  if [[ $KEEP -eq 1 ]]; then
    echo "cluster kept: data=$DATA port=$PORT" >&2
  else
    "$PG_CTL" -D "$DATA/db" -m immediate stop >/dev/null 2>&1 || true
    rm -rf "$DATA"
  fi
}
trap cleanup EXIT

log "== cluster: $("$PSQL" --version) port=$PORT"
"$INITDB" -D "$DATA/db" -U postgres -A trust -E UTF8 --locale=C >"$DATA/initdb.log" 2>&1 || { cat "$DATA/initdb.log" >&2; die "initdb failed"; }
"$PG_CTL" -D "$DATA/db" -l "$DATA/server.log" -w \
  -o "-p $PORT -k $DATA -c listen_addresses=127.0.0.1 -c fsync=off -c timezone=UTC" start >/dev/null \
  || { cat "$DATA/server.log" >&2; die "server did not start"; }

DB=migration_upgrade_test
ADMIN="postgresql://postgres@127.0.0.1:$PORT/postgres"
CONN="postgresql://postgres@127.0.0.1:$PORT/$DB"
"$PSQL" "$ADMIN" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $DB" >/dev/null

if [[ -n "$BASE_SCHEMA" ]]; then
  log "== base schema: $(basename "$BASE_SCHEMA")"
  t0=$(now_ms)
  "$PSQL" "$CONN" -X -q -v ON_ERROR_STOP=1 -f "$BASE_SCHEMA" >"$OUT/base-schema.log" 2>&1 \
    || { tail -5 "$OUT/base-schema.log" >&2; die "base schema did not load"; }
  log "   loaded in $(( $(now_ms) - t0 )) ms"
fi

# The schema the migrations run in goes first on the database's search_path, the
# way a deployment's connection settings would put it.
"$PSQL" "$CONN" -X -q -v ON_ERROR_STOP=1 \
  -c "CREATE SCHEMA IF NOT EXISTS $SCHEMA" \
  -c "ALTER DATABASE $DB SET search_path TO $SCHEMA, public" >/dev/null

if [[ -n "$APPLIED_LIST" ]]; then
  "$PSQL" "$CONN" -X -q -v ON_ERROR_STOP=1 \
    -c "CREATE TABLE IF NOT EXISTS csm_migration_applied_migration (filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());" >/dev/null
  grep -v '^[[:space:]]*$' "$APPLIED_LIST" | "$PSQL" "$CONN" -X -q -v ON_ERROR_STOP=1 \
    -c "\copy csm_migration_applied_migration (filename) FROM STDIN" >/dev/null
  log "== recorded as applied beforehand: $("$PSQL" "$CONN" -X -tAc "SELECT count(*) FROM csm_migration_applied_migration")"
fi

for s in ${SEEDS[@]+"${SEEDS[@]}"}; do
  log "== seed: $(basename "$s")"
  "$PSQL" "$CONN" -X -q -v ON_ERROR_STOP=1 -f "$s" >>"$OUT/seed.log" 2>&1 \
    || { tail -5 "$OUT/seed.log" >&2; die "seed failed: $s"; }
done

# ---- the apply loop, as in the Makefile's migrate target --------------------
"$PSQL" "$CONN" -v ON_ERROR_STOP=1 -c "CREATE TABLE IF NOT EXISTS csm_migration_applied_migration (filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());" >/dev/null || exit 1

cd "$MIG_DIR/.."
mig=$(basename "$MIG_DIR")
files=$( { ls "$mig"/[0-9][0-9][0-9][0-9]_*.sql 2>/dev/null | sort; \
           ls "$mig"/1[0-9][0-9][0-9][0-9][0-9]_*.sql 2>/dev/null | sort; \
           ls "$mig"/0[0-9][0-9][0-9][0-9][0-9]_*.up.sql 2>/dev/null | sort; } )

n=0 applied=0 skipped=0 failed=0 first_failure=""
for f in $files; do
  n=$((n + 1))
  name=$(basename "$f")
  name_escaped=$(printf '%s' "$name" | sed "s/'/''/g")
  is_applied=$("$PSQL" "$CONN" -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM csm_migration_applied_migration WHERE filename = '$name_escaped';")
  if [[ "$is_applied" == "1" ]]; then
    printf '%d\t%s\tskipped\t\n' "$n" "$name" >> "$OUT/results.tsv"
    skipped=$((skipped + 1))
    continue
  fi
  t0=$(now_ms)
  if "$PSQL" "$CONN" -v ON_ERROR_STOP=1 -f "$f" >"$OUT/errors/$name.out" 2>"$OUT/errors/$name.err"; then
    ms=$(( $(now_ms) - t0 ))
    "$PSQL" "$CONN" -v ON_ERROR_STOP=1 -c "INSERT INTO csm_migration_applied_migration (filename) VALUES ('$name_escaped');" >/dev/null || exit 1
    printf '%d\t%s\tapplied\t%d\n' "$n" "$name" "$ms" >> "$OUT/results.tsv"
    log "applied  $name (${ms} ms)"
    applied=$((applied + 1))
    rm -f "$OUT/errors/$name.out"
    [[ -s "$OUT/errors/$name.err" ]] || rm -f "$OUT/errors/$name.err"
  else
    ms=$(( $(now_ms) - t0 ))
    printf '%d\t%s\tFAILED\t%d\n' "$n" "$name" "$ms" >> "$OUT/results.tsv"
    log "FAILED   $name (${ms} ms): $(grep -m1 -E 'ERROR|FATAL' "$OUT/errors/$name.err" || tail -1 "$OUT/errors/$name.err")"
    failed=$((failed + 1))
    [[ -n "$first_failure" ]] || first_failure=$name
    if [[ $KEEP_GOING -eq 0 ]]; then
      for rest in $(printf '%s\n' $files | tail -n +$((n + 1))); do
        n=$((n + 1)); printf '%d\t%s\tnot-run\t\n' "$n" "$(basename "$rest")" >> "$OUT/results.tsv"
      done
      break
    fi
  fi
done

not_run=$(awk -F'\t' '$3=="not-run"' "$OUT/results.tsv" | wc -l | tr -d ' ')
log "== summary: files=$(printf '%s\n' $files | wc -l | tr -d ' ') applied=$applied skipped=$skipped failed=$failed not_run=$not_run first_failure=${first_failure:-none}"

recorded_missing=$("$PSQL" "$CONN" -X -tAc "SELECT count(*) FROM csm_migration_applied_migration" )
log "== rows in csm_migration_applied_migration after run: $recorded_missing"

"$PG_DUMP" --schema-only --no-owner --no-privileges "$CONN" | grep -v '^\\\(un\)\?restrict ' > "$OUT/schema-after.sql"
log "== schema dump: $OUT/schema-after.sql"

[[ $failed -eq 0 ]]

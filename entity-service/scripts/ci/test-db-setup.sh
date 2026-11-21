#!/usr/bin/env bash
# Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.
#
# Prepares a throwaway PostgreSQL database for entity-service's integration
# tests, for CI and for running the same thing locally.
#
#   1. Applies every migration from an empty database via `make migrate`
#      (all three phases, in the canonical order).
#   2. Creates the application role: NOSUPERUSER NOBYPASSRLS, so tests that
#      prove row-level security are not silently exempt from it.
#   3. Prints one NAME=value line per test DSN variable on stdout, ready to
#      append to $GITHUB_ENV or to `export`.
#
# Inputs (environment):
#   SUPER_URL  superuser connection URL of the empty database (required)
#   APP_URL    connection URL of the application role (required); its user and
#              password are used to create the role
#
# Which variable gets which role: see the lists below. A variable is on the
# application role unless its tests seed rows straight into tables that
# row-level security would reject for a non-superuser.
set -euo pipefail

: "${SUPER_URL:?SUPER_URL is required}"
: "${APP_URL:?APP_URL is required}"

cd "$(dirname "$0")/../.."   # entity-service/

# Everything below except the final listing goes to stderr, so stdout stays a
# clean NAME=value list.
{
  DATABASE_URL="$SUPER_URL" make migrate
  echo "migrations applied: $(psql "$SUPER_URL" -tAc 'SELECT count(*) FROM csm_migration_applied_migration')"

  app_user=$(psql "$SUPER_URL" -tAc "SELECT split_part(split_part('${APP_URL}', '://', 2), ':', 1)")
  app_pass=$(psql "$SUPER_URL" -tAc "SELECT split_part(split_part(split_part('${APP_URL}', '://', 2), '@', 1), ':', 2)")
  psql "$SUPER_URL" -v ON_ERROR_STOP=1 -v app_user="$app_user" -v app_pass="$app_pass" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE', :'app_user', :'app_pass')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')
\gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), :'app_user') \gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I', :'app_user') \gexec
SELECT format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %I', :'app_user') \gexec
-- UPDATE as well as USAGE, SELECT: setval needs it, and the application
-- itself calls setval on the deployed-product number sequence when it moves it
-- past numbers already stored (advanceDeployedProductNumberSeqQuery).
SELECT format('GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO %I', :'app_user') \gexec
SELECT format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO %I', :'app_user') \gexec
SQL
  echo "application role: $(psql "$SUPER_URL" -tAc "SELECT rolname || ' super=' || rolsuper || ' bypassrls=' || rolbypassrls FROM pg_roles WHERE rolname = '${app_user}'")"
} >&2

# Which role each variable gets. Chosen from a run of every variable under both
# roles on a fresh database, not guessed: a variable is on the application role
# when its tests pass there (that is the stronger run, since the application
# role is subject to row-level security), and on the superuser when its tests
# seed past row-level security, alter tables or create databases, which the
# application role cannot do.
APP_VARS=(
  ANNOUNCEMENT_VISIBILITY_TEST_DSN
  CALL_REQUEST_TEST_DSN
  CASE_CREATE_RLS_TEST_DSN
  CASE_STATS_TEST_DSN
  GITHUB_SYNC_TEST_DSN
  INCIDENT_AUTOCLOSE_TEST_DSN
  INCIDENT_CREATE_TEST_DSN
  INCIDENT_HANDOFF_TEST_DSN
  INCIDENT_TASK_GATE_TEST_DSN
  PG_OUTAGE_TEST_DSN
  SR_NOTICE_TEST_DSN
  STORED_ATTACHMENT_TEST_DSN
)
SUPER_VARS=(
  ALERT_MUTE_RULE_TEST_DSN
  CHANGE_REQUEST_TEST_DSN
  ENTITY_SERVICE_TEST_PG_DSN
  ENTITY_TEST_DATABASE_URL
  INCIDENT_CREATE_GROUP_TEST_DSN
  INCIDENT_LIFECYCLE_TEST_DSN
  INCIDENT_REPORT_TEST_DSN
  INSTANCE_INGESTION_TEST_DSN
  IT_SERVICE_TEST_DSN
  MIGRATION_TEST_DSN
  OUTAGE_COMM_TEST_DSN
  OUTAGE_NOTIFY_TEST_DSN
  PAGING_CONTACT_TEST_DSN
  RUNBOOK_TEST_DSN
)
# MIGRATION_TEST_DSN: the KB manager, knowledge-article foreign key and KB
# article history migration tests create schemas and run DDL on the real tables
# inside a rolled-back transaction (CREATE on the database, ownership of the
# tables). The application role has neither, and must not: they are DDL tests,
# not row-level-security tests, so the superuser is the right connection.
#
# CASE_STATS_TEST_DSN stays on the application role, although some of its tests
# once failed there. Those fixtures inserted the case and engagement extension
# rows through the bare pool, which carries no identity, so row-level security
# rejected the insert; the fixtures now go through the scoped pool with the
# system identity, as the rest of the package does, so the stronger application
# role run stays and no test moves to the superuser to get past a policy.
#
# Deliberately NOT set (their tests need a populated deployment, not a fresh
# schema, and skip via scripts/ci/skip-allowlist.txt): CLOUD_STATUS_TEST_DSN,
# STANDARD_CHANGE_TEST_DSN, STANDARD_CHANGE_TEST_APP_DSN, PLG_TEST_DATABASE_URL
# (its harness builds its own database with a naive migration glob that no
# longer works against the current migrations, and a TestMain failure takes the
# whole repository package down), and every *_LIVE_DB_URL (those read a real
# deployment and must never point at a CI database).
for v in "${SUPER_VARS[@]}"; do echo "$v=$SUPER_URL"; done
for v in "${APP_VARS[@]}"; do echo "$v=$APP_URL"; done

-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
--
-- WSO2 LLC. licenses this file to you under the Apache License,
-- Version 2.0 (the "License"); you may not use this file except
-- in compliance with the License.
-- You may obtain a copy of the License at
--
-- http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing,
-- software distributed under the License is distributed on an
-- "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
-- KIND, either express or implied.  See the License for the
-- specific language governing permissions and limitations
-- under the License.

-- Data repair for a bug in MarkProjectHealthy (fixed alongside this
-- migration, internal/repository/customer_health_repo.go): marking a
-- project healthy never closed a project_risk row that was still genuinely
-- OPEN underneath it, so a project reached through a path that skips the
-- Close Risk step (e.g. "Mark Healthy" offered directly from a "to be
-- reviewed" display caused by an unrelated, now-fixed sys_id-format bug on
-- the caller side) could end up with project_health_status.status =
-- 'HEALTHY' while its project_risk table still held an OPEN row from
-- before. GetAccountProjectHealthStatuses only looks up a project's open
-- risk while its health status is AT_RISK, so that orphaned OPEN row became
-- invisible to every read and to the UI's own Close Risk action -- yet
-- OpenProjectRisk's own "does this project already have an open risk"
-- check still found it, so every later "Mark At Risk" attempt on the same
-- project failed forever with "project already has an open risk".
--
-- This closes every project_risk row that is still OPEN on a project whose
-- current health status is not AT_RISK -- the exact inconsistent
-- combination the bug produces, and the one case the application's own
-- invariant (an OPEN risk implies the project is AT_RISK) disallows. Safe
-- to re-run: it only ever matches rows still in that inconsistent state.

BEGIN;

UPDATE project_risk pr
SET status = 'CLOSED',
    closed_comment = 'Closed automatically: this risk was left open after the project was marked healthy, due to a bug in MarkProjectHealthy (now fixed).',
    closed_by_email = 'system',
    closed_on = NOW()
FROM project_health_status phs
WHERE phs.project_id = pr.project_id
  AND pr.status = 'OPEN'
  AND phs.status <> 'AT_RISK';

COMMIT;

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

-- A window whose tiers are one crew for the whole rota, not one per team.
--
-- SaaS SRE's night (SRE_TZ3, 21:00-06:00 LK, every day) is worked by ONE L1,
-- L2 and L3 for Apollo and Artemis together: an alert from either team at
-- night pages that night's TZ3 crew. Every other zoned window is one crew per
-- team -- Apollo's TZ1 L1 and Artemis's TZ1 L1 are two different people.
--
-- That difference decides what "somebody already holds this tier" means when
-- a lead sets a turn: on a per-team window it is the same team's holder, on a
-- shared one it is anybody on the rota. It is a fact about the window, so it
-- is kept on the window rather than spelt out as a code in Go -- a rota that
-- later shares a window says so here, without a release.
--
-- Additive only: one new column, FALSE for every existing window, and TRUE
-- for SRE_TZ3. IaaS's night (SRE_IAAS_NIGHT) stays per team, as do every CRE
-- and SME window. The column is not one the used-shift guard (0153) freezes,
-- so the update below is allowed on a window that already has assignments.
--
-- One transaction with a short lock timeout, as the other schedule
-- migrations; safe to re-run.
BEGIN;
SET LOCAL lock_timeout = '5s';

ALTER TABLE team_schedule_shift
    ADD COLUMN IF NOT EXISTS shared_across_teams BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN team_schedule_shift.shared_across_teams IS
    'TRUE when each tier of this window is held by one person for the whole rota (every team of the zone''s rota), not one per team. SaaS SRE_TZ3 is the one such window.';

UPDATE team_schedule_shift
   SET shared_across_teams = TRUE, updated_on = NOW(), updated_by = 'migration'
 WHERE code = 'SRE_TZ3' AND NOT shared_across_teams;

COMMIT;

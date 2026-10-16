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

-- The TZ3 people a lead chose for a month of a generated rota.
--
-- "Generate month" lets a lead choose who works SaaS TZ3 (the night cover,
-- one crew for Apollo and Artemis): those people take the month's TZ3 L1, L2
-- and L3 and work no other zone. The choice used to live only in the lead's
-- browser, so nobody else -- and not the Month roster -- could tell who the
-- night crew was. A real (not dry-run) generation now records it here, one row
-- per person, replacing the month's earlier choice; generating the month again
-- with nobody chosen clears it, which is how somebody goes back to their team.
--
-- The Month roster reads it to list those people under "Night cover" instead
-- of under their team. Nothing pages from it: who is called is still the
-- turns themselves.
--
-- Additive only: one new table, nothing else touched. Safe to re-run.
BEGIN;
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS team_schedule_night_crew (
    -- The generated rota (team_schedule_rota.code), e.g. SRE_SAAS.
    rota_code   VARCHAR(16)  NOT NULL,
    -- The first day of the month the choice is for.
    month       DATE         NOT NULL CHECK (EXTRACT(DAY FROM month) = 1),
    user_id     UUID         NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    -- The team the person was chosen under (team.key): their own team, whose
    -- row they leave while on the night crew.
    team_key    VARCHAR(64)  NOT NULL,
    created_on  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by  VARCHAR(255) NULL,
    PRIMARY KEY (rota_code, month, user_id)
);

COMMIT;

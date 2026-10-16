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

-- SaaS SRE's night cover as a team of its own: "SaaS Night cover".
--
-- The SaaS night (TZ3, 21:00-06:00 LK) is worked by a few Apollo and Artemis
-- engineers who go onto it for three to six months and then come back. Until
-- now the roster could only show them as a "Night cover" group drawn from the
-- months Generate month had recorded a crew for, so a month made any other
-- way showed nobody. This makes it what CRE's Americas team already is: a
-- team people are moved to for a span, by a tag, and shown under while it
-- lasts.
--
-- Data only: no table, column, type or constraint is created or changed.
--
--   * the team, SaaS Night cover, typed sre-abt so it is on the SaaS rota
--     beside Apollo and Artemis. Nobody is a member: everyone on it is moved
--     there by the tag below, and stays a member of their own team;
--   * the tag that moves somebody there, "SaaS Night cover", SRE only. It is
--     worked there (works_rota_there) and has no hours of its own
--     (shows_as_shift_code NULL): being on the team is the whole of it. L1,
--     L2 and L3 stay turns on SRE_TZ3, still one crew for the whole rota
--     (0223), and only those page.
--
-- Nobody is moved by this script: a lead moves their own people from the
-- roster, for the months they are on nights. Their TZ3 turns already set in
-- those months go with them (see syncMoveShifts in the schedule repository).
--
-- Re-runnable: the team is added only when no team has its key, and the tag
-- only when its code is new. One transaction with a short lock timeout, as
-- the other schedule migrations.

BEGIN;
SET LOCAL lock_timeout = '5s';

-- ── The team ──────────────────────────────────────────────────────────────
INSERT INTO team (id, created_on, updated_on, created_by, updated_by, name, key, type)
SELECT gen_random_uuid(), NOW(), NOW(), 'migration', 'migration', 'SaaS Night cover', 'saas-night-cover', 'sre-abt'
WHERE NOT EXISTS (SELECT 1 FROM team t WHERE lower(t.key) = 'saas-night-cover');

-- ── The tag that moves somebody onto it ───────────────────────────────────
INSERT INTO team_schedule_absence_kind
    (code, short_code, label, bucket, colour_token, sort_order, is_active, family,
     moves_to_team_key, works_rota_there, created_by, updated_by)
SELECT 'SRE_NIGHT_COVER', 'NC', 'SaaS Night cover', 'ALLOCATION', 'NLK', 75, TRUE, 'SRE',
       t.key, TRUE, 'migration', 'migration'
  FROM team t
 WHERE t.key = 'saas-night-cover'
ON CONFLICT (code) DO NOTHING;

COMMIT;

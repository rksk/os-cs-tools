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

-- The team rows of the seven SME rotations migration 0200 created: Asgardeo,
-- Choreo Runtime, Bijira, Devant, WSO2 Cloud · Agent platform, WSO2 Cloud ·
-- Core and Moesif; and IaaS's, for SRE's IaaS rota (SRE_IAAS, type sre-iaas).
--
-- 0200 added their rotas, zones and windows but no team: a team's rota is read
-- from its type, and teams were expected to arrive from the ServiceNow sync.
-- These seven never do (they are Team Schedule rotations, not ServiceNow
-- groups), so on a deployed database the SME rotas have no team, nobody can be
-- rostered on them, and an escalation to Special Ops pages nobody: the SME page
-- looks the team up by the key entity-service's SPECIALIST_HANDOFF_CONFIG names
-- (smeTeam) and finds none. 0217 already added U2's and B-Central's team rows
-- this way; this does the same for the other seven, with the keys, names and
-- types the local stack has always used (scripts/csm-compose/
-- seed-team-schedule-rotas.sql), so a key that works locally works everywhere.
-- IaaS's team (key iaas) is on production already but not on a database that
-- did not get it from there, such as staging: its IaaS rota then lists no team
-- and its windows (0231) have nobody to roster. Where it exists this is a no-op.
--
-- Data only: no table, column, type or constraint is created or changed.
-- Re-runnable: a team is added only when no team has its key (compared without
-- case). A key already held by a team of another type is left alone (never
-- re-typed: that team may be in use) and named in a NOTICE, so whoever runs
-- this sees which rota is still without its team. One transaction with a
-- short lock timeout, as the other schedule migrations.
BEGIN;
SET LOCAL lock_timeout = '5s';

INSERT INTO team (id, created_on, updated_on, created_by, updated_by, name, key, type)
SELECT gen_random_uuid(), NOW(), NOW(), 'migration', 'migration', v.name, v.key, v.type
FROM (VALUES
    ('Asgardeo',                    'asgardeo',       'sme-asgardeo'),
    ('Choreo Runtime',              'choreo-runtime', 'sme-choreo-runtime'),
    ('Bijira',                      'bijira',         'sme-bijira'),
    ('Devant',                      'devant',         'sme-devant'),
    ('WSO2 Cloud · Agent platform', 'cloud-agent',    'sme-cloud-agent'),
    ('WSO2 Cloud · Core',           'cloud-core',     'sme-cloud-core'),
    ('Moesif',                      'moesif',         'sme-moesif'),
    ('IaaS',                        'iaas',           'sre-iaas')
) AS v(name, key, type)
WHERE NOT EXISTS (SELECT 1 FROM team t WHERE lower(t.key) = v.key);

DO $$
DECLARE
    clash RECORD;
BEGIN
    FOR clash IN
        SELECT t.key, t.type, v.type AS wanted
        FROM team t
        JOIN (VALUES
            ('asgardeo',       'sme-asgardeo'),
            ('choreo-runtime', 'sme-choreo-runtime'),
            ('bijira',         'sme-bijira'),
            ('devant',         'sme-devant'),
            ('cloud-agent',    'sme-cloud-agent'),
            ('cloud-core',     'sme-cloud-core'),
            ('moesif',         'sme-moesif'),
            ('iaas',           'sre-iaas')
        ) AS v(key, type) ON lower(t.key) = v.key
        WHERE t.type IS DISTINCT FROM v.type
    LOOP
        RAISE NOTICE 'team % already exists with type %, not %: that rota has no team until this is resolved by hand',
            clash.key, clash.type, clash.wanted;
    END LOOP;
END
$$;

COMMIT;

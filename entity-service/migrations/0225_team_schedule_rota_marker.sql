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

-- Rota access: a rota marker per team, and a history of rota grants.
--
-- Until now a team's rota could be marked by its lead, or by a rota admin for
-- the whole family (cre_rota_admin, sre_rota_admin, sme_rota_admin -- 0156,
-- 0200). Teams want somebody who is not the lead to keep their own rota up to
-- date -- rotations, leave, allocations -- without being handed every team in
-- the family. A rota marker is that: one person, one team.
--
-- Its own table rather than a new team_member.role, because team_member is
-- written by the ServiceNow sync and its role feeds the escalation ladder; a
-- marker is a schedule permission, granted and taken back on the Team
-- Schedule's Rota access tab. A grant only counts while the holder is still a
-- member of the team and internal staff; the service checks both, so a
-- leaver's grant simply stops working rather than needing a clean-up.
--
-- The same audit trigger the rota tables use (0155) records every grant and
-- removal. Rota admin grants live in user_role, which the ServiceNow and
-- Salesforce syncs also write; only the three rota admin roles are recorded,
-- so the history holds rota changes and nothing else.
--
-- Additive and re-runnable. One transaction with a short lock timeout, as
-- 0155 and 0214: user_role is written by the syncs, and creating its trigger
-- must not queue those writes behind it.

BEGIN;
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS team_schedule_rota_marker (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_on  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_on  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  VARCHAR(255) NOT NULL,
    updated_by  VARCHAR(255) NOT NULL,
    user_id     UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    team_key    VARCHAR(64)  NOT NULL REFERENCES team(key) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT team_schedule_rota_marker_user_team_uq UNIQUE (user_id, team_key)
);

COMMENT ON TABLE team_schedule_rota_marker IS
    'Who may mark a team''s rota (rotations, leave, allocations) without leading it. Granted on the Team Schedule''s Rota access tab.';

-- "Who marks for this team" is the tab's question; the unique index above
-- already answers "what does this person mark".
CREATE INDEX IF NOT EXISTS team_schedule_rota_marker_team_idx
    ON team_schedule_rota_marker (team_key);

DROP TRIGGER IF EXISTS team_schedule_rota_marker_audit ON team_schedule_rota_marker;
CREATE TRIGGER team_schedule_rota_marker_audit
    AFTER INSERT OR UPDATE OR DELETE ON team_schedule_rota_marker
    FOR EACH ROW EXECUTE FUNCTION team_schedule_audit_row();

-- user_role carries every role there is; only a rota admin grant is a rota
-- change. A trigger's WHEN cannot look the role up, so the function does and
-- hands the rest to team_schedule_audit_row()'s own rules by writing the same
-- columns. A grant is an insert and a removal a delete; nothing updates a
-- user_role row in place.
CREATE OR REPLACE FUNCTION team_schedule_audit_rota_admin_role() RETURNS TRIGGER AS $$
DECLARE
    v_row user_role;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_row := OLD;
    ELSE
        v_row := NEW;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM role r
                    WHERE r.id = v_row.role_id
                      AND r.name IN ('cre_rota_admin', 'sre_rota_admin', 'sme_rota_admin')) THEN
        RETURN NULL;
    END IF;

    INSERT INTO team_schedule_audit (table_name, row_id, action, actor, old_row, new_row)
    VALUES (
        TG_TABLE_NAME,
        v_row.id,
        TG_OP,
        COALESCE(NULLIF(current_setting('app.actor', true), ''), v_row.updated_by, v_row.created_by),
        CASE WHEN TG_OP = 'DELETE' THEN to_jsonb(OLD) END,
        CASE WHEN TG_OP = 'INSERT' THEN to_jsonb(NEW) END);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS user_role_rota_admin_audit ON user_role;
CREATE TRIGGER user_role_rota_admin_audit
    AFTER INSERT OR DELETE ON user_role
    FOR EACH ROW EXECUTE FUNCTION team_schedule_audit_rota_admin_role();

COMMIT;

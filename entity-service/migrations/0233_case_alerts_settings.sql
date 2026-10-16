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

-- Case Alerts settings: the chat rooms and ServiceNow group routing the
-- paging ladders use, kept here so an admin can change them from the Team
-- Schedule's Alert Settings tab instead of a Choreo secret and the YAML file
-- mount of csm-notification-service.
--
-- Nothing reads these tables until csm-notification-service is started with
-- PAGING_CONFIG_SOURCE set; until then the file and env vars are all there
-- is. Even then a row only replaces the file's value when it exists and is
-- enabled, so an empty table changes nothing.
--
--   paging_chat_room    one row per room: a ladder's shared room (team_key
--                       NULL) or an SRE team's own room, with (team rooms) how
--                       that team's alerts go out. No secret in it, only the
--                       URL's last characters (webhook_hint).
--   paging_chat_secret  the webhook URL, encrypted by entity-service (AES-GCM,
--                       key from CASE_ALERTS_SECRET_KEY). Its own table so no
--                       ordinary read or join touches it. Deliberately NOT
--                       audited: team_schedule_audit_row copies whole rows, and
--                       a webhook URL lets anyone who holds it post into the
--                       room. A replaced URL shows in the history as a new
--                       webhook_hint on the room.
--   paging_group_route  a ServiceNow group name to a team, or (SRE) to a
--                       default rota.
--   paging_ladder_value one row per other ladder setting saved here: a key of
--                       the file's cre: or sre: section (teams.abts,
--                       timing.interval, heads.cre, ...) and its value as
--                       JSON. entity-service checks each key and its limits;
--                       the safety caps, channel and on/off switches are not
--                       among them and stay in the file.
--   paging_config_revision
--                       one row: a number every change above moves on, and
--                       what the running csm-notification-service last
--                       reported it loaded.
--
-- Room, route and ladder value changes are audited by the same trigger as the
-- rota tables (0155). Additive and re-runnable; one transaction with a short
-- lock timeout, as 0155 and 0216.

BEGIN;
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS paging_chat_room (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_on       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_on       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by       VARCHAR(255) NOT NULL,
    updated_by       VARCHAR(255) NOT NULL,
    ladder           VARCHAR(8)   NOT NULL,
    -- NULL: the ladder's shared room. Otherwise the SRE team whose room it is.
    team_key         VARCHAR(64)  NULL REFERENCES team(key) ON UPDATE CASCADE ON DELETE CASCADE,
    -- The room's name in logs and on the tab, as the file's "audience".
    label            VARCHAR(100) NOT NULL,
    enabled          BOOLEAN      NOT NULL DEFAULT TRUE,
    -- SRE team rooms only: also gets the cards of an incident with no SRE
    -- team of its own (the file's sre.teamlessChats).
    teamless         BOOLEAN      NOT NULL DEFAULT FALSE,
    -- SRE team rooms only: how this team's alerts go out -- chat, call, both
    -- or log. NULL is the ladder's channel (the file's). The alert service
    -- never lets a team do more than its ladder: the file's channel is the
    -- ceiling.
    channel          VARCHAR(8)   NULL,
    webhook_hint     VARCHAR(64)  NULL,
    webhook_set_at   TIMESTAMPTZ  NULL,
    webhook_set_by   VARCHAR(255) NULL,
    last_test_at     TIMESTAMPTZ  NULL,
    last_test_status VARCHAR(20)  NULL,
    CONSTRAINT paging_chat_room_ladder_check
        CHECK (ladder IN ('CRE', 'SRE')),
    CONSTRAINT paging_chat_room_team_rooms_sre_check
        CHECK (team_key IS NULL OR ladder = 'SRE'),
    CONSTRAINT paging_chat_room_teamless_check
        CHECK (NOT teamless OR team_key IS NOT NULL),
    CONSTRAINT paging_chat_room_channel_check
        CHECK (channel IS NULL OR (team_key IS NOT NULL AND channel IN ('log', 'chat', 'call', 'both'))),
    CONSTRAINT paging_chat_room_label_check
        CHECK (btrim(label) <> ''),
    CONSTRAINT paging_chat_room_last_test_status_check
        CHECK (last_test_status IS NULL OR last_test_status IN ('delivered', 'failed'))
);

COMMENT ON TABLE paging_chat_room IS
    'Case Alerts chat rooms, set on the Team Schedule''s Alert Settings tab. Used by csm-notification-service only when PAGING_CONFIG_SOURCE is set.';

-- One shared room per ladder, one room per team.
CREATE UNIQUE INDEX IF NOT EXISTS paging_chat_room_scope_uq
    ON paging_chat_room (ladder, COALESCE(team_key, ''));

DROP TRIGGER IF EXISTS paging_chat_room_audit ON paging_chat_room;
CREATE TRIGGER paging_chat_room_audit
    AFTER INSERT OR UPDATE OR DELETE ON paging_chat_room
    FOR EACH ROW EXECUTE FUNCTION team_schedule_audit_row();

CREATE TABLE IF NOT EXISTS paging_chat_secret (
    room_id     UUID PRIMARY KEY REFERENCES paging_chat_room(id) ON DELETE CASCADE,
    -- Which key sealed it (CASE_ALERTS_SECRET_KEY_ID). A row sealed by
    -- another key -- a database copied from another environment -- cannot be
    -- opened and is treated as no URL.
    key_id      VARCHAR(32) NOT NULL,
    nonce       BYTEA       NOT NULL,
    ciphertext  BYTEA       NOT NULL,
    set_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    set_by      VARCHAR(255) NOT NULL
);

COMMENT ON TABLE paging_chat_secret IS
    'Encrypted webhook URL of a paging_chat_room. Never audited, never returned to a browser.';

CREATE TABLE IF NOT EXISTS paging_group_route (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_on  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_on  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  VARCHAR(255) NOT NULL,
    updated_by  VARCHAR(255) NOT NULL,
    ladder      VARCHAR(8)   NOT NULL,
    -- As the case or incident carries it, lower-cased: a CRE case's team
    -- name, an SRE incident's assignment group.
    group_name  VARCHAR(255) NOT NULL,
    team_key    VARCHAR(64)  NULL REFERENCES team(key) ON UPDATE CASCADE ON DELETE CASCADE,
    -- SRE only: a group with no team of its own climbs this rota.
    rota_code   VARCHAR(16)  NULL REFERENCES team_schedule_rota(code) ON UPDATE CASCADE,
    enabled     BOOLEAN      NOT NULL DEFAULT TRUE,
    CONSTRAINT paging_group_route_ladder_check
        CHECK (ladder IN ('CRE', 'SRE')),
    CONSTRAINT paging_group_route_group_name_check
        CHECK (group_name = lower(btrim(group_name)) AND group_name <> ''),
    CONSTRAINT paging_group_route_one_target_check
        CHECK ((team_key IS NULL) <> (rota_code IS NULL)),
    CONSTRAINT paging_group_route_rota_sre_check
        CHECK (rota_code IS NULL OR ladder = 'SRE'),
    CONSTRAINT paging_group_route_group_uq UNIQUE (ladder, group_name)
);

COMMENT ON TABLE paging_group_route IS
    'Case Alerts routing: a ServiceNow group to a team or a default rota. Used by csm-notification-service only when PAGING_CONFIG_SOURCE is set.';

DROP TRIGGER IF EXISTS paging_group_route_audit ON paging_group_route;
CREATE TRIGGER paging_group_route_audit
    AFTER INSERT OR UPDATE OR DELETE ON paging_group_route
    FOR EACH ROW EXECUTE FUNCTION team_schedule_audit_row();

CREATE TABLE IF NOT EXISTS paging_ladder_value (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_on     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_on     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by     VARCHAR(255) NOT NULL,
    updated_by     VARCHAR(255) NOT NULL,
    ladder         VARCHAR(8)   NOT NULL,
    -- The file's key under cre: or sre:, as it spells it (teams.abts).
    setting_key    VARCHAR(64)  NOT NULL,
    setting_value  JSONB        NOT NULL,
    CONSTRAINT paging_ladder_value_ladder_check
        CHECK (ladder IN ('CRE', 'SRE')),
    CONSTRAINT paging_ladder_value_key_check
        CHECK (setting_key ~ '^[a-zA-Z][a-zA-Z0-9.]*$'),
    CONSTRAINT paging_ladder_value_key_uq UNIQUE (ladder, setting_key)
);

COMMENT ON TABLE paging_ladder_value IS
    'Case Alerts ladder settings (a key of the paging file and its value), set on the Team Schedule''s Alert Settings tab. Used by csm-notification-service only when PAGING_CONFIG_SOURCE is set.';

DROP TRIGGER IF EXISTS paging_ladder_value_audit ON paging_ladder_value;
CREATE TRIGGER paging_ladder_value_audit
    AFTER INSERT OR UPDATE OR DELETE ON paging_ladder_value
    FOR EACH ROW EXECUTE FUNCTION team_schedule_audit_row();

CREATE TABLE IF NOT EXISTS paging_config_revision (
    id                SMALLINT PRIMARY KEY DEFAULT 1,
    revision          BIGINT      NOT NULL DEFAULT 0,
    changed_at        TIMESTAMPTZ NULL,
    -- What csm-notification-service reported at its last start: the revision
    -- it loaded, in which mode, and a summary of what it used (names and
    -- sources only, never a URL).
    applied_revision  BIGINT      NULL,
    applied_at        TIMESTAMPTZ NULL,
    applied_mode      VARCHAR(10) NULL,
    applied_by        VARCHAR(255) NULL,
    applied_summary   JSONB       NULL,
    CONSTRAINT paging_config_revision_single_row_check CHECK (id = 1),
    CONSTRAINT paging_config_revision_mode_check
        CHECK (applied_mode IS NULL OR applied_mode IN ('off', 'shadow', 'db'))
);

INSERT INTO paging_config_revision (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- Any change to a room, its URL, a route or a ladder value moves the revision
-- on, including one made outside the application, so "saved" and "running"
-- can be told apart on the tab. Row triggers, so a statement that changes
-- nothing moves nothing; and on an update only the columns the ladders read
-- count -- a room's test result is not a change of what is used.
CREATE OR REPLACE FUNCTION paging_config_bump() RETURNS TRIGGER AS $$
BEGIN
    UPDATE paging_config_revision
       SET revision = revision + 1, changed_at = now()
     WHERE id = 1;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS paging_chat_room_bump ON paging_chat_room;
CREATE TRIGGER paging_chat_room_bump
    AFTER INSERT OR DELETE ON paging_chat_room
    FOR EACH ROW EXECUTE FUNCTION paging_config_bump();

DROP TRIGGER IF EXISTS paging_chat_room_bump_update ON paging_chat_room;
CREATE TRIGGER paging_chat_room_bump_update
    AFTER UPDATE ON paging_chat_room
    FOR EACH ROW
    WHEN ((OLD.ladder, OLD.team_key, OLD.label, OLD.enabled, OLD.teamless, OLD.channel)
          IS DISTINCT FROM (NEW.ladder, NEW.team_key, NEW.label, NEW.enabled, NEW.teamless, NEW.channel))
    EXECUTE FUNCTION paging_config_bump();

DROP TRIGGER IF EXISTS paging_chat_secret_bump ON paging_chat_secret;
CREATE TRIGGER paging_chat_secret_bump
    AFTER INSERT OR UPDATE OR DELETE ON paging_chat_secret
    FOR EACH ROW EXECUTE FUNCTION paging_config_bump();

DROP TRIGGER IF EXISTS paging_group_route_bump ON paging_group_route;
CREATE TRIGGER paging_group_route_bump
    AFTER INSERT OR DELETE ON paging_group_route
    FOR EACH ROW EXECUTE FUNCTION paging_config_bump();

DROP TRIGGER IF EXISTS paging_group_route_bump_update ON paging_group_route;
CREATE TRIGGER paging_group_route_bump_update
    AFTER UPDATE ON paging_group_route
    FOR EACH ROW
    WHEN ((OLD.ladder, OLD.group_name, OLD.team_key, OLD.rota_code, OLD.enabled)
          IS DISTINCT FROM (NEW.ladder, NEW.group_name, NEW.team_key, NEW.rota_code, NEW.enabled))
    EXECUTE FUNCTION paging_config_bump();

DROP TRIGGER IF EXISTS paging_ladder_value_bump ON paging_ladder_value;
CREATE TRIGGER paging_ladder_value_bump
    AFTER INSERT OR DELETE ON paging_ladder_value
    FOR EACH ROW EXECUTE FUNCTION paging_config_bump();

DROP TRIGGER IF EXISTS paging_ladder_value_bump_update ON paging_ladder_value;
CREATE TRIGGER paging_ladder_value_bump_update
    AFTER UPDATE ON paging_ladder_value
    FOR EACH ROW
    WHEN ((OLD.ladder, OLD.setting_key, OLD.setting_value)
          IS DISTINCT FROM (NEW.ladder, NEW.setting_key, NEW.setting_value))
    EXECUTE FUNCTION paging_config_bump();

COMMIT;

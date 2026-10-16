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

-- Case Alerts (0233): SME teams' chat rooms, and webhook URLs stored as
-- saved, so saving and using a room needs no Choreo setting.
--
-- 1. SME team rooms. Each SME team's Google Chat space can be saved on the
--    Alert Settings tab, as each SRE team's already can, instead of a
--    GOOGLE_CHAT_SPACES entry in Choreo. Only the room: SME has no shared
--    room, teamless or channel here (a check below, and entity-service).
-- 2. Plain URLs. paging_chat_secret.webhook_url holds the URL as saved, so no
--    CASE_ALERTS_SECRET_KEY is needed. The table is still never audited or
--    returned to a browser, but anyone who can read the database (or a copy
--    of it) can read the URLs and post into those spaces: an accepted risk,
--    the database being internal. A row sealed before this (key_id, nonce,
--    ciphertext) is kept and opens only where its key is set; saving the URL
--    again replaces it with the plain one.
--
-- Nothing else changes: no row is added or rewritten. Additive and
-- re-runnable: each widened check is dropped by name and added again, so a
-- re-run lands on the same checks. One transaction with a short lock
-- timeout, as 0233.

BEGIN;
SET LOCAL lock_timeout = '5s';

ALTER TABLE paging_chat_room DROP CONSTRAINT IF EXISTS paging_chat_room_ladder_check;
ALTER TABLE paging_chat_room ADD CONSTRAINT paging_chat_room_ladder_check
    CHECK (ladder IN ('CRE', 'SRE', 'SME'));

ALTER TABLE paging_chat_room DROP CONSTRAINT IF EXISTS paging_chat_room_team_rooms_sre_check;
ALTER TABLE paging_chat_room ADD CONSTRAINT paging_chat_room_team_rooms_sre_check
    CHECK (team_key IS NULL OR ladder IN ('SRE', 'SME'));

-- An SME room is always a team's: SME has no shared room here.
ALTER TABLE paging_chat_room DROP CONSTRAINT IF EXISTS paging_chat_room_sme_team_check;
ALTER TABLE paging_chat_room ADD CONSTRAINT paging_chat_room_sme_team_check
    CHECK (ladder <> 'SME' OR (team_key IS NOT NULL AND NOT teamless AND channel IS NULL));

ALTER TABLE paging_chat_secret ADD COLUMN IF NOT EXISTS webhook_url TEXT NULL;
ALTER TABLE paging_chat_secret ALTER COLUMN key_id DROP NOT NULL;
ALTER TABLE paging_chat_secret ALTER COLUMN nonce DROP NOT NULL;
ALTER TABLE paging_chat_secret ALTER COLUMN ciphertext DROP NOT NULL;
-- Each row holds a URL one way or the other.
ALTER TABLE paging_chat_secret DROP CONSTRAINT IF EXISTS paging_chat_secret_url_check;
ALTER TABLE paging_chat_secret ADD CONSTRAINT paging_chat_secret_url_check
    CHECK (COALESCE(btrim(webhook_url), '') <> '' OR (key_id IS NOT NULL AND nonce IS NOT NULL AND ciphertext IS NOT NULL));

COMMENT ON TABLE paging_chat_secret IS
    'Webhook URL of a paging_chat_room: as saved (webhook_url), or sealed before 0237. Never audited, never returned to a browser.';

COMMIT;

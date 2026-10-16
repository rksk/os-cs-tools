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

-- Team Schedule: U2 and B-Central's SME rotations run Day 06:00-18:00 and
-- Night 18:00-06:00 (LK), not the 10:00-22:00 / 22:00-10:00 that 0217 gave
-- them. For all time: those were never their hours, so every turn on these
-- windows, past and future, reads the new ones.
--
-- Data only: no table, column, type or constraint is created or changed.
--
--   * the four windows' hours are corrected in place (same codes, zones,
--     labels and colours). A used window's hours are frozen by the used-shift
--     guard (0153) unless a migration corrects the catalogue and recomputes
--     the affected turns in the same transaction, which this one does -- as
--     0154 did -- and so says team_schedule.allow_shift_rewrite;
--   * every turn on them gets starts_at / ends_at resolved again from the new
--     hours (the instants the day view, the roster and Case Paging read);
--   * the no-overlap rule is checked row by row, and the same engineers hold
--     day and night, so a day moved to 06:00 overlaps the night ahead of it
--     until that night moves too. Every turn is first parked a minute apart
--     far ahead, where nothing else is, then given its new hours, which abut;
--   * a turn whose new hours would overlap the same engineer's turn on another
--     rota keeps its old instants and is named in a NOTICE, rather than
--     failing the whole migration; a lead re-marks it.
--
-- Times are minutes from midnight in the windows' authoring time zone
-- (Asia/Colombo); an end past 1440 runs into the next day.
--
-- Re-runnable: a window already on the new hours is left alone, and only
-- turns whose instants differ from their window's hours are touched, so a
-- second run changes nothing. One transaction with a short lock timeout, as
-- the other schedule migrations.

BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL team_schedule.allow_shift_rewrite = 'on';

-- No turn may be written while this runs: a write resolves its instants from
-- the windows' committed hours, so a turn added after the snapshot below would
-- keep the old hours for good. Reads go on; writes wait for the commit (and a
-- writer already mid-transaction makes this fail on the lock timeout, safe to
-- re-run).
LOCK TABLE team_schedule_assignment IN SHARE ROW EXCLUSIVE MODE;

-- ── The windows ───────────────────────────────────────────────────────────
UPDATE team_schedule_shift s
   SET start_minute = v.start_minute, end_minute = v.end_minute,
       updated_on = NOW(), updated_by = 'migration'
  FROM (VALUES
    ('SME_U2_DAY',     360, 1080),
    ('SME_U2_NIGHT',  1080, 1800),
    ('SME_BCN_DAY',    360, 1080),
    ('SME_BCN_NIGHT', 1080, 1800)
  ) AS v(code, start_minute, end_minute)
 WHERE s.code = v.code
   AND (s.start_minute, s.end_minute) IS DISTINCT FROM (v.start_minute, v.end_minute);

-- ── Their turns: instants resolved again from the new hours ───────────────
CREATE TEMP TABLE _sme_move ON COMMIT DROP AS
SELECT a.id, a.user_id,
       (a.rota_date::timestamp + make_interval(mins => s.start_minute)) AT TIME ZONE s.authoring_time_zone AS new_starts,
       (a.rota_date::timestamp + make_interval(mins => s.end_minute))   AT TIME ZONE s.authoring_time_zone AS new_ends
  FROM team_schedule_assignment a
  JOIN team_schedule_shift s ON s.id = a.shift_id
 WHERE s.code IN ('SME_U2_DAY', 'SME_U2_NIGHT', 'SME_BCN_DAY', 'SME_BCN_NIGHT');
DELETE FROM _sme_move m
 USING team_schedule_assignment a
 WHERE a.id = m.id AND a.starts_at = m.new_starts AND a.ends_at = m.new_ends;

-- A turn that would overlap a turn of the same engineer staying where it is
-- stays too -- and may then be what another overlaps -- so this repeats until
-- no move overlaps anything that stays.
CREATE TEMP TABLE _sme_clash (id UUID PRIMARY KEY) ON COMMIT DROP;

DO $$
DECLARE n INT; r RECORD;
BEGIN
    LOOP
        WITH kept AS (
            DELETE FROM _sme_move m
             WHERE EXISTS (
                SELECT 1 FROM team_schedule_assignment b
                 WHERE b.user_id = m.user_id AND b.is_rotation AND b.id <> m.id
                   AND NOT EXISTS (SELECT 1 FROM _sme_move o WHERE o.id = b.id)
                   AND tstzrange(b.starts_at, b.ends_at, '[)') && tstzrange(m.new_starts, m.new_ends, '[)'))
            RETURNING m.id)
        INSERT INTO _sme_clash SELECT id FROM kept;
        GET DIAGNOSTICS n = ROW_COUNT;
        EXIT WHEN n = 0;
    END LOOP;
    FOR r IN
        SELECT a.id, a.user_id, a.rota_date, s.code
          FROM _sme_clash c
          JOIN team_schedule_assignment a ON a.id = c.id
          JOIN team_schedule_shift s ON s.id = a.shift_id
         ORDER BY a.rota_date, s.code
    LOOP
        RAISE NOTICE '0236: assignment % (user %) on % (%) keeps its old instants: the new hours overlap another of their turns', r.id, r.user_id, r.rota_date, r.code;
    END LOOP;
END
$$;

UPDATE team_schedule_assignment a
   SET starts_at = timestamptz '2999-01-01 00:00+00' + m.rn * interval '2 minutes',
       ends_at   = timestamptz '2999-01-01 00:01+00' + m.rn * interval '2 minutes'
  FROM (SELECT id, row_number() OVER (ORDER BY id) AS rn FROM _sme_move) m
 WHERE a.id = m.id;

UPDATE team_schedule_assignment a
   SET starts_at = m.new_starts, ends_at = m.new_ends,
       updated_on = NOW(), updated_by = 'migration'
  FROM _sme_move m
 WHERE a.id = m.id;

COMMIT;

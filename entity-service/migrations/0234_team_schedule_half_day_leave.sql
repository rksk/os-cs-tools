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

-- Team Schedule: half-day leave, a morning and an evening tag, under Leave.
--
-- Data only, no schema change. Built-in like the others (created_by
-- 'migration'), so neither is a custom tag a lead could delete. Sorted right
-- after Annual/Casual leave (10), which is where somebody marking leave looks.
--
-- An absence is a whole rota day: OnDutyAt leaves anybody with one off the
-- on-duty list for that day, so somebody on half-day leave is not paged that
-- day either, whichever half they work. That is the safe side: never page
-- somebody who may be on leave. Paging by the half they are at work would be
-- a change to OnDutyAt, not to this catalogue.
--
-- Safe to re-run: ON CONFLICT DO NOTHING.
INSERT INTO team_schedule_absence_kind
    (code, short_code, label, bucket, colour_token, sort_order, is_active, family, created_by, updated_by)
VALUES
    ('HALF_DAY_LEAVE_MORNING', 'HD-AM', 'Half day leave — morning', 'LEAVE', 'HDM', 11, TRUE, NULL, 'migration', 'migration'),
    ('HALF_DAY_LEAVE_EVENING', 'HD-PM', 'Half day leave — evening', 'LEAVE', 'HDE', 12, TRUE, NULL, 'migration', 'migration')
ON CONFLICT (code) DO NOTHING;

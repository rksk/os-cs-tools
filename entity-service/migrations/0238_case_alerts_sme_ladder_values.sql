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

-- Case Alerts (0233): SME ladder values. The SME page's timing, teams,
-- on-leave rule and closing message can be saved on the Alert Settings tab,
-- as CRE's and SRE's already can, instead of the paging file's sme: section.
-- What decides whether it runs and what it may cost (enabled, channel,
-- safety) stays in the file.
--
-- paging_ladder_value takes ladder SME. Which keys an SME value may be is
-- entity-service's to say. Nothing else changes: no row is added.
--
-- Additive and re-runnable: the widened check is dropped by name and added
-- again. One transaction with a short lock timeout, as 0233.

BEGIN;
SET LOCAL lock_timeout = '5s';

ALTER TABLE paging_ladder_value DROP CONSTRAINT IF EXISTS paging_ladder_value_ladder_check;
ALTER TABLE paging_ladder_value ADD CONSTRAINT paging_ladder_value_ladder_check
    CHECK (ladder IN ('CRE', 'SRE', 'SME'));

COMMIT;

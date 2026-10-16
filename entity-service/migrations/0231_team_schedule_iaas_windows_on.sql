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

-- Team Schedule: switch on IaaS SRE's two windows.
--
-- 0200 seeded SRE_IAAS_DAY and SRE_IAAS_NIGHT inactive, so a portal from
-- before rotas -- which read every zoned SRE window as part of SaaS -- never
-- saw them during the deploy. Switching them on was left as a manual step
-- once the rota-aware webapp was live. That webapp has been live since, the
-- IaaS team carries its type (the IaaS roster is on screen), but the step
-- was never run: the catalogue serves only active windows, so an IaaS lead
-- opening a roster cell got leave and allocations and an empty Rota side,
-- with no L1-L3 to pick.
--
-- Safe now: every reader of SRE windows scopes them to a rota -- the
-- webapp by zone.rotaCode, Case Paging and the notification service by the
-- incident's rota, the SaaS generator by its own window codes -- so these
-- two never reach SaaS's day, roster or ladder.
--
-- Data only. is_active is not a column the used-shift guard (0153) freezes.
-- Safe to re-run; a deployment that already ran the step is left alone.
BEGIN;
SET LOCAL lock_timeout = '5s';

UPDATE team_schedule_shift
   SET is_active = TRUE, updated_on = NOW(), updated_by = 'migration'
 WHERE code IN ('SRE_IAAS_DAY', 'SRE_IAAS_NIGHT')
   AND NOT is_active;

COMMIT;

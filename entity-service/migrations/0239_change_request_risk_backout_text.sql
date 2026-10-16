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

-- change_request: risk_impact_analysis and backout_plan become TEXT.
--
-- Both were created as VARCHAR(4000) in 0043, while every other long-text column on the table
-- (justification, implementation_plan, test_plan, ...) is TEXT. A change request whose risk
-- impact analysis or backout plan is longer than 4000 characters, as can happen when one is
-- imported from the backing data source or typed in with a long runbook, was refused with
-- "value too long for type character varying(4000)" (SQLSTATE 22001). TEXT matches the sibling
-- columns and removes the cap; nothing is truncated.
--
-- No view, policy or index references either column, so the type change has no dependants.
-- Widening VARCHAR(n) to TEXT is a catalog-only change in PostgreSQL (no table rewrite).
--
-- Re-runnable: setting a column to the type it already has is a no-op. One transaction with a
-- short lock timeout, as 0238.

BEGIN;
SET LOCAL lock_timeout = '5s';

ALTER TABLE change_request
    ALTER COLUMN risk_impact_analysis TYPE TEXT,
    ALTER COLUMN backout_plan TYPE TEXT;

COMMIT;

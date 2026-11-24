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

-- Reference rows migration 0130 expects to find (supplied by a sync in a real
-- environment). Applied right after migration 0031 creates project_type.
INSERT INTO project_type (id, created_on, updated_on, created_by, updated_by, name, description, is_active) VALUES
  ('00000000-0000-0000-0000-0000000000a1', now(), now(), 'seed', 'seed', 'Managed Cloud Subscription', 'Baseline project type', TRUE),
  ('00000000-0000-0000-0000-0000000000a2', now(), now(), 'seed', 'seed', 'Evaluation Subscription', 'Baseline project type', TRUE),
  ('00000000-0000-0000-0000-0000000000a3', now(), now(), 'seed', 'seed', 'Subscription', 'Baseline project type', TRUE),
  ('00000000-0000-0000-0000-0000000000a4', now(), now(), 'seed', 'seed', 'Cloud Support', 'Baseline project type', TRUE),
  ('00000000-0000-0000-0000-0000000000a5', now(), now(), 'seed', 'seed', 'Cloud Evaluation Support', 'Baseline project type', TRUE),
  ('00000000-0000-0000-0000-0000000000a6', now(), now(), 'seed', 'seed', 'Development Support', 'Baseline project type', TRUE),
  ('00000000-0000-0000-0000-0000000000a7', now(), now(), 'seed', 'seed', 'Professional Services', 'Baseline project type', TRUE)
ON CONFLICT (name) DO NOTHING;

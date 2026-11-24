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

-- Fictional seed for the black-box API baseline. Every name, email and id
-- below is made up. Applied once, as the database superuser, after the tree's
-- own migrations. Fixed ids so the scenarios (and the results) are stable.
--
-- Roles: names matter (the user_type trigger treats 'internal' as INTERNAL and
-- 'customer' as EXTERNAL).
BEGIN;

INSERT INTO role (id, created_on, updated_on, name, description) VALUES
  ('00000000-0000-0000-0000-000000000101', now(), now(), 'internal', 'Internal engineer'),
  ('00000000-0000-0000-0000-000000000102', now(), now(), 'customer', 'External customer contact')
ON CONFLICT (id) DO NOTHING;

INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, name, first_name, last_name, email, is_active, is_system_user) VALUES
  ('00000000-0000-0000-0000-000000000001', now(), now(), 'seed', 'seed', 'jane.doe@example.com',        'Jane Doe',        'Jane',  'Doe',        'jane.doe@example.com',        true,  false),
  ('00000000-0000-0000-0000-000000000011', now(), now(), 'seed', 'seed', 'alice.perera@example.com',    'Alice Perera',    'Alice', 'Perera',     'alice.perera@example.com',    true,  false),
  ('00000000-0000-0000-0000-000000000012', now(), now(), 'seed', 'seed', 'bob.fernando@example.com',    'Bob Fernando',    'Bob',   'Fernando',   'bob.fernando@example.com',    true,  false),
  ('00000000-0000-0000-0000-000000000013', now(), now(), 'seed', 'seed', 'carol.silva@example.com',     'Carol Silva',     'Carol', 'Silva',      'carol.silva@example.com',     true,  false),
  ('00000000-0000-0000-0000-000000000031', now(), now(), 'seed', 'seed', 'dormant.user@example.com',    'Dormant User',    'Dormant','User',      'dormant.user@example.com',    false, false),
  ('00000000-0000-0000-0000-000000000021', now(), now(), 'seed', 'seed', 'dave.mendis@example.net',     'Dave Mendis',     'Dave',  'Mendis',     'dave.mendis@example.net',     true,  false),
  ('00000000-0000-0000-0000-000000000022', now(), now(), 'seed', 'seed', 'erin.jayawardena@example.net','Erin Jayawardena','Erin', 'Jayawardena','erin.jayawardena@example.net',true,  false),
  ('00000000-0000-0000-0000-000000000003', now(), now(), 'seed', 'seed', 'sam.other@othercorp.example', 'Sam Other',       'Sam',   'Other',      'sam.other@othercorp.example', true,  false)
ON CONFLICT (id) DO NOTHING;

-- No created_by on purpose: some seeds delete 'seed' internal grants.
INSERT INTO user_role (id, created_on, updated_on, user_id, role_id) VALUES
  ('00000000-0000-0000-0000-000000000201', now(), now(), '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000101'),
  ('00000000-0000-0000-0000-000000000211', now(), now(), '00000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000101'),
  ('00000000-0000-0000-0000-000000000212', now(), now(), '00000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000101'),
  ('00000000-0000-0000-0000-000000000213', now(), now(), '00000000-0000-0000-0000-000000000013', '00000000-0000-0000-0000-000000000101'),
  ('00000000-0000-0000-0000-000000000231', now(), now(), '00000000-0000-0000-0000-000000000031', '00000000-0000-0000-0000-000000000101'),
  ('00000000-0000-0000-0000-000000000221', now(), now(), '00000000-0000-0000-0000-000000000021', '00000000-0000-0000-0000-000000000102'),
  ('00000000-0000-0000-0000-000000000222', now(), now(), '00000000-0000-0000-0000-000000000022', '00000000-0000-0000-0000-000000000102'),
  ('00000000-0000-0000-0000-000000000203', now(), now(), '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000102')
ON CONFLICT (id) DO NOTHING;

INSERT INTO project_type (id, created_on, updated_on, created_by, updated_by, name, description, is_active) VALUES
  ('00000000-0000-0000-0000-0000000000a3', now(), now(), 'seed', 'seed', 'Subscription', 'Baseline project type', TRUE)
ON CONFLICT (id) DO NOTHING;
UPDATE project_type SET has_change_request_read_access = TRUE, has_service_request_read_access = TRUE
WHERE id = '00000000-0000-0000-0000-0000000000a3';

INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id, customer_success_manager_id, country, city, life_cycle) VALUES
  ('00000000-0000-0000-0000-000000000301', now(), now(), 'seed', 'seed', 'Example Corp', 'ACC-0001', 'SF-0001', '00000000-0000-0000-0000-000000000001', 'United States', 'Springfield', 'Customer'),
  ('00000000-0000-0000-0000-000000000302', now(), now(), 'seed', 'seed', 'Other Corp',   'ACC-0002', 'SF-0002', '00000000-0000-0000-0000-000000000001', 'United Kingdom', 'Shelbyville', 'Customer')
ON CONFLICT (id) DO NOTHING;

INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, name, account_id, is_active, project_type_id) VALUES
  ('00000000-0000-0000-0000-000000000401', now(), now(), 'seed', 'seed', 'EXCORP-PROJ-1',    'SF-PROJ-0001', 'Example Corp Production', '00000000-0000-0000-0000-000000000301', true, '00000000-0000-0000-0000-0000000000a3'),
  ('00000000-0000-0000-0000-000000000402', now(), now(), 'seed', 'seed', 'OTHERCORP-PROJ-1', 'SF-PROJ-0002', 'Other Corp Production',   '00000000-0000-0000-0000-000000000302', true, '00000000-0000-0000-0000-0000000000a3')
ON CONFLICT (id) DO NOTHING;

INSERT INTO deployment (id, created_on, updated_on, created_by, updated_by, number, name, type, is_active, project_id) VALUES
  ('00000000-0000-0000-0000-000000000501', now(), now(), 'seed', 'seed', 'DEP-0001', 'Production',  'PRIMARY_PRODUCTION', true, '00000000-0000-0000-0000-000000000401'),
  ('00000000-0000-0000-0000-000000000502', now(), now(), 'seed', 'seed', 'DEP-0002', 'Staging',     'STAGING',            true, '00000000-0000-0000-0000-000000000401'),
  ('00000000-0000-0000-0000-000000000541', now(), now(), 'seed', 'seed', 'DEP-0010', 'Other Prod',  'PRIMARY_PRODUCTION', true, '00000000-0000-0000-0000-000000000402')
ON CONFLICT (id) DO NOTHING;

INSERT INTO product (id, created_on, updated_on, created_by, updated_by, manufacturer, category, name) VALUES
  ('00000000-0000-0000-0000-000000000511', now(), now(), 'seed', 'seed', 'WSO2', 'SOFTWARE', 'Example Gateway'),
  ('00000000-0000-0000-0000-000000000512', now(), now(), 'seed', 'seed', 'WSO2', 'SOFTWARE', 'Example Identity')
ON CONFLICT DO NOTHING;

INSERT INTO product_version (id, created_on, updated_on, created_by, updated_by, version, product_id, current_support_status, release_date) VALUES
  ('00000000-0000-0000-0000-000000000521', now(), now(), 'seed', 'seed', '4.3.0', '00000000-0000-0000-0000-000000000511', 'AVAILABLE', '2024-06-01'),
  ('00000000-0000-0000-0000-000000000522', now(), now(), 'seed', 'seed', '4.4.0', '00000000-0000-0000-0000-000000000511', 'AVAILABLE', '2025-02-01'),
  ('00000000-0000-0000-0000-000000000523', now(), now(), 'seed', 'seed', '7.0.0', '00000000-0000-0000-0000-000000000512', 'AVAILABLE', '2024-03-01')
ON CONFLICT DO NOTHING;

INSERT INTO deployed_product (id, created_on, updated_on, created_by, updated_by, number, name, active, life_cycle_stage_status, life_cycle_stage, core_count, project_id, deployment_id, product_id, version_id, product_category) VALUES
  ('00000000-0000-0000-0000-000000000531', now(), now(), 'seed', 'seed', 'DP-0001', 'Example Gateway 4.3.0',  true, 'AVAILABLE', 'OPERATIONAL', 8, '00000000-0000-0000-0000-000000000401', '00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000511', '00000000-0000-0000-0000-000000000521', 'PDP'),
  ('00000000-0000-0000-0000-000000000532', now(), now(), 'seed', 'seed', 'DP-0002', 'Example Identity 7.0.0', true, 'AVAILABLE', 'OPERATIONAL', 4, '00000000-0000-0000-0000-000000000401', '00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000512', '00000000-0000-0000-0000-000000000523', 'PDP'),
  ('00000000-0000-0000-0000-000000000533', now(), now(), 'seed', 'seed', 'DP-0003', 'Example Gateway 4.4.0',  true, 'AVAILABLE', 'OPERATIONAL', 4, '00000000-0000-0000-0000-000000000401', '00000000-0000-0000-0000-000000000502', '00000000-0000-0000-0000-000000000511', '00000000-0000-0000-0000-000000000522', 'PDP'),
  ('00000000-0000-0000-0000-000000000534', now(), now(), 'seed', 'seed', 'DP-0004', 'Other Gateway 4.3.0',    true, 'AVAILABLE', 'OPERATIONAL', 2, '00000000-0000-0000-0000-000000000402', '00000000-0000-0000-0000-000000000541', '00000000-0000-0000-0000-000000000511', '00000000-0000-0000-0000-000000000521', 'PDP')
ON CONFLICT DO NOTHING;

-- Team and group (same id on both sides, as in a synced environment).
INSERT INTO team (id, created_on, updated_on, created_by, updated_by, name, type, key) VALUES
  ('00000000-0000-0000-0000-000000000901', now(), now(), 'seed', 'seed', 'Example Corp ABT', 'ABT', 'example corp abt')
ON CONFLICT (id) DO NOTHING;
INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name, is_active) VALUES
  ('00000000-0000-0000-0000-000000000901', now(), now(), 'seed', 'seed', 'Example Corp ABT', true)
ON CONFLICT (id) DO NOTHING;
UPDATE account SET cre_team_id = '00000000-0000-0000-0000-000000000901' WHERE id = '00000000-0000-0000-0000-000000000301';
INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id) VALUES
  ('00000000-0000-0000-0000-000000000902', now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000000901', '00000000-0000-0000-0000-000000000001', NULL),
  ('00000000-0000-0000-0000-000000000904', now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000000901', '00000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000901'),
  ('00000000-0000-0000-0000-000000000905', now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000000901', '00000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000901')
ON CONFLICT (id) DO NOTHING;

-- Registered customer contacts (what makes a customer-portal caller project-scoped).
INSERT INTO project_role (id, created_on, updated_on, created_by, updated_by, role) VALUES
  ('00000000-0000-0000-0000-000000001401', now(), now(), 'seed', 'seed', 'PORTAL_USER')
ON CONFLICT (role) DO NOTHING;
INSERT INTO project_group (id, created_on, updated_on, created_by, updated_by, "group") VALUES
  ('00000000-0000-0000-0000-000000001402', now(), now(), 'seed', 'seed', 'General Access')
ON CONFLICT ("group") DO NOTHING;
INSERT INTO project_group_role (id, created_on, updated_on, created_by, updated_by, project_group_id, project_role_id)
SELECT '00000000-0000-0000-0000-000000001403', now(), now(), 'seed', 'seed', pg.id, pr.id
FROM project_group pg, project_role pr WHERE pg."group" = 'General Access' AND pr.role = 'PORTAL_USER'
ON CONFLICT DO NOTHING;
INSERT INTO account_contact (id, created_on, updated_on, created_by, updated_by, is_active, user_name, account_id) VALUES
  ('00000000-0000-0000-0000-000000001414', now(), now(), 'seed', 'seed', true, 'dave.mendis@example.net',      '00000000-0000-0000-0000-000000000301'),
  ('00000000-0000-0000-0000-000000001415', now(), now(), 'seed', 'seed', true, 'erin.jayawardena@example.net', '00000000-0000-0000-0000-000000000301'),
  ('00000000-0000-0000-0000-000000001413', now(), now(), 'seed', 'seed', true, 'sam.other@othercorp.example',  '00000000-0000-0000-0000-000000000302')
ON CONFLICT (id) DO NOTHING;
INSERT INTO project_contact (id, created_on, updated_on, created_by, updated_by, email, state, account_contact_id, project_id) VALUES
  ('00000000-0000-0000-0000-000000001424', now(), now(), 'seed', 'seed', 'dave.mendis@example.net',      'REGISTERED', '00000000-0000-0000-0000-000000001414', '00000000-0000-0000-0000-000000000401'),
  ('00000000-0000-0000-0000-000000001425', now(), now(), 'seed', 'seed', 'erin.jayawardena@example.net', 'REGISTERED', '00000000-0000-0000-0000-000000001415', '00000000-0000-0000-0000-000000000401'),
  ('00000000-0000-0000-0000-000000001423', now(), now(), 'seed', 'seed', 'sam.other@othercorp.example',  'REGISTERED', '00000000-0000-0000-0000-000000001413', '00000000-0000-0000-0000-000000000402')
ON CONFLICT (id) DO NOTHING;
INSERT INTO project_contact_group (id, created_on, updated_on, created_by, updated_by, project_contact_id, project_group_id)
SELECT c.id, now(), now(), 'seed', 'seed', c.contact, pg.id
FROM (VALUES
  ('00000000-0000-0000-0000-000000001433'::uuid, '00000000-0000-0000-0000-000000001423'::uuid),
  ('00000000-0000-0000-0000-000000001434'::uuid, '00000000-0000-0000-0000-000000001424'::uuid),
  ('00000000-0000-0000-0000-000000001435'::uuid, '00000000-0000-0000-0000-000000001425'::uuid)
) AS c(id, contact), project_group pg WHERE pg."group" = 'General Access'
ON CONFLICT (id) DO NOTHING;

-- Three cases already in the database, for reads, filters and pagination.
-- work_state is NULL: an OPEN case has not been started.
INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, account_id, project_id, deployment_id, opened_by_user_id, assigned_to_id, description) VALUES
  ('00000000-0000-0000-0000-000000000601', now() - interval '3 days', now() - interval '3 days', 'seed', 'seed', 'CASE-0001', 'CASE-0001', 'Seeded case one (S3 question)',  'CASE', '00000000-0000-0000-0000-000000000301', '00000000-0000-0000-0000-000000000401', '00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000021', '00000000-0000-0000-0000-000000000001', 'Fictional description one.'),
  ('00000000-0000-0000-0000-000000000602', now() - interval '2 days', now() - interval '2 days', 'seed', 'seed', 'CASE-0002', 'CASE-0002', 'Seeded case two (S1 outage)',    'CASE', '00000000-0000-0000-0000-000000000301', '00000000-0000-0000-0000-000000000401', '00000000-0000-0000-0000-000000000501', '00000000-0000-0000-0000-000000000021', '00000000-0000-0000-0000-000000000011', 'Fictional description two.'),
  ('00000000-0000-0000-0000-000000000603', now() - interval '1 days', now() - interval '1 days', 'seed', 'seed', 'CASE-0003', 'CASE-0003', 'Seeded case three (other corp)', 'CASE', '00000000-0000-0000-0000-000000000302', '00000000-0000-0000-0000-000000000402', '00000000-0000-0000-0000-000000000541', '00000000-0000-0000-0000-000000000003', NULL,                                   'Fictional description three.')
ON CONFLICT (id) DO NOTHING;
INSERT INTO "case" (id, severity, issue_type, state, current_escalation_level, is_escalated, work_state) VALUES
  ('00000000-0000-0000-0000-000000000601', 'S3', 'QUESTION',      'OPEN',             'EL0', false, NULL),
  ('00000000-0000-0000-0000-000000000602', 'S1', 'TOTAL_OUTAGE',  'WORK_IN_PROGRESS', 'EL0', false, NULL),
  ('00000000-0000-0000-0000-000000000603', 'S4', 'QUESTION',      'AWAITING_INFO',    'EL0', false, NULL)
ON CONFLICT (id) DO NOTHING;
INSERT INTO comment (id, created_on, created_by, type, work_item_id, content) VALUES
  ('00000000-0000-0000-0000-000000000701', now(), 'jane.doe@example.com', 'COMMENT', '00000000-0000-0000-0000-000000000601', 'Seeded comment on case one.')
ON CONFLICT (id) DO NOTHING;
INSERT INTO time_card (id, created_on, updated_on, created_by, updated_by, case_id, customer_project_id, user_id, work_date, is_billable, state, analyzing_minutes, work_log_comment) VALUES
  ('00000000-0000-0000-0000-000000000801', now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000000601', '00000000-0000-0000-0000-000000000401', '00000000-0000-0000-0000-000000000001', CURRENT_DATE, true, 'SUBMITTED', 30, 'Seeded time card.')
ON CONFLICT (id) DO NOTHING;

-- SLA policies for cases. The resolver only considers rows whose source is the externally synced one (the enum literal below); conditions use the field=value syntax.
-- A policy with no start condition is not treated as a case policy at all, so
-- clocks only register on a case when one of these matches.
INSERT INTO sla_policy (id, created_on, updated_on, created_by, updated_by, name, is_active, target, duration, schedule, source, start_condition)
VALUES
  ('00000000-0000-0000-0000-000000001501', now(), now(), 'seed', 'seed', 'BL S1 - Response',   true, 'RESPONSE',   interval '1 hour',   '24 x 7', 'SERVICENOW', 'state=OPEN^case_type=case^priority=S1'),
  ('00000000-0000-0000-0000-000000001502', now(), now(), 'seed', 'seed', 'BL S1 - Resolution', true, 'RESOLUTION', interval '48 hours', '24 x 7', 'SERVICENOW', 'state=OPEN^case_type=case^priority=S1'),
  ('00000000-0000-0000-0000-000000001503', now(), now(), 'seed', 'seed', 'BL S2 - Response',   true, 'RESPONSE',   interval '4 hours',  '24 x 7', 'SERVICENOW', 'state=OPEN^case_type=case^priority=S2'),
  ('00000000-0000-0000-0000-000000001504', now(), now(), 'seed', 'seed', 'BL S2 - Resolution', true, 'RESOLUTION', interval '72 hours', '24 x 7', 'SERVICENOW', 'state=OPEN^case_type=case^priority=S2'),
  ('00000000-0000-0000-0000-000000001505', now(), now(), 'seed', 'seed', 'BL S3 - Response',   true, 'RESPONSE',   interval '8 hours',  '24 x 7', 'SERVICENOW', 'state=OPEN^case_type=case^priority=S3'),
  ('00000000-0000-0000-0000-000000001506', now(), now(), 'seed', 'seed', 'BL S3 - Resolution', true, 'RESOLUTION', interval '120 hours', '24 x 7', 'SERVICENOW', 'state=OPEN^case_type=case^priority=S3')
ON CONFLICT (id) DO NOTHING;

-- A service for incidents, and a service-request catalog (a category with one
-- item and one mandatory question, offered for every deployed product).
INSERT INTO service (id, created_on, updated_on, created_by, updated_by, name, number) VALUES
  ('00000000-0000-0000-0000-0000000000aa', now(), now(), 'seed', 'seed', 'Baseline Service', 'SVC-0001')
ON CONFLICT (id) DO NOTHING;
INSERT INTO sr_category (id, created_on, updated_on, created_by, updated_by, name, is_active, catalog) VALUES
  ('00000000-0000-0000-0000-000000001601', now(), now(), 'seed', 'seed', 'Baseline Services', true, 'SERVICE_CATALOG')
ON CONFLICT (id) DO NOTHING;
INSERT INTO catalog_item (id, created_on, updated_on, created_by, updated_by, name) VALUES
  ('00000000-0000-0000-0000-000000001602', now(), now(), 'seed', 'seed', 'Baseline Request')
ON CONFLICT (id) DO NOTHING;
INSERT INTO catalog_item_category (id, created_on, updated_on, created_by, updated_by, catalog_item_id, sr_category_id) VALUES
  ('00000000-0000-0000-0000-000000001603', now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000001602', '00000000-0000-0000-0000-000000001601')
ON CONFLICT (id) DO NOTHING;
INSERT INTO sr_category_routing_rule (id, created_on, updated_on, created_by, updated_by, "order", catalog_item_id) VALUES
  ('00000000-0000-0000-0000-000000001604', now(), now(), 'seed', 'seed', 1, '00000000-0000-0000-0000-000000001602')
ON CONFLICT (id) DO NOTHING;
INSERT INTO catalog_variable (id, created_on, updated_on, created_by, updated_by, name, is_active, question_text, type, "order", is_mandatory, catalog_item_id, read_only, hidden) VALUES
  ('00000000-0000-0000-0000-000000001605', now(), now(), 'seed', 'seed', 'reason', true, 'Why is this needed?', 'String', 1, true, '00000000-0000-0000-0000-000000001602', false, false)
ON CONFLICT (id) DO NOTHING;

-- Change-approval group membership: a change cannot be sent for approval
-- unless the CAB group (created empty by a migration) has an eligible internal
-- member, and the "Devops Approval" fallback group has to exist for normal changes.
INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id) VALUES
  ('00000000-0000-0000-0000-000000001111', now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000000901', '00000000-0000-0000-0000-000000000011', '00000000-0000-4000-8000-00000000ca01'),
  ('00000000-0000-0000-0000-000000001112', now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000000901', '00000000-0000-0000-0000-000000000012', '00000000-0000-4000-8000-00000000ca01')
ON CONFLICT (id) DO NOTHING;
INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name, description, is_active)
SELECT '00000000-0000-4000-8000-0000000de001'::uuid, now(), now(), 'seed', 'seed', 'Devops Approval', 'Peer approval fallback group', true
WHERE NOT EXISTS (SELECT 1 FROM "group" WHERE name = 'Devops Approval')
ON CONFLICT (id) DO NOTHING;
INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
SELECT m.id, now(), now(), 'seed', 'seed', '00000000-0000-0000-0000-000000000901'::uuid, m.user_id,
       (SELECT g.id FROM "group" g WHERE g.name = 'Devops Approval' ORDER BY g.created_on, g.id LIMIT 1)
FROM (VALUES
  ('00000000-0000-0000-0000-000000001131'::uuid, '00000000-0000-0000-0000-000000000011'::uuid),
  ('00000000-0000-0000-0000-000000001132'::uuid, '00000000-0000-0000-0000-000000000012'::uuid)
) AS m(id, user_id)
ON CONFLICT (id) DO NOTHING;

COMMIT;

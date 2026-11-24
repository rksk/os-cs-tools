-- Fictional baseline rows for migration-upgrade-test.sh, loaded on top of a
-- schema at the point before the pending migrations. No anomalies: every unique
-- index a pending migration adds can be built on these rows.
--
-- Volumes are a few thousand rows on the tables the pending migrations rewrite
-- or backfill, so their durations and locks mean something: 2,200 projects,
-- 7,400 deployed products, 11,400 service requests and 1,110 user skills.
--
-- Every value is made up (Jane Doe NNN, jane.doe.NNN@example.com). Triggers are
-- switched off for the load so no outbound queue rows are produced; every
-- foreign key still points at a real parent row.

SET session_replication_role = replica;

-- People: 60 internal users.
INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, name,
                    first_name, last_name, email, is_active, is_system_user)
SELECT ('00000000-0000-4000-8000-' || lpad(to_hex(g), 12, '0'))::uuid,
       now() - interval '400 days', now() - interval '400 days', 'seed', 'seed',
       format('jane.doe.%s@example.com', lpad(g::text, 3, '0')),
       format('Jane Doe %s', lpad(g::text, 3, '0')), 'Jane', format('Doe %s', lpad(g::text, 3, '0')),
       format('jane.doe.%s@example.com', lpad(g::text, 3, '0')), true, false
FROM generate_series(1, 60) g;

-- Skill catalogue: one scale with three levels, five categories, 30 skills with
-- distinct names.
INSERT INTO skill_level_type (id, created_on, updated_on, created_by, updated_by, name)
VALUES ('00000000-0000-4000-8001-000000000001', now(), now(), 'seed', 'seed', 'Example scale');

INSERT INTO skill_level (id, created_on, updated_on, created_by, updated_by, name, value, level_type_id)
SELECT ('00000000-0000-4000-8002-' || lpad(to_hex(g), 12, '0'))::uuid, now(), now(), 'seed', 'seed',
       format('Level %s', g), g, '00000000-0000-4000-8001-000000000001'
FROM generate_series(1, 3) g;

INSERT INTO skill_category (id, created_on, updated_on, created_by, updated_by, name, code)
SELECT ('00000000-0000-4000-8003-' || lpad(to_hex(g), 12, '0'))::uuid, now(), now(), 'seed', 'seed',
       format('Example category %s', g), format('CAT%s', g)
FROM generate_series(1, 5) g;

INSERT INTO skill (id, created_on, updated_on, created_by, updated_by, name, active, level_type_id)
SELECT ('00000000-0000-4000-8004-' || lpad(to_hex(g), 12, '0'))::uuid, now(), now(), 'seed', 'seed',
       format('Example skill %s', lpad(g::text, 2, '0')), true, '00000000-0000-4000-8001-000000000001'
FROM generate_series(1, 30) g;

-- User skills: 40 users x 27 skills = 1,080 own active rows, one per
-- (user, skill). On top, rows the new partial unique index must tolerate:
-- 20 inactive copies and 10 inherited copies of existing own pairs.
INSERT INTO user_skill (created_on, updated_on, created_by, updated_by, user_id, skill_id,
                        skill_level_id, active, inherited)
SELECT now() - interval '200 days', now() - interval '200 days', 'seed', 'seed',
       ('00000000-0000-4000-8000-' || lpad(to_hex(u), 12, '0'))::uuid,
       ('00000000-0000-4000-8004-' || lpad(to_hex(s), 12, '0'))::uuid,
       ('00000000-0000-4000-8002-' || lpad(to_hex(1 + (u + s) % 3), 12, '0'))::uuid,
       true, false
FROM generate_series(1, 40) u, generate_series(1, 27) s;

INSERT INTO user_skill (created_on, updated_on, created_by, updated_by, user_id, skill_id, active, inherited)
SELECT now() - interval '300 days', now() - interval '250 days', 'seed', 'seed',
       ('00000000-0000-4000-8000-' || lpad(to_hex(u), 12, '0'))::uuid,
       '00000000-0000-4000-8004-000000000001'::uuid, false, false
FROM generate_series(1, 20) u;

INSERT INTO user_skill (created_on, updated_on, created_by, updated_by, user_id, skill_id, active, inherited)
SELECT now() - interval '100 days', now() - interval '100 days', 'seed', 'seed',
       ('00000000-0000-4000-8000-' || lpad(to_hex(u), 12, '0'))::uuid,
       '00000000-0000-4000-8004-000000000002'::uuid, true, true
FROM generate_series(21, 30) u;

-- Projects: one project type the category backfill recognises, 2,200 projects.
INSERT INTO project_type (id, created_on, updated_on, created_by, updated_by, name, is_active)
VALUES ('00000000-0000-4000-8005-000000000001', now(), now(), 'seed', 'seed', 'Subscription', true);

INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, name, project_type_id,
                     onboarding_start_on)
SELECT ('00000000-0000-4000-8006-' || lpad(to_hex(g), 12, '0'))::uuid, now(), now(), 'seed', 'seed',
       format('EXAMPLE%s', lpad(g::text, 5, '0')), format('Example project %s', g),
       '00000000-0000-4000-8005-000000000001',
       CASE WHEN g % 3 = 0 THEN date '2026-01-01' + (g % 300) END
FROM generate_series(1, 2200) g;

-- Deployed products: 7,400, spread over the projects, no category yet.
INSERT INTO deployed_product (created_on, updated_on, created_by, updated_by, number, project_id)
SELECT now(), now(), 'seed', 'seed', format('DP%s', lpad(g::text, 7, '0')),
       ('00000000-0000-4000-8006-' || lpad(to_hex(1 + g % 2200), 12, '0'))::uuid
FROM generate_series(1, 7400) g;

-- Service requests: 11,400 work items of that type, each with its subtype row.
INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type, project_id)
SELECT ('00000000-0000-4000-8007-' || lpad(to_hex(g), 12, '0'))::uuid, now(), now(), 'seed', 'seed',
       format('SR%s', lpad(g::text, 7, '0')), format('Example request %s', g), 'SERVICE_REQUEST',
       ('00000000-0000-4000-8006-' || lpad(to_hex(1 + g % 2200), 12, '0'))::uuid
FROM generate_series(1, 11400) g;

INSERT INTO service_request (id)
SELECT ('00000000-0000-4000-8007-' || lpad(to_hex(g), 12, '0'))::uuid
FROM generate_series(1, 11400) g;

SET session_replication_role = DEFAULT;

ANALYZE;

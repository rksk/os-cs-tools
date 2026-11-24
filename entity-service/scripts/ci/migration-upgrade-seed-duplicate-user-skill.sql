-- Fictional anomaly for migration-upgrade-test.sh, loaded after
-- migration-upgrade-seed-baseline.sql: 43 (user_id, skill_id) pairs that hold
-- two active, own (not inherited) user_skill rows each. A database in this state
-- cannot take a unique index on (user_id, skill_id) WHERE active AND NOT inherited
-- until the duplicates are resolved.

INSERT INTO user_skill (created_on, updated_on, created_by, updated_by, user_id, skill_id,
                        skill_level_id, active, inherited)
SELECT now() - interval '30 days', now() - interval '30 days', 'seed', 'seed',
       ('00000000-0000-4000-8000-' || lpad(to_hex(u), 12, '0'))::uuid,
       ('00000000-0000-4000-8004-' || lpad(to_hex(s), 12, '0'))::uuid,
       '00000000-0000-4000-8002-000000000002'::uuid,
       true, false
FROM (SELECT u, s FROM generate_series(1, 40) u, generate_series(1, 27) s
      ORDER BY u, s LIMIT 43) pairs;

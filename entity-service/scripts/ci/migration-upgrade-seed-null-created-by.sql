-- Fictional anomaly for migration-upgrade-test.sh, loaded after
-- migration-upgrade-seed-baseline.sql: one user row whose created_by is NULL.
-- A database in this state cannot have created_by made mandatory on "user"
-- until the row is filled in.

SET session_replication_role = replica;

INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, name, email,
                    is_active, is_system_user)
VALUES ('00000000-0000-4000-8000-0000000000ff', now(), now(), NULL, NULL,
        'john.doe@example.com', 'John Doe', 'john.doe@example.com', true, false);

SET session_replication_role = DEFAULT;

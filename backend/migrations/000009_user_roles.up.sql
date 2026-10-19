BEGIN;

ALTER TABLE users
  ADD COLUMN role varchar(20) NOT NULL DEFAULT 'student'
  CHECK (role IN ('admin', 'student'));

CREATE INDEX users_role_created_idx ON users(role, created_at DESC);

COMMIT;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Back to a unit's SQL running as the runtime role. The role stays: it is
-- cluster-level and may serve other databases.
SET LOCAL lock_timeout = '3s';

DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'ext_runtime') THEN
    ALTER DEFAULT PRIVILEGES IN SCHEMA ext REVOKE USAGE, SELECT ON SEQUENCES FROM ext_runtime;
    ALTER DEFAULT PRIVILEGES IN SCHEMA ext REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM ext_runtime;
    REVOKE USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ext FROM ext_runtime;
    REVOKE SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ext FROM ext_runtime;
    REVOKE USAGE ON SCHEMA ext FROM ext_runtime;
  END IF;
END $$;

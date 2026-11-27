-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A member's identity family, with the same query as before. PostgreSQL 16
-- plans a SQL function's body again in every statement that calls it, and one
-- row-scope read calls it at many places. PL/pgSQL keeps the body's plan for
-- the session.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION member_identity_family(member uuid) RETURNS uuid[]
    LANGUAGE plpgsql STABLE PARALLEL SAFE
    AS $$
BEGIN
  RETURN (
    WITH RECURSIVE member_identities(id) AS (
      SELECT member
      UNION
      SELECT retired.id FROM app_user retired JOIN member_identities survivor
        ON retired.merged_into_user_id = survivor.id
       WHERE retired.removed_at IS NOT NULL)
    SELECT array_agg(id) FROM member_identities);
END;
$$;

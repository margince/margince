-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A member's identity family: the member and every retired account merged
-- into it, through any chain of merges. Every row-scope clause asks this
-- question, often dozens of times in one query.
--
-- Spelled inline, each ask was its own recursive CTE, and Postgres planned
-- every copy. A function call is one node to plan, and its body is planned
-- once per connection. A scalar body with a CTE is never inlined, which is
-- the point.
SET LOCAL lock_timeout = '3s';

CREATE FUNCTION member_identity_family(member uuid) RETURNS uuid[]
    LANGUAGE sql STABLE PARALLEL SAFE
    AS $$
  WITH RECURSIVE member_identities(id) AS (
    SELECT member
    UNION
    SELECT retired.id FROM app_user retired JOIN member_identities survivor
      ON retired.merged_into_user_id = survivor.id
     WHERE retired.removed_at IS NOT NULL)
  SELECT array_agg(id) FROM member_identities
$$;

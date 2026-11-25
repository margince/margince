-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION member_identity_family(member uuid) RETURNS uuid[]
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

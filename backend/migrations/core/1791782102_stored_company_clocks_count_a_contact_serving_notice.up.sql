-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A file of its own so it runs after 1791782101 has committed and released its locks.
-- Rows lock in id order, the order every writer locks companies, so a concurrent write queues.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE
  r record;
BEGIN
  FOR r IN
    SELECT id FROM company
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_company(id)
     ORDER BY id
  LOOP
    PERFORM move_last_activity('company'::regclass, r.id);
  END LOOP;
END $$;

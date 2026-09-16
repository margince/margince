-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- Refuse to discard judgements that closed real work.
--
-- A settled row is why a task is done and why a request left the waiting queue.
-- Dropping the table would not reopen either — the task stays completed — so
-- the rollback would silently destroy the only record of WHY, and nothing
-- downstream would look wrong. An installation that genuinely wants this table
-- gone settles or clears its rows first.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM activity_request_settlement WHERE verdict <> 'unsure') THEN
  RAISE EXCEPTION 'Request settlements exist; rollback would discard why a request was closed';
 END IF;
END $$;

DROP TABLE IF EXISTS activity_request_settlement;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- This unit is a buffer: the core owns the record. An outbound attempt keeps
-- the activity it sent, not the account it went to, and an inbound message
-- keeps its body only until it lands. Who a message went to, and what it said,
-- then live on the core's activity, where erasure and export reach them.
--
-- An attempt recorded before this has no activity to name, only the account,
-- so it is deleted rather than kept with a person and no link.

DELETE FROM ext.ext_openchannel_outbound;

ALTER TABLE ext.ext_openchannel_outbound DROP COLUMN recipient;

-- The timeline entry this attempt sent, as the core handed it over.
ALTER TABLE ext.ext_openchannel_outbound ADD COLUMN activity_id uuid NOT NULL;

-- A landed message's body is the core's now.
ALTER TABLE ext.ext_openchannel_inbound ALTER COLUMN body DROP NOT NULL;

UPDATE ext.ext_openchannel_inbound
   SET body = NULL
 WHERE state IN ('ingested', 'withdrawn');

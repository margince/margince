-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A draft an agent wrote for a human waits in that human's saved drafts, and
-- says so until the human saves over it.
--
-- draft_email writes the draft for the human the agent acts for, so the row
-- belongs to that human exactly like one their composer saved. The flag is the
-- AI marking: the screen tells the human the words are an agent's before they
-- send them. A save from the composer clears it, because from then on the
-- words are the ones the human kept, and an agent may replace only a draft
-- that still carries it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE mail_draft ADD COLUMN agent_drafted boolean DEFAULT false NOT NULL;

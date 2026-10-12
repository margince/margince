-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An agent's draft names every record its words were written from, so reading
-- it re-proves the reader may still see each one and discards it when they may
-- not. Entries are {entity_type, entity_id}; a draft a human started names none.
SET LOCAL lock_timeout = '3s';

ALTER TABLE mail_draft ADD COLUMN grounding jsonb DEFAULT '[]'::jsonb NOT NULL;

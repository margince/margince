-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- consent_qualifying_event.about_deal_id names the deal a hand-recorded request
-- was about. A follow-up on a deal, and a quote on one, rest only on a request
-- about that deal, so a request about one deal does not carry another. Null
-- means the request named no deal, which every existing row reads as.
--
-- No foreign key, like source_entity_id beside it: the row is the record of
-- what was said, and a deal removed later must not take the statement with it
-- or point it at nothing else.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event
    ADD COLUMN about_deal_id uuid;

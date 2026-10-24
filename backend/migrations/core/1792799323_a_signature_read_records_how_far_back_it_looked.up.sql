-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- How many of the contact's newest eligible mails the signature pass was allowed
-- to read when it set this watermark. NULL marks a watermark the one-mail pass
-- set: the data task core/1792799323_signature_title_history reopens those for
-- contacts still without a title, and skips every row the walking pass wrote.
ALTER TABLE contact_signature_enrich_state
    ADD COLUMN history_depth smallint;

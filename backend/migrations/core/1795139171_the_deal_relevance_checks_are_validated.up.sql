-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the checks the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail:
-- the columns were created there with no default, so every row is NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_deal_relevance_check;
ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_relevance_source_check;
ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_deal_relevance_shape;

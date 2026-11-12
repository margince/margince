-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- A cleared body cannot come back, so a landed row keeps an empty one, and an
-- attempt cannot name the account it no longer holds, so the ledger empties.

UPDATE ext.ext_openchannel_inbound SET body = '\x'::bytea WHERE body IS NULL;

ALTER TABLE ext.ext_openchannel_inbound ALTER COLUMN body SET NOT NULL;

ALTER TABLE ext.ext_openchannel_inbound DROP COLUMN body_bytes;

DELETE FROM ext.ext_openchannel_outbound;

ALTER TABLE ext.ext_openchannel_outbound DROP COLUMN activity_id;

ALTER TABLE ext.ext_openchannel_outbound
    ADD COLUMN recipient text NOT NULL CHECK (length(recipient) > 0);

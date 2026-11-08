-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Every kept stop an anonymize wrote still holds its plaintext address, so the
-- older code matches it as before. A stop recorded on an anonymized contact
-- holds only a hash and cannot be written back as an address. It keeps its row
-- under a placeholder address that matches no mailbox, so the old check holds
-- and no stop is deleted.
SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS bind_requests_on_address_move ON contact_email;
DROP FUNCTION IF EXISTS bind_requests_on_address_move();

UPDATE communication_suppression
   SET address = 'kept-hash+' || address_hash || '@example.invalid'
 WHERE contact_id IS NULL AND lead_id IS NULL AND address IS NULL AND address_hash IS NOT NULL;

DROP INDEX IF EXISTS communication_suppression_live_address_hash;
ALTER TABLE contact_consent DROP COLUMN IF EXISTS recorded_for_addresses;
ALTER TABLE contact DROP COLUMN IF EXISTS anonymized_address_hashes;
ALTER TABLE communication_suppression
    DROP CONSTRAINT communication_suppression_names_a_target,
    ADD CONSTRAINT communication_suppression_names_a_target
        CHECK (contact_id IS NOT NULL OR lead_id IS NOT NULL OR address IS NOT NULL) NOT VALID,
    DROP CONSTRAINT IF EXISTS communication_suppression_recorded_through,
    DROP COLUMN IF EXISTS recorded_through,
    DROP COLUMN IF EXISTS recorded_for_addresses,
    DROP COLUMN IF EXISTS address_hash;

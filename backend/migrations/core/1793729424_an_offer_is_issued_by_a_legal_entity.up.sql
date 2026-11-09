-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An offer is issued by one of the installation's legal entities. A new offer
-- takes its author's entity; NULL, which every offer written before this file
-- carries, means the default entity. Nothing is backfilled.
--
-- The key is added NOT VALID and validated by the file beside this one, so the
-- offer table is not scanned under the ACCESS EXCLUSIVE lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer
    ADD COLUMN legal_entity_id uuid,
    ADD CONSTRAINT offer_legal_entity_id_fkey
        FOREIGN KEY (legal_entity_id) REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;

CREATE INDEX idx_offer_legal_entity ON offer (legal_entity_id) WHERE legal_entity_id IS NOT NULL;

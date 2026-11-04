-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the key back the way the file beside this one found it: present,
-- unvalidated. Postgres has no ALTER that un-validates, so it is dropped and
-- re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer
    DROP CONSTRAINT offer_legal_entity_id_fkey,
    ADD CONSTRAINT offer_legal_entity_id_fkey
        FOREIGN KEY (legal_entity_id) REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;

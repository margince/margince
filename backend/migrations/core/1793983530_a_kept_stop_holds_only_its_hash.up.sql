-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A stop on a bare address, one an anonymize kept or one a link pressed for an
-- address nobody holds, holds only the hash of that address. The migration that
-- added address_hash hashed every such stop, and this one clears its plaintext
-- address. Every reader of these rows matches the hash through
-- storekit.StopOnAddressesSQL: the send check, the refusal review, the stop
-- list, the Art. 15 export, the erasure and the dedup of a pressed link. A hard
-- bounce is a fact about a mailbox, lifted by its address, and keeps it. Every
-- cleared row still names a target through address_hash, which
-- communication_suppression_names_a_target accepts.
SET LOCAL lock_timeout = '3s';

UPDATE communication_suppression
   SET address = NULL
 WHERE contact_id IS NULL AND lead_id IS NULL AND kind <> 'hard_bounce'
   AND address_hash IS NOT NULL AND address IS NOT NULL;

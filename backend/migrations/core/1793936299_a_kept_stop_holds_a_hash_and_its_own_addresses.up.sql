-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Three facts a stop carries.
--
-- address_hash: a stop kept after its subject was anonymized holds the hash of
-- the address it covers (storekit.SuppressionHash), and the send check matches
-- it. The plaintext address stays beside it, so a worker that predates the hash
-- keeps blocking during a rolling deploy and a rollback loses nothing.
--
-- contact.anonymized_address_hashes: the hashes of an anonymized contact's
-- addresses. A stop recorded on the record after the anonymize is pinned to
-- them.
--
-- recorded_for_addresses, here and on contact_consent: the addresses of the
-- record a subject-wide stop or a withdrawal was made on, written before a
-- merge moves addresses. NULL is a row recorded on this record, which covers
-- every address it holds. An anonymize pins the row to these addresses only.
--
-- recorded_through: how the stop was first recorded. Every copy keeps it, so
-- a carried press never reads as typed by the colleague who merged.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression
    ADD COLUMN address_hash text,
    ADD COLUMN recorded_for_addresses text[],
    ADD COLUMN recorded_through text NOT NULL DEFAULT 'unknown',
    ADD CONSTRAINT communication_suppression_recorded_through
        CHECK (recorded_through IN ('colleague', 'agent', 'subject', 'machine', 'unknown')) NOT VALID,
    DROP CONSTRAINT communication_suppression_names_a_target,
    ADD CONSTRAINT communication_suppression_names_a_target
        CHECK (contact_id IS NOT NULL OR lead_id IS NOT NULL OR address IS NOT NULL
               OR address_hash IS NOT NULL) NOT VALID;

ALTER TABLE contact_consent ADD COLUMN recorded_for_addresses text[];
ALTER TABLE contact ADD COLUMN anonymized_address_hashes text[];

CREATE INDEX communication_suppression_live_address_hash ON communication_suppression (address_hash)
    WHERE address_hash IS NOT NULL AND revoked_at IS NULL;

-- How each stop was recorded, read off the first row of its carry chain. A
-- colleague typed it through the suppress door, a subject pressed a mailed
-- link, or the product wrote it. A chain whose first row is gone reads unknown,
-- which no rule treats as typed by a colleague.
WITH RECURSIVE chain AS (
    SELECT id, carried_from, captured_by, source, kind FROM communication_suppression
  UNION ALL
    SELECT chain.id, parent.carried_from, parent.captured_by, parent.source, parent.kind
      FROM chain JOIN communication_suppression parent ON parent.id = chain.carried_from
), first_row AS (
    SELECT id, captured_by, source, kind FROM chain WHERE carried_from IS NULL
)
UPDATE communication_suppression s
   SET recorded_through = CASE
       WHEN starts_with(f.source, 'carried: ') THEN 'unknown'
       WHEN f.kind = 'hard_bounce' THEN 'machine'
       WHEN f.source = 'public_link' THEN 'subject'
       WHEN f.source = 'withdrawal kept when the record was anonymized' THEN 'machine'
       WHEN (f.source = 'recorded by a contact' OR starts_with(f.source, 'recorded by a contact: '))
            AND starts_with(f.captured_by, 'human:') THEN 'colleague'
       WHEN (f.source = 'recorded by a contact' OR starts_with(f.source, 'recorded by a contact: '))
            AND starts_with(f.captured_by, 'agent:') THEN 'agent'
       ELSE 'unknown' END
  FROM first_row f
 WHERE f.id = s.id;

-- A stop an anonymize already pinned to a plaintext address gains the hash of
-- that address, and keeps the address. Such a stop names no subject. A hard
-- bounce is a fact about a mailbox and is matched by its address alone.
UPDATE communication_suppression
   SET address_hash = encode(sha256(convert_to(lower(btrim(address)), 'UTF8')), 'hex')
 WHERE contact_id IS NULL AND lead_id IS NULL AND address IS NOT NULL AND kind <> 'hard_bounce';

-- A merge moves the retiring contact's addresses with one UPDATE of
-- contact_email. Before the first address leaves, each request, subject-wide
-- stop and marketing withdrawal of either contact that is not bound yet is
-- bound to the addresses its record holds. The merge code binds them first
-- (consent's BindRequestsToAddressesTx). This binds the same rows for a process
-- that predates that code and merges during a rolling deploy.
CREATE FUNCTION bind_requests_on_address_move() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    subject uuid;
BEGIN
    FOREACH subject IN ARRAY ARRAY[OLD.contact_id, NEW.contact_id] LOOP
        UPDATE consent_qualifying_event q
           SET recorded_for_addresses = (SELECT coalesce(array_agg(ce.email ORDER BY ce.email), '{}')
                                           FROM contact_email ce
                                          WHERE ce.contact_id = subject AND ce.archived_at IS NULL)
         WHERE q.contact_id = subject AND q.recorded_for_addresses IS NULL;
        UPDATE contact_acquisition_evidence e
           SET recorded_for_addresses = (SELECT coalesce(array_agg(ce.email ORDER BY ce.email), '{}')
                                           FROM contact_email ce
                                          WHERE ce.contact_id = subject AND ce.archived_at IS NULL)
         WHERE e.contact_id = subject AND e.recorded_for_addresses IS NULL;
        UPDATE communication_suppression s
           SET recorded_for_addresses = (SELECT coalesce(array_agg(ce.email ORDER BY ce.email), '{}')
                                           FROM contact_email ce
                                          WHERE ce.contact_id = subject AND ce.archived_at IS NULL)
         WHERE s.contact_id = subject AND s.address IS NULL AND s.revoked_at IS NULL
           AND s.recorded_for_addresses IS NULL;
        UPDATE contact_consent c
           SET recorded_for_addresses = (SELECT coalesce(array_agg(ce.email ORDER BY ce.email), '{}')
                                           FROM contact_email ce
                                          WHERE ce.contact_id = subject AND ce.archived_at IS NULL)
         WHERE c.contact_id = subject AND c.state = 'withdrawn' AND c.recorded_for_addresses IS NULL;
    END LOOP;
    RETURN NEW;
END $$;

CREATE TRIGGER bind_requests_on_address_move
    BEFORE UPDATE OF contact_id ON contact_email
    FOR EACH ROW WHEN (OLD.contact_id IS DISTINCT FROM NEW.contact_id)
    EXECUTE FUNCTION bind_requests_on_address_move();

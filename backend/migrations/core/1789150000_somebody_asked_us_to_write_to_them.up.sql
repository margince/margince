-- A person asking us to write to them is a qualifying event.
--
-- Ordinary business correspondence is lawful under Art 6(1)(f) when something
-- on the record connects us to the person. The engine derives most of that
-- itself — an inbound message, an inquiry, an open deal, a meeting — and one
-- kind it cannot: an exchange that happened away from every system. `in_person`
-- is that kind today, and its name is narrower than the fact it stands for.
--
-- A CUSTOMER RINGS UP AND ASKS FOR A QUOTE. Nothing lands in a mailbox, nothing
-- appears in a calendar, and the rep who took the call is the only record that
-- it happened. The send is then refused as correspondence with somebody who
-- "has never written to you" — which is true of the inbox and false of the
-- world — and the rep has nowhere to say otherwise, because `in_person` says
-- they were in a room together and they were not.
--
-- So the kind is added rather than the meaning of `in_person` stretched. A
-- reader asking what evidence stands behind a send gets the answer in the word:
-- somebody was there, or somebody rang.
--
-- IT GRANTS NO MARKETING CONSENT and could not. §7 UWG asks for express
-- consent, and "send me a quote" is not one. What it settles is the narrower
-- question of whether an ordinary business email may be sent at all — the same
-- scope `in_person` has carried since the baseline.
--
-- HAND-RECORDED, SO IT NEEDS ITS NOTE. It joins `in_person` on the left of the
-- evidence CHECK rather than the derived group on the right: there is no source
-- row to cite, so the note IS the evidence and a row without one asserts a fact
-- nobody can check. That is also why it cannot be derived — nothing in the
-- product observes a phone call — and why the uniqueness the derived kinds get
-- from source_entity_id does not apply: two calls on two days are two events.
--
-- Both CHECKs are dropped and re-added because a CHECK cannot be altered in
-- place. The rewrite is additive in effect: every value either constraint
-- admitted before is still admitted.

-- The table is small and these are catalog-only rewrites, but the ALTERs still
-- take ACCESS EXCLUSIVE: an open transaction holding a conflicting lock would
-- stall every write to the table for as long as this is willing to queue.
-- Bounded, so a busy database fails the deploy instead of freezing consent
-- writes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event
  DROP CONSTRAINT consent_qualifying_event_kind_check;

ALTER TABLE consent_qualifying_event
  ADD CONSTRAINT consent_qualifying_event_kind_check
  CHECK (kind = ANY (ARRAY['inbound_message'::text, 'inquiry'::text,
                           'active_deal'::text, 'in_person'::text,
                           'meeting'::text, 'requested_by_subject'::text]));

-- The evidence rule gains its second hand-recorded kind. The shape is the same
-- one `in_person` has: a note and no source row.
ALTER TABLE consent_qualifying_event
  DROP CONSTRAINT consent_qualifying_event_evidence;

ALTER TABLE consent_qualifying_event
  ADD CONSTRAINT consent_qualifying_event_evidence
  CHECK (
    (kind IN ('in_person'::text, 'requested_by_subject'::text)
        AND note IS NOT NULL)
    OR (kind NOT IN ('in_person'::text, 'requested_by_subject'::text)
        AND source_entity_type IS NOT NULL
        AND source_entity_id IS NOT NULL));

-- A preference link says which address it was sent to.
--
-- The token carried only person_id, so the page it opens masked the person's
-- PRIMARY address. For a person holding more than one, a link delivered to a
-- secondary or shared address showed the first character and the full domain of
-- an address its holder was never written at — a disclosure to whoever reads
-- the shared mailbox, on a credential that stays valid for thirty days.
--
-- Nullable, because every token minted before this migration was minted without
-- one and there is no way to learn after the fact which address it went to. A
-- token with no address falls back to the primary exactly as before; the view
-- is what decides, and a NULL is the honest record of "not known" rather than a
-- guess that would read like a fact.
--
-- ON DELETE SET NULL rather than CASCADE: an address the subject erased must
-- not take the unsubscribe credential with it. Suppression is what you most
-- want still working once somebody has asked to be forgotten, and a token that
-- vanished with the address would leave the last message's link dead.
-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock on a table the
-- send path writes on every tokenized send: an open transaction holding a
-- conflicting lock would otherwise stall every one of those writes for as long
-- as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE preference_token
    ADD COLUMN person_email_id uuid REFERENCES person_email(id) ON DELETE SET NULL;

-- One live token per (person, ADDRESS) rather than per person.
--
-- The token is the credential for a pair, not for a person: reusing one minted
-- for a first address on a send to a second is what would make the page name
-- the wrong one again, one level down from the column above.
--
-- NULLS NOT DISTINCT so the pre-migration rows keep their old guarantee: two
-- live tokens for one person with no address recorded would be exactly the
-- duplicate the old index forbade.
DROP INDEX uq_preference_token_person;
CREATE UNIQUE INDEX uq_preference_token_person_address
    ON preference_token (person_id, person_email_id) NULLS NOT DISTINCT
    WHERE revoked_at IS NULL;

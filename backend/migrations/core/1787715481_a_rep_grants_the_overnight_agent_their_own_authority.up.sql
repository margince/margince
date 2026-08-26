-- Bounded: the passport ALTER below takes a lock on a table this migration did
-- not create, so an open transaction holding a conflicting one would otherwise
-- stall every passport write for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

-- Whether each rep has said yes to the overnight agent working on their behalf,
-- and which passport carries that authority.
--
-- WHY A TABLE AND NOT "does this rep have a passport".
--
-- The product asks ONCE. A rep who declined has no passport, and so does a rep
-- who has never been asked — the two are indistinguishable from the passport
-- table alone, and a product that cannot tell them apart asks the declining rep
-- again every night. That is the one outcome this feature exists to avoid, so
-- the decision is recorded as its own fact rather than inferred from the
-- absence of a credential.
--
-- WHOSE AUTHORITY. passport_id is the rep's OWN passport: the single production
-- mint binds on_behalf_of and granted_by to the same session user, and no
-- admin-side mint path exists. This table therefore RECORDS which credential a
-- rep minted for this purpose; it never confers one. An admin enabling the
-- feature for the workspace is a separate fact and lives in `setting`, because
-- workspace intent and a personal credential are different things and merging
-- them is how an admin ends up able to grant on somebody's behalf.
--
-- ON DELETE CASCADE on the rep, matching brief_run: a departed colleague's
-- standing authority is not a record anybody needs, and leaving it would keep
-- a grant row pointing at a user who cannot answer for it.
--
-- ON DELETE RESTRICT on the passport, and the choice is load-bearing.
--
-- Cascade is wrong: deleting the grant makes the product offer the first-time
-- question again, which reads as it having forgotten a decision the rep made.
--
-- SET NULL is wrong too, and less obviously: it clears the column without
-- touching the state, landing the row in exactly the granted-with-no-passport
-- shape the CHECK above forbids — so the constraint would refuse the delete
-- anyway, at a moment nobody expects a grant to be involved.
--
-- RESTRICT says the real rule: a passport row is never deleted while a rep's
-- standing authority rests on it. Revocation and expiry do not delete it; they
-- set revoked_at or pass expires_at, and the reads resolve liveness from those
-- by joining. Nothing in the product deletes a passport today, so RESTRICT
-- costs nothing and fails loudly if something starts to.
CREATE TABLE agent_standing_grant (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    agent_spec text NOT NULL,
    -- granted:  the rep said yes, and the passport below carries it.
    -- declined: the rep said no. Remembered so they are not asked again.
    --
    -- There is deliberately no "lapsed" value. Whether the credential still
    -- works is the passport's fact and it changes when nothing writes here — a
    -- passport expires at a moment nobody observes — so a stored state would
    -- drift out of step with it and every reader would hold a row claiming
    -- authority that stopped working hours ago. The reads resolve liveness by
    -- joining the passport instead.
    state text DEFAULT 'granted' NOT NULL,
    passport_id uuid,
    decided_at timestamptz DEFAULT now() NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT agent_standing_grant_state_check
        CHECK (state IN ('granted', 'declined')),
    -- A granted row names the credential the rep minted; a declined row names
    -- none, because there is nothing it could be the authority for. Granted
    -- with no passport is the shape this feature must never reach: it reads as
    -- standing authority to anything checking the state, while carrying
    -- nothing to act with.
    CONSTRAINT agent_standing_grant_passport_shape
        CHECK ((state = 'granted' AND passport_id IS NOT NULL)
            OR (state <> 'granted' AND passport_id IS NULL))
);

ALTER TABLE agent_standing_grant
    ADD CONSTRAINT agent_standing_grant_pkey PRIMARY KEY (id);

-- One answer per rep per agent. Asking twice and recording two answers is the
-- same defect as asking twice, one layer down: a second row would let the
-- product read whichever it found first.
ALTER TABLE agent_standing_grant
    ADD CONSTRAINT uq_agent_standing_grant_user_spec UNIQUE (user_id, agent_spec);

ALTER TABLE agent_standing_grant
    ADD CONSTRAINT agent_standing_grant_user_fkey
    FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE;

-- The passport must belong to the REP THE GRANT IS FOR, and this is the
-- constraint that makes "nobody is acted for by a credential they did not mint"
-- true end to end.
--
-- The mint alone is not enough. It binds a passport to its own owner correctly,
-- and no admin path can mint one for somebody else — but a GRANT row pairing
-- one rep's user id with another rep's passport reintroduces exactly the hole
-- from the other side: the fan-out reads (user, passport) and runs for the named
-- rep under the named credential, so the run acts as the PASSPORT's owner on the
-- say-so of whoever wrote the row. Nothing about the mint prevents that pairing.
--
-- A composite foreign key onto (id, on_behalf_of) is what refuses it: the
-- database will not accept a passport id unless the user beside it is the user
-- that passport acts as. It holds for every writer, including ones not written
-- yet, which a check in one function cannot.
ALTER TABLE passport
    ADD CONSTRAINT uq_passport_id_owner UNIQUE (id, on_behalf_of);

ALTER TABLE agent_standing_grant
    ADD CONSTRAINT agent_standing_grant_passport_fkey
    FOREIGN KEY (passport_id, user_id) REFERENCES passport(id, on_behalf_of) ON DELETE RESTRICT;

-- The scheduler's read: every rep who granted this spec, so the nightly fan-out
-- enumerates live grants instead of seeding one authority-less occurrence.
CREATE INDEX idx_agent_standing_grant_live
    ON agent_standing_grant (agent_spec, user_id)
    WHERE state = 'granted' AND passport_id IS NOT NULL;

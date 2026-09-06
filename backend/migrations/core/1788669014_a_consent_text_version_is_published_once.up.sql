-- Bound the wait for the lock below. Without it an open transaction holding a
-- conflicting lock stalls every write to consent_event for as long as this
-- migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

-- The wording a consent proof points AT, published once and never edited.
--
-- consent_event already stores the sentence a subject was shown, verbatim. That
-- makes a proof row honest about what it holds; it does not make the wording
-- reproducible. "Show me exactly what this person saw on 4 March" is answerable
-- today only from the copy on the event, so a workspace cannot show the
-- canonical text it published, cannot show what ELSE that version said
-- alongside the sentence, and cannot prove the copy was not edited afterwards.
--
-- IMMUTABLE ONCE PUBLISHED, enforced by the trigger below rather than by
-- convention. A version whose text can change is not a version: every proof row
-- pointing at it would silently start describing wording the subject never saw,
-- and nothing in the tree would notice. Changing wording means publishing a NEW
-- row, which is what leaves the old proofs standing.
CREATE TABLE consent_text_version (
    id uuid DEFAULT uuidv7() NOT NULL,
    -- Which wording this is: the template key for a controller mail, or the
    -- purpose's own key for a preference-centre sentence.
    key text NOT NULL,
    -- The version within that key. Text, not an integer, because the two
    -- version spaces in the tree today disagree — comms_outbound.template_version
    -- is an integer and consent_event.policy_version is text ('v1') — and a
    -- column that has to hold both cannot be the narrower of the two.
    version text NOT NULL,
    -- Where and to whom this wording applies. NULL means "everywhere": a
    -- version narrowed to a jurisdiction or locale is the exception, and a
    -- default of 'all' would make the common case state something it does not
    -- know.
    jurisdiction text,
    locale text,
    channel text,
    purpose_id uuid,
    subject_line text,
    -- The words themselves. NOT NULL: a version with no body names nothing, and
    -- a proof row pointing at it would be worse than one pointing nowhere.
    body text NOT NULL,
    -- What else this version disclosed beside the sentence — the controller's
    -- identity, the objection route, the retention period. Held as JSON because
    -- the set differs per jurisdiction and a column per disclosure would be a
    -- schema change every time a pack lands.
    disclosures jsonb NOT NULL DEFAULT '{}'::jsonb,
    effective_from timestamptz,
    effective_until timestamptz,
    -- NULL until published. A draft may be edited; a published row may not, and
    -- this column is what the trigger below reads to tell them apart.
    published_at timestamptz,
    -- sha256 of subject_line and body, so a proof row can carry the hash beside
    -- its own copy and a later reader can prove the two still agree.
    --
    -- NOT recomputed in SQL, and the reason is worth stating so nobody tries
    -- again: consent.ContentHashOf separates subject from body with a NUL, and
    -- Postgres refuses a NUL byte in a text value outright ("invalid byte
    -- sequence for encoding UTF8: 0x00"), so no CHECK and no generated column
    -- can reproduce it. The pairing is held in Go instead, by
    -- TestOnlyContentHashOfProducesAStoredHash, which fails if any writer
    -- spells the hash itself rather than calling the one function.
    content_hash text,
    CONSTRAINT consent_text_version_pkey PRIMARY KEY (id),
    CONSTRAINT consent_text_version_purpose_fkey
        FOREIGN KEY (purpose_id) REFERENCES consent_purpose(id) ON DELETE RESTRICT,
    -- A published row carries that hash: it is what a proof row is compared
    -- against, and publishing without one defeats the point of publishing.
    CONSTRAINT consent_text_version_published_shape
        CHECK ((published_at IS NULL) OR (content_hash IS NOT NULL)),
    -- A window that ends before it starts describes no period at all.
    CONSTRAINT consent_text_version_window
        CHECK ((effective_until IS NULL) OR (effective_from IS NULL)
               OR (effective_until > effective_from))
);

-- One published row per key+version. A second would make "which wording is
-- v2 of the newsletter sentence" a question with two answers, and a proof row
-- naming that version could not be resolved to either.
--
-- Partial, so drafts of the same version may exist while one is worked on.
CREATE UNIQUE INDEX consent_text_version_published
    ON consent_text_version (key, version)
    WHERE published_at IS NOT NULL;

-- What a proof row was rendered from.
--
-- NULLABLE, and it stays nullable: every consent_event written before this
-- migration genuinely does not know, and backfilling from today's catalog would
-- claim those subjects saw wording that may have changed since. The verbatim
-- policy_text beside it remains what those rows stand on.
ALTER TABLE consent_event
    ADD COLUMN consent_text_version_id uuid REFERENCES consent_text_version(id);

-- Published wording cannot be edited.
--
-- A trigger and not a convention: the reason a version exists is that a proof
-- row can point at it, and text that changes under a live pointer makes every
-- such proof describe a screen the subject never saw. It refuses the CONTENT
-- columns only — effective_until may still be set, because withdrawing a
-- version from use is not the same as rewriting what it said.
CREATE OR REPLACE FUNCTION consent_text_version_is_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.published_at IS NULL THEN
        RETURN NEW;
    END IF;
    IF NEW.key IS DISTINCT FROM OLD.key
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.subject_line IS DISTINCT FROM OLD.subject_line
       OR NEW.body IS DISTINCT FROM OLD.body
       OR NEW.disclosures IS DISTINCT FROM OLD.disclosures
       OR NEW.content_hash IS DISTINCT FROM OLD.content_hash
       OR NEW.published_at IS DISTINCT FROM OLD.published_at
       -- Not only the words: what the version APPLIED TO is evidence too.
       -- Backdating effective_from makes a version claim it was live on a day
       -- it was not, which is how a proof row from March is made to resolve
       -- against wording published in June — retroactive fabrication of the
       -- evidentiary window, and unlike an edited body it leaves no trace.
       -- jurisdiction, locale and channel say who it applied to; purpose_id
       -- says what it is evidence OF.
       OR NEW.effective_from IS DISTINCT FROM OLD.effective_from
       OR NEW.jurisdiction IS DISTINCT FROM OLD.jurisdiction
       OR NEW.locale IS DISTINCT FROM OLD.locale
       OR NEW.channel IS DISTINCT FROM OLD.channel
       OR NEW.purpose_id IS DISTINCT FROM OLD.purpose_id THEN
        RAISE EXCEPTION 'consent_text_version % is published and its wording cannot change; publish a new version instead', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER consent_text_version_no_edit_after_publish
    BEFORE UPDATE ON consent_text_version
    FOR EACH ROW
    EXECUTE FUNCTION consent_text_version_is_immutable();

-- The trigger above cannot see a DELETE, and the app role inherits one from the
-- baseline's default privileges. Without this, published wording is rewritable
-- by deleting the row and inserting it again under the same key and version —
-- and PublishTextVersionTx's hash check does not catch it, because after the
-- delete there is no published row left to compare against.
--
-- Column-scoped like the sibling evidence tables (1788529047 did the same for
-- consent_event and communication_decision, which carry margince_app=ard for
-- this reason): closing a version's window is a legitimate write, rewriting
-- what it said is not.
REVOKE UPDATE, DELETE ON TABLE consent_text_version FROM margince_app;
GRANT UPDATE (effective_until) ON TABLE consent_text_version TO margince_app;

-- The lead table says almost nothing about itself.
--
-- Four CHECK constraints over 34 columns, the thinnest set of any record table
-- here — `company` carries thirteen. Seven more rules are true of every row
-- today and are enforced only by the one Go function that happens to write
-- them, which means they hold exactly as long as that function is the only
-- writer. A constraint is what makes a rule true of the TABLE.
--
-- What the first two buy, concretely: nothing stopped a lead sitting at
-- `status = 'promoted'` with no `promoted_contact_id`. Two queries then
-- disagree about one row — "how many leads did we promote" counts the status
-- and says one, "show me the contact this lead became" follows the pointer and
-- finds nothing. Neither is wrong; the row is.
--
-- VALIDATED rather than NOT VALID, unlike 1789605300 one table over. That
-- migration's columns were new, so every existing row was NULL and validation
-- would have proved nothing; these columns are old and populated, and the claim
-- being made is about the rows that are already there. A constraint that
-- declines to check them asserts the thing this migration exists to establish.
-- If an installation holds a row that violates one, the migration says so
-- loudly, which is the outcome worth having.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead
    -- NOT here, though the issue asks for it: `status = 'disqualified'` does NOT
-- imply a disqualify_reason_id. The field is optional by design, and the store
-- says why beside it — "an absent reason is a disqualification nobody explained
-- rather than one naming a retired code", kept so the governed agent path can
-- close a lead it cannot pick a reason code for. The UI always sends one. A
-- constraint here would refuse a write the product deliberately accepts.
--
-- The outcome shape: a terminal status names what it produced. `promoted`
    -- is archived BY the promotion — one statement sets both — so the row that
    -- claims promotion carries the moment it happened.
    --
    -- Note the interaction with lead_promoted_contact_id_fkey, which is ON
    -- DELETE SET NULL: deleting a contact a promoted lead points at now fails
    -- this check rather than quietly emptying the pointer. That is the better
    -- of the two, and it is the reason the constraint is stated in terms of the
    -- pointer rather than only the timestamp — a promoted lead whose contact is
    -- gone is the exact disagreement above, arrived at from the other side.
    ADD CONSTRAINT lead_promoted_names_its_contact
        CHECK (status <> 'promoted'
               OR (promoted_contact_id IS NOT NULL AND archived_at IS NOT NULL)),

    -- The score is a 0-100 scale everywhere it is read, and lead_score_history
    -- already refuses anything else. The live columns accepted -5 and 900,
    -- so the table recording what a score WAS could not faithfully record a
    -- value its own parent allowed.
    ADD CONSTRAINT lead_score_range CHECK (score BETWEEN 0 AND 100),
    ADD CONSTRAINT lead_score_computed_range
        CHECK (score_computed IS NULL OR score_computed BETWEEN 0 AND 100),

    -- A human override RETAINS the machine value rather than replacing it:
    -- that is what lets the override be explained, and what the recompute
    -- suppression is measured against. An override with no computed score
    -- behind it has thrown away the thing it is overriding.
    ADD CONSTRAINT lead_override_retains_the_computed_score
        CHECK (score_override_reason IS NULL OR score_computed IS NOT NULL),

    -- Clocks do not run backwards. Both of these are stamped by passes that
    -- read the row's own created_at, so an earlier value is a bug in the pass
    -- rather than a lead that was answered before it existed.
    ADD CONSTRAINT lead_first_response_follows_creation
        CHECK (first_response_at IS NULL OR first_response_at >= created_at),
    ADD CONSTRAINT lead_sla_breach_follows_creation
        CHECK (sla_breached_at IS NULL OR sla_breached_at >= created_at);

-- And the NULL branch of the status_set_by rule, said out loud.
--
-- The column is nullable and the constraint had no `IS NULL OR` arm, so NULL
-- was admitted by three-valued logic rather than by intent. Its two neighbours
-- — lead_email_norm and lead_score_override_reason_check — both spell theirs,
-- and a reader cannot tell which this one meant. Same admitted set, stated
-- rather than inherited.
ALTER TABLE lead DROP CONSTRAINT lead_status_set_by_check;

ALTER TABLE lead
    ADD CONSTRAINT lead_status_set_by_check
        CHECK (status_set_by IS NULL OR status_set_by IN ('human', 'system'));

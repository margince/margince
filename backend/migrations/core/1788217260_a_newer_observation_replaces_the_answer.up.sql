-- A newer observation replaces the answer a field holds.
--
-- Until now a machine fill CLAIMED an unanswered field and never replaced one,
-- so a correspondent who changed jobs or phone kept the details they had when
-- the record was first written. The rule becomes recency: the most recently
-- OBSERVED value wins, whoever wrote the one it replaces, and what it replaced
-- stays on the row so a human can put it back.
--
-- observed_at is when the source SAID it, not when this pass ran. Mail arrives
-- late, is re-delivered, and is read by a job that may run days after capture;
-- comparing processing times would let a replayed two-year-old signature
-- outrank last week's. The column carries a DEFAULT because four writers insert
-- these rows without one — person_children, providerclaimfill, enrichsignature
-- and the merge relink — and a bare NOT NULL would break every one of them.
--
-- superseded_* is a one-level undo buffer. The audit log records WHICH fields
-- changed and never their values (an append-only log outlives Art. 17 erasure),
-- so without these three columns the value a human is being asked to restore
-- exists nowhere queryable.
--
-- person_phone carries superseded_phone_id rather than a value, because value
-- plus timestamp does not identify a row: the table permits several live
-- non-primary numbers of one type, and "unarchive the old one" is ambiguous
-- among them. The provider-claim revert already learned this and records ids.
SET LOCAL lock_timeout = '3s';

ALTER TABLE person_profile_field
    ADD COLUMN observed_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN superseded_value text,
    ADD COLUMN superseded_captured_by text,
    ADD COLUMN superseded_observed_at timestamptz;

-- The backfill runs with the row's own updated-at trigger DISABLED.
--
-- A plain UPDATE here would fire trg_person_profile_field_updated, which
-- rewrites updated_at and bumps version on every existing row. A human's
-- correction is judged against that column (ai.Verdict.AsOf, applied by
-- person360's readProfileFields): a verdict older than the value's current form
-- is treated as being about a value the record no longer holds, and is not
-- shown. Touching updated_at across the table would therefore retire every
-- correction anybody has ever made, in one statement, at deploy time — with no
-- value changed and nothing to notice.
ALTER TABLE person_profile_field DISABLE TRIGGER trg_person_profile_field_updated;
UPDATE person_profile_field SET observed_at = created_at;
ALTER TABLE person_profile_field ENABLE TRIGGER trg_person_profile_field_updated;

COMMENT ON COLUMN person_profile_field.observed_at IS
    'When the source stated this value, not when the pass read it. A newer observation replaces the row; an older or equal one leaves it alone.';
COMMENT ON COLUMN person_profile_field.superseded_value IS
    'The value this row replaced, kept so a human can restore it. Null when this row answered an empty field.';

ALTER TABLE person_phone
    ADD COLUMN observed_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN superseded_phone_id uuid REFERENCES person_phone (id) ON DELETE SET NULL;

-- Trigger disabled for the profile-field backfill's reason: a version bump
-- across every number on the installation is a change nobody made, and an
-- If-Match write held by a client that read the row a moment earlier would
-- start failing on a row whose value never moved.
ALTER TABLE person_phone DISABLE TRIGGER trg_person_phone_updated;
UPDATE person_phone SET observed_at = created_at;
ALTER TABLE person_phone ENABLE TRIGGER trg_person_phone_updated;

COMMENT ON COLUMN person_phone.superseded_phone_id IS
    'The archived row this number replaced. Identity and not value, because several live numbers of one type are permitted.';

-- address and website: a signature and a business card both state them, and the
-- vocabulary is shared by both readers.
ALTER TABLE person_profile_field DROP CONSTRAINT person_profile_field_field_check;

ALTER TABLE person_profile_field
    ADD CONSTRAINT person_profile_field_field_check
    CHECK (field IN ('title', 'phone', 'role', 'linkedin', 'org_name', 'address', 'website'));

SET LOCAL lock_timeout = '3s';

-- A disclosure duty somebody owns, or has excused, or has already met.
--
-- The table shipped with five states: open, queued, completed, blocked and
-- not_required. That vocabulary can say a duty exists and that a mail went out
-- for it, and nothing else. Three things a privacy officer actually does had
-- no representation at all:
--
--   * taking a case, so the queue shows who is working it rather than a pile
--     nobody has claimed;
--   * excusing one on a stated ground — Art. 14(5) disapplies the duty where
--     the subject already has the information, where notice is impossible, or
--     where disclosure is laid down by law. Before this the only way to close
--     such a case was `not_required`, which records the conclusion and destroys
--     the reason;
--   * recording that the disclosure happened somewhere this installation did
--     not send it from, which is the ordinary case for a contact told in a
--     meeting or by a colleague's own mail.
--
-- `completed` stays and keeps its meaning, so nothing already closed is
-- reinterpreted. Of the three new states, `assigned` is a live one — the duty
-- is still owed, it just has a name on it — and the two excusing states END a
-- duty, which takes the ways one can end from two to four. They are
-- distinguishable from `completed` and from each other, which is the point: an
-- auditor asking "why is this closed" gets a different answer for each, where
-- before a case closed without a mail could only say not_required.
ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_state;

ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_state
        CHECK (state = ANY (ARRAY[
            'open'::text,
            'assigned'::text,
            'queued'::text,
            'completed'::text,
            'provided_elsewhere'::text,
            'exempt_with_reason'::text,
            'blocked'::text,
            'not_required'::text
        ]));

-- Why this case was excused or where the disclosure was provided, in the
-- officer's own words. NOT a code: the grounds in Art. 14(5) are stated in
-- prose in the regulation and an installation's ground is usually specific to
-- the contact ("told at the workshop on the 3rd"). A closed enum would force
-- every real reason into `other`, which records nothing.
--
-- Bounded at 500 because it is a justification, not a case file, and an
-- unbounded text column on a compliance record is how a free-text field becomes
-- somewhere people paste personal data.
ALTER TABLE privacy_notice_case
    ADD COLUMN resolution_note text;

ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_resolution_note_length
        CHECK (resolution_note IS NULL OR char_length(resolution_note) <= 500);

-- Who excused or closed it. Nullable and SET NULL on delete for the same reason
-- owner_user_id is: a compliance record outlives the seat, and a case must not
-- vanish with the person who closed it.
ALTER TABLE privacy_notice_case
    ADD COLUMN resolved_by uuid
        REFERENCES app_user(id) ON DELETE SET NULL;

-- When somebody took it. Distinct from updated_at, which moves for any write
-- including a merge relinking the contact — a queue that sorted by updated_at
-- would reorder itself when nobody touched the case at all.
ALTER TABLE privacy_notice_case
    ADD COLUMN assigned_at timestamptz;

-- The two new excusing states say why, and every other state gives no reason.
-- Without this an exempt case carrying no note is indistinguishable from one
-- somebody closed and could not be bothered to justify, which is precisely the
-- record this change exists to produce.
ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_resolution_shape
        CHECK ((state IN ('exempt_with_reason', 'provided_elsewhere'))
               = (resolution_note IS NOT NULL));

-- NO assigned-names-an-owner CONSTRAINT, deliberately, and it is worth saying
-- why the obvious one is wrong here.
--
-- `CHECK (state <> 'assigned' OR owner_user_id IS NOT NULL)` reads correctly
-- and would break deleting a user. owner_user_id is ON DELETE SET NULL, because
-- a compliance record has to outlive the seat that created it — so removing an
-- employee nulls their column, and on an assigned case that would produce
-- exactly the row the CHECK forbids. The delete would abort, and the person
-- doing it would see a privacy table refuse to let somebody leave the company.
--
-- The fix is not to make the FK restrict, which would be the same deadlock from
-- the other side. It is that "assigned with no owner" is a REAL state: the duty
-- was claimed by somebody who has since gone, and it is now owed and unclaimed.
-- The queue already shows it, because `assigned` is an unresolved state, and a
-- reader asking who is accountable reads owner_user_id and finds nobody. That
-- is the honest answer.

-- The completion shape has to learn the two new terminal states, or excusing a
-- case would fail on a CHECK written when there were only two ways to end one.
ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_completion_shape;

ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_completion_shape
        CHECK ((state IN ('completed', 'not_required',
                          'exempt_with_reason', 'provided_elsewhere'))
               = (completed_at IS NOT NULL));

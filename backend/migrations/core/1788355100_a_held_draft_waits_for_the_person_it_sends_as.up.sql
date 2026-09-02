-- A held draft is decided by the person it would go out as, and the drafts
-- staged before that was true record nobody.
--
-- Releasing a held draft SENDS it, and the send stamps its identity from the
-- approving human: the mailbox credential, the From display name and the
-- signature all come from whoever pressed Accept. Deciding one is therefore
-- restricted to the human the automation acted for (approvals' selfOnlyKinds),
-- and that human is read from approval.on_behalf_of.
--
-- Rows staged before this release carry no on_behalf_of at all: the workflow
-- engine staged them under the bare system principal, so there is nobody
-- recorded for the predicate to narrow to. Under the new rule they are
-- decidable by no one, which would leave them pending until their own window
-- closes with a card nobody can clear.
--
-- They are expired here rather than backfilled. Nothing in the row says whose
-- message it is: the anchor activity carries no owner for an email, so any
-- backfill would be this migration GUESSING whose mailbox a customer-facing
-- message should leave from — and guessing wrong sends a stranger's reply into
-- a live thread. Expiring is the reversible half of that choice: the automation
-- that composed each one fires again and stages a fresh draft, now carrying its
-- owner.
--
-- Scoped three ways so it cannot reach anything else: this kind, this defect
-- (on_behalf_of IS NULL), and rows a human could still have acted on.
UPDATE approval
   SET status = 'expired',
       expires_at = now() - interval '1 day'
 WHERE kind = 'held_draft'
   AND on_behalf_of IS NULL
   AND status = 'pending';

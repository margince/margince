// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Recording who wrote an activity in the system it was imported from.
//
// The repair that calls this reads a mirror of that system and hands over one
// answer per record. It never creates a row: an activity this installation does
// not hold is the caller's mistake to see, not ours to paper over.
//
// WHY THIS IS NOT A PATCH ON THE ORDINARY UPDATE PATH. `captured_by` is stamped
// from the authenticated principal and answers who recorded the row HERE; this
// answers who wrote it THERE, and on an imported row those are different
// colleagues. Keeping them apart is the whole point, so the write that fills one
// must not be able to touch the other — this statement names its two columns
// and nothing else.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SourceAuthorInput is one record's answer: who wrote it, in whichever of the
// two spellings the source could give.
type SourceAuthorInput struct {
	// AuthorID is the member who wrote it, when the author holds a seat here.
	AuthorID *ids.UUID
	// AuthorName is the source system's own spelling, for an author who never
	// held one. At least one of the two must be set; see SetSourceAuthorTx.
	AuthorName *string
}

// SourceAuthorOutcome is what happened to one record, in the words the wire
// reports them.
type SourceAuthorOutcome string

// The three answers this store gives.
//
// `unchanged` was once the caller's to decide, from a digest it read before
// calling here. That was wrong twice over, and both ways were found in review.
// The decision needs the row lock this function takes, or a concurrent write
// lands between the reading and the skipping and the two records disagree
// forever. And it needs the visibility check this function makes, or a caller
// outside an activity's audience learns whether its author matches a guess —
// a refusal for a wrong guess, `unchanged` for a right one — without ever being
// allowed to read the row.
//
// So it is answered HERE, under both. Whether the answer is NEWER than the one
// on record is still the ledger's question and still the caller's.
const (
	SourceAuthorApplied   SourceAuthorOutcome = "applied"
	SourceAuthorSkipped   SourceAuthorOutcome = "skipped"
	SourceAuthorUnchanged SourceAuthorOutcome = "unchanged"
)

// SetSourceAuthorTx records the author on one activity, inside the caller's
// transaction, and answers what it did.
//
// IN THE CALLER'S TRANSACTION, deliberately. The repair writes its own ledger
// row for the same record in the same commit, and a ledger that could say
// "attributed" about a row whose write was rolled back is worse than no ledger:
// the next run would read it and skip the record forever.
//
// The refusals are the interesting part, and each is a `skipped` with a reason
// rather than an error, because a batch of five hundred must not die on one bad
// row:
//
//   - the activity is not here, or is archived — an erasure may have taken it,
//     and writing an author back onto a row somebody destroyed would restore a
//     name the erasure just removed;
//   - it is outside the caller's own audience, which answers in exactly those
//     same words, deliberately: a caller who may not read a message must not be
//     able to tell it from one that is missing;
//   - it carries no `source_system` — the CHECK on the table refuses an author
//     on a row that came from nowhere, and answering that as a skip names the
//     reason instead of surfacing a constraint violation;
//   - the author names a seat this installation does not have.
//
// A record under a retention hold is NOT refused. The hold protects content —
// what was said — and an author is a marker like the date or the direction. The
// write below reaches the restricted-mutation trigger on its own terms, and if
// the estate disagrees the trigger says so rather than this function guessing.
func (s *Store) SetSourceAuthorTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, in SourceAuthorInput,
) (SourceAuthorOutcome, string, error) {
	// The object grant is `activity:update`, the same one an ordinary edit
	// takes, and that is the right object: this writes a column on an activity
	// and the RBAC vocabulary has no finer name for "rewrite attribution".
	//
	// It is deliberately NOT the whole gate. Who may reach the repair at all is
	// answered one layer up, where the route calls auth.RequireHuman — the
	// contract's `x-agent-access: human-only` is a declaration and refuses
	// nobody by itself. Both halves are needed: this one bounds WHAT may be
	// written, that one bounds WHO may ask.
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return SourceAuthorSkipped, "", err
	}
	if in.AuthorID == nil && (in.AuthorName == nil || *in.AuthorName == "") {
		// Refused rather than treated as "clear it". A caller that sends
		// neither has lost its answer somewhere upstream, and silently emptying
		// an attribution it previously wrote is the one outcome nobody could
		// have intended.
		return SourceAuthorSkipped, "no author given: send source_author_id, source_author_name, or both", nil
	}

	held, reason, err := reachableForWrite(ctx, tx, id.UUID)
	if err != nil || reason != "" {
		return SourceAuthorSkipped, reason, err
	}

	beforeID, beforeName, reason, err := attributableNow(ctx, tx, id.UUID, in)
	if err != nil {
		return SourceAuthorSkipped, "", err
	}
	if reason != "" {
		return SourceAuthorSkipped, reason, nil
	}

	// DID ANYTHING ACTUALLY CHANGE? Asked here rather than by the caller, and
	// asked of the ROW rather than of a digest beside it.
	//
	// The repair is resumed and re-run across tens of thousands of records, so a
	// clean second pass offers answers already standing. Rewriting them would
	// bump `version` and `updated_at` on every one — trg_activity_updated fires
	// on any UPDATE — restamping a whole timeline for no change, and reporting
	// `applied` for work not done.
	//
	// The two columns just read by attributableNow ARE the stored answer, read
	// under the lock taken above and behind the visibility check beside it.
	// Comparing them needs no second table and cannot race: a concurrent
	// erasure or a rival batch must wait for this lock before it can make the
	// comparison stale.
	if sameAuthor(beforeID, beforeName, in) {
		return SourceAuthorUnchanged, "", nil
	}

	p := storekit.NewPatch()
	p.Set("source_author_id", beforeID, in.AuthorID)
	p.Set("source_author_name", beforeName, in.AuthorName)
	// APPLIED THROUGH THE LOCK, not through a nil version. There is genuinely
	// no version to compare: the wire takes no If-Match, because the repair
	// walks records the caller enumerated from another system and holds no
	// version for any of them. Saying that in the types is the point — a nil
	// pinned version reads as a caller who HAD one and declined it, and drops
	// the If-Match clause silently. The row is already held FOR UPDATE by
	// lockActivityForWrite above, and ApplyLocked takes that witness so the
	// guarantee is the lock rather than an absent comparison.
	lock, err := storekit.LockRow(ctx, tx, "activity", id.UUID, activityArchivedFilter(held))
	if err != nil {
		return SourceAuthorSkipped, "", err
	}
	if reason, err := applyThroughSavepoint(ctx, tx, p, lock); err != nil || reason != "" {
		return SourceAuthorSkipped, reason, err
	}
	// The audit goes on the OUTER transaction, not the savepoint: it belongs
	// with the ledger row the caller writes next, and both must stand or fall
	// with the attribution as one commit.
	if _, err := storekit.Audit(ctx, tx, "import", "activity", id.UUID,
		map[string]any{"source_author_id": beforeID, "source_author_name": beforeName},
		map[string]any{"source_author_id": in.AuthorID, "source_author_name": in.AuthorName}); err != nil {
		return SourceAuthorSkipped, "", fmt.Errorf("activities: auditing the attribution: %w", err)
	}
	return SourceAuthorApplied, "", nil
}

// applyThroughSavepoint writes the patch inside a SAVEPOINT and answers a skip
// reason when the row itself refused it.
//
// A SAVEPOINT, because the write can be refused by a TRIGGER rather than by a
// predicate, and a refused statement aborts the surrounding transaction
// outright. Catching the error is not enough on its own: every statement after
// it — this batch's own bookkeeping included — would fail with "current
// transaction is aborted", so a single held activity would still take the whole
// batch down while reporting itself as a mere skip.
//
// An empty reason with a nil error means the write landed.
func applyThroughSavepoint(
	ctx context.Context, tx pgx.Tx, p *storekit.Patch, lock storekit.RowLock,
) (string, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return "", err
	}
	if err := p.ApplyLocked(ctx, sp, lock); err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return "", rbErr
		}
		// A row under a statutory retention hold refuses every update, by
		// trigger. lockActivityForWrite deliberately admits such a row — the
		// hold is about content, and most writes here are not — so the refusal
		// arrives at the write rather than at the lock.
		//
		// It is this record's answer, not the batch's: a repair walking ten
		// thousand activities will meet held ones, and failing the whole
		// transaction would abandon every row after it while the rows before it
		// stayed committed. A resumed run would then hit the same wall in the
		// same place, forever.
		if _, held := storekit.CheckViolation(err); held {
			return "the activity is under a statutory retention hold, which refuses every write until it lifts", nil
		}
		return "", err
	}
	// Released into the outer transaction. Without this the write above is
	// discarded when the savepoint falls out of scope, which would turn the
	// guard against losing a batch into a guard that loses every write.
	return "", sp.Commit(ctx)
}

// reachableForWrite takes the row lock and answers whether this caller may
// write the activity at all: `held` for the retention state the caller needs,
// or a skip reason when it may not.
//
// THE TWO REFUSALS ANSWER IN THE SAME WORDS, and that is the point of putting
// them together rather than the accident of it.
//
// A row that is absent or archived, and a row the caller simply may not read,
// must be indistinguishable. If they differed, somebody outside a message's
// audience could offer a guessed author and read which refusal came back as an
// answer — a wrong guess one way, a right one the other — learning what the row
// says without ever being allowed to read it.
//
// Both are SKIPS rather than errors for the same second reason: a batch of five
// hundred walked from another system will contain rows this installation cannot
// attribute, and failing the request on the first one would discard the other
// four hundred and ninety-nine and do it again on every resumed run.
func reachableForWrite(ctx context.Context, tx pgx.Tx, id ids.UUID) (held bool, reason string, err error) {
	held, err = lockActivityForWrite(ctx, tx, id)
	if err == nil {
		err = auth.EnsureActivityWritableIn(ctx, tx, id, !held)
	}
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return false, "no such activity here, or it has been archived", nil
	case err != nil:
		return false, "", err
	}
	return held, "", nil
}

// sameAuthor reports whether the row already carries exactly this answer.
//
// Both halves must match. An offer naming only a name, against a row carrying
// a name AND a seat id, is a different answer — it drops the seat — so it is a
// write rather than a no-op.
func sameAuthor(beforeID *ids.UUID, beforeName *string, in SourceAuthorInput) bool {
	switch {
	case (beforeID == nil) != (in.AuthorID == nil):
		return false
	case beforeID != nil && *beforeID != *in.AuthorID:
		return false
	case (beforeName == nil) != (in.AuthorName == nil):
		return false
	case beforeName != nil && *beforeName != *in.AuthorName:
		return false
	}
	return true
}

// attributableNow reads the row's current answer and decides whether this one
// may replace it: the before-image for the audit, and a reason when it may not.
//
// Split out of SetSourceAuthorTx because the refusals are their own question.
// The caller decides what to WRITE; this decides whether the record can carry
// an author at all, and each arm of it answers with words an operator can act
// on rather than a constraint violation.
func attributableNow(
	ctx context.Context, tx pgx.Tx, id ids.UUID, in SourceAuthorInput,
) (beforeID *ids.UUID, beforeName *string, reason string, err error) {
	var sourceSystem *string
	if err := tx.QueryRow(ctx, `
		SELECT source_system, source_author_id, source_author_name
		  FROM activity WHERE id = $1`, id).Scan(&sourceSystem, &beforeID, &beforeName); err != nil {
		return nil, nil, "", fmt.Errorf("activities: reading the activity before attributing it: %w", err)
	}
	if sourceSystem == nil || *sourceSystem == "" {
		return nil, nil, "the activity names no source system, so it was written here rather than imported", nil
	}
	if in.AuthorID == nil {
		return beforeID, beforeName, "", nil
	}
	// No liveness filter: a departed colleague is exactly who this repair most
	// often names, and a deactivated seat is still a seat. Only a row that does
	// not exist at all is a refusal.
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1)`, *in.AuthorID).Scan(&exists); err != nil {
		return nil, nil, "", fmt.Errorf("activities: resolving the named author: %w", err)
	}
	if !exists {
		return nil, nil, "the named author holds no seat in this installation", nil
	}
	return beforeID, beforeName, "", nil
}

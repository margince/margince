// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A captured message's audience is DERIVED, not set. One email is one row even
// when it reached four mailboxes, so no single seat's decision can be the
// answer: the row has to end at the strictest thing any importing seat asked
// for, whichever order the four syncs ran in.
//
// That is what makes this a recompute rather than a write. A writer that
// applied one seat's decision to the row would be correct only until the next
// seat's sync overwrote it, and which seat won would depend on scheduling. The
// recompute reads every contributor and derives the same answer from any of
// them, so running it twice, or running it from a sync that arrives late,
// cannot move the row somewhere a contributor did not ask for.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// audienceRank orders the audiences from most open to most closed. The
// recompute takes the maximum, which is what "strictest contributor wins"
// means in one line.
//
// `selected` outranks `participants` because it is the narrower set in
// practice: participants admits everyone stamped on the message, selected
// admits only the users and teams named on it. A human choosing `selected`
// has said something more specific than any derivation could, which is why
// the recompute refuses to move it at all (see RecomputeAudienceTx).
var audienceRank = map[string]int{
	audienceWorkspace:    0,
	audienceParticipants: 1,
	audienceSelected:     2,
}

// The two columns an audience audit image is made of. Named because SetAudience
// and the derivation both assemble that image, and a diff of two images built
// from different spellings of one column reads as a change nobody made.
const (
	auditFieldAudience       = "audience"
	auditFieldAudienceReason = "audience_reason"
)

const (
	audienceWorkspace    = "workspace"
	audienceParticipants = "participants"
	audienceSelected     = "selected"
)

// RecomputeAudienceTx derives one captured activity's audience from every
// contribution to it and writes the result when, and only when, it changed.
//
// The contributors, each able to tighten and none able to loosen:
//
//   - the workspace mail-sharing floor and each importing mailbox's posture,
//     both recorded on that seat's capture_import row at import;
//   - each importing seat's verdict on the message's thread, likewise;
//   - a human decision recorded on the row itself.
//
// A row with no import rows is not a captured row — a hand-logged note, or a
// captured row whose importing seat was deleted — and this leaves it alone.
// SetAudience is the writer for those, and the two must never both write one
// row: a derivation that overwrote a human's explicit `selected` would silently
// widen a message a person deliberately narrowed.
//
// Writes nothing when the derived audience equals the stored one, so a sync
// that changes nothing produces no audit row and no event. That is not an
// optimization: this runs on every import of every message, and an unconditional
// write would put one audit row and one activity.updated event on the bus per
// re-sync per message, which is a stream of events saying nothing happened.
func RecomputeAudienceTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) error {
	// The activity FIRST, and locked. Every writer of a derived row in this
	// tree takes the activity before the row derived from it (the embedding
	// upsert's share lock, the retraction's own), and a recompute that took
	// capture_import first would deadlock against them.
	//
	// The lock is also what makes the derivation correct rather than merely
	// plausible: two seats' syncs committing at once would otherwise each read
	// a set of import rows that did not include the other's, and each write a
	// row the other's contribution should have tightened.
	//
	// restricted_at IS NULL excludes a row under a statutory retention
	// obligation, and excluding it is the whole treatment rather than a
	// precaution. Such a row is already unavailable in every ordinary read
	// path for every principal, so tightening its audience changes nothing a
	// reader could observe, and widening it would be the one write here that
	// could make a held row more readable than the obligation allows. Leaving
	// it exactly as it is has neither effect.
	var stored string
	var storedReason *string
	err := tx.QueryRow(ctx, `
		SELECT audience, audience_reason FROM activity
		 WHERE id = $1 AND restricted_at IS NULL FOR UPDATE`,
		activityID).Scan(&stored, &storedReason)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Erased, deleted, or under a statutory hold. Nothing to derive an
			// audience for in any of the three.
			return nil
		}
		return fmt.Errorf("activities: reading the activity being recomputed: %w", err)
	}
	// A human's explicit member list is not a derivation's to move at all:
	// widening publishes what a person narrowed by hand, and narrowing discards
	// the member set they named, which no contribution here knows how to
	// rebuild.
	if stored == audienceSelected {
		return nil
	}
	// A human's decision binds in ONE direction, and it is asked BEFORE anything
	// is derived.
	//
	// Asymmetric on purpose. A person who narrowed their own correspondence has
	// said something no contribution can rebuild, so nothing here widens past
	// it. But write authority over an activity is broader than membership of it
	// — a link-less message admits any content-visible caller — so a colleague
	// merely cc'd can open a message by hand, and treating that as a veto would
	// let one seat's click outrank the mailbox owner's own posture.
	//
	// Asked FIRST because the contributions that held the message are still
	// there after a human opens it: the seat's import row still records the
	// posture it was captured under, and deriving from that would re-narrow the
	// row on the very next sync, silently undoing what the person did.
	if deref(storedReason) == ReasonManual {
		manual, err := manualDecisionStands(ctx, tx, activityID, stored)
		if err != nil || manual {
			return err
		}
	}
	derived, reason, ok, err := deriveAudienceTx(ctx, tx, activityID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	// A hold placed by a writer that leaves no capture_import row behind is a
	// contributor too, and it names itself in audience_reason.
	// >= , not >: on a TIE the row-carried reason wins. Both say participants,
	// but only one of them is recoverable — a mailbox posture is re-read from
	// its import row on every pass, while the floor and the ladder's hold exist
	// nowhere but this column. Letting the posture take the tie overwrites the
	// only record of the durable hold, and the message opens the moment that
	// mailbox's verdict clears.
	held, why := rowCarriedHold(stored, storedReason)
	if held && why == ReasonNoCounterparty {
		stands, err := noRecordHoldStands(ctx, tx, activityID)
		if err != nil {
			return err
		}
		held = stands
	}
	if held && audienceRank[audienceParticipants] >= audienceRank[derived] {
		derived, reason = audienceParticipants, why
	}
	if derived == stored && sameReason(storedReason, reason) {
		return nil
	}
	return writeDerivedAudienceTx(ctx, tx, activityID, stored, storedReason, derived, reason)
}

// deriveAudienceTx reads every contribution and returns the strictest, with
// the reason of whichever contributor set it. ok is false when the row has no
// import rows at all, which means it is not a captured row and not this
// function's to decide.
func deriveAudienceTx(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
) (audience string, reason string, ok bool, err error) {
	rows, err := tx.Query(ctx, `
		SELECT posture_at_import, verdict_status, verdict_reason
		  FROM capture_import
		 WHERE activity_id = $1
		 ORDER BY user_id`, activityID)
	if err != nil {
		return "", "", false, fmt.Errorf("activities: reading the import rows of %s: %w", activityID, err)
	}
	defer rows.Close()

	audience, reason = audienceWorkspace, ""
	for rows.Next() {
		var posture, status, why *string
		if err := rows.Scan(&posture, &status, &why); err != nil {
			return "", "", false, fmt.Errorf("activities: reading an import row of %s: %w", activityID, err)
		}
		ok = true
		want, whyDerived := contributionOf(posture, status, why)
		if audienceRank[want] > audienceRank[audience] {
			audience, reason = want, whyDerived
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", false, fmt.Errorf("activities: reading the import rows of %s: %w", activityID, err)
	}
	return audience, reason, ok, nil
}

// rowCarriedHold answers whether the audience already on the row is a hold that
// no capture_import row records, and which one.
//
// It reads audience_reason, which is also the column the API publishes as a
// sentence for the reader — so one column carries both the display string and
// the durable fact that a hold exists. That is a real compromise and worth
// naming: capture_import exists precisely so a decision has a per-contributor
// home, and these two writers were given none. The alternative shape is a
// column recording the hold with the reason derived from it for display.
//
// What keeps it sound for now is that the column has six writers, all in this
// tree: the capture sink's insert and its link-less update, SetAudience, this
// function, ClearCounterpartyHoldTx below, and the migration that introduced
// the column. The sixth is the only one that CLEARS rather than sets, and it is
// scoped to the one value it is entitled to remove — see its own comment. A new writer that set
// the reason for display alone would break it, which is why the vocabulary is
// closed and TestTheTwoModulesSpellTheRowCarriedReasonsTheSameWay holds the two
// modules' spellings together.
//
// Three writers place such a hold, and none of them is re-derivable from the
// row afterwards:
//
//   - the WORKSPACE mail-sharing floor, which holds every new email whatever
//     the mailboxes want (`workspace_floor`);
//   - an importing seat's COUNTERPARTY hold (`counterparty`) and a sender's own
//     subject-line marker (`explicitly_confidential`). Both are decided at
//     capture, per message, and neither is re-derivable afterwards: a hold
//     lifted next week does not un-hold the mail it caught, and the subject
//     line is content the derivation deliberately never reads;
//   - the capture ladder, which holds a message it filed under no record at all
//     — a suppressed newsletter, an infrastructure notice (`no_record`).
//
// Read from the reason rather than re-derived, because neither condition can be
// recovered from the row afterwards. The ladder narrows only on its TERMINAL
// no-record outcomes, so a sender still awaiting a verdict is also link-less
// right now and is deliberately left open; and the mail-sharing setting moves
// the default for NEW mail only, so its value today says nothing about what it
// was when a message landed. Only the writer that made the hold knew, and the
// reason is where it said so.
func rowCarriedHold(stored string, storedReason *string) (bool, string) {
	if stored != audienceParticipants {
		return false, ""
	}
	switch why := deref(storedReason); why {
	case ReasonNoRecord, ReasonNoCounterparty, ReasonWorkspaceFloor, ReasonCounterparty, ReasonConfidentialMarker:
		return true, why
	}
	return false, ""
}

// noRecordHoldStands answers whether a "named nobody" hold is still the truth.
//
// It asks ONE question — has anything filed this row yet — and it is asked only
// of ReasonNoCounterparty, which is the hold capture writes when a record named
// nobody a contact could be created FOR. A calendar meeting is that case:
// attendance is a list, the mapper leaves the counterparty unset, and the
// limiter holds a row whose only defect is that nothing had filed it.
//
// It is deliberately NOT asked of ReasonNoRecord. That reason records a
// judgement about a SENDER — a suppression rule, a settled verdict, a thread
// the mailbox owner's own — and a link arriving afterwards says nothing about
// it. The two were one reason until this probe needed to tell them apart, and
// telling them apart by KIND was the mistake worth naming: a meeting-shaped
// record can carry a mail counterparty (the sink admits by counterparty shape,
// never by kind), reach the private-thread or suppression branch, and be held
// for a real reason. Inferring "structural" from kind would have opened exactly
// that message.
//
// Answering false only removes the row-carried hold; the derivation over the
// import rows still decides, and a seat's own posture or verdict still holds
// the row if one asks for it.
func noRecordHoldStands(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (bool, error) {
	var filed bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = $1)`,
		activityID).Scan(&filed); err != nil {
		return false, fmt.Errorf("activities: asking whether %s is filed anywhere: %w", activityID, err)
	}
	return !filed, nil
}

// contributionOf answers what ONE importing seat's row asks of the audience.
//
// Order matters and runs strictest-first: a seat whose verdict holds the
// message has said something about this message, and a posture that would open
// it says only what the mailbox asks of mail in general.
//
// A row with nothing decided on it contributes nothing — audience workspace,
// the floor. That is the state the migration's backfill leaves every existing
// row in, and it is the honest one: those rows were captured under a product
// that had no posture and no verdict, and inventing a hold for them now would
// hide mail that has been shared for months on a guess about what the seat
// would have wanted.
// why is the verdict's own account of itself — the word the reader is shown when
// this contribution is the strictest one (`legal`, `personnel`).
func contributionOf(posture, status, why *string) (audience, reason string) {
	switch deref(status) {
	case "held", "held_by_owner":
		// Never ReasonManual as the fallback: that word tells the next recompute
		// to stop deriving entirely, so a verdict that held without recording a
		// reason would freeze the row against every later contributor.
		return audienceParticipants, reasonOr(why, ReasonVerdict)
	case "unsure", "pending":
		return audienceParticipants, ReasonPendingVerdict
	case "cleared", "shared_by_owner":
		// Judged ordinary, or opened by the seat whose mail it is. The posture
		// does not re-close what a verdict opened: the posture's whole job was
		// to hold the message until something judged it, and something has.
		return audienceWorkspace, ""
	}
	switch deref(posture) {
	case "held", "classified":
		// reasonOr, not a flat ReasonPosture. The capture records WHY this seat's
		// mailbox held the message — a counterparty hold, the sender's own
		// marker, the workspace floor — and this is what carries that word onto
		// the activity row, where rowCarriedHold reads it back on every later
		// pass. Flattening it to `posture` loses the distinction that matters:
		// a posture is documented as clearable by a verdict and a hold is not,
		// so the message would open the moment its thread was judged ordinary.
		// TestACounterpartyHoldSurvivesAnotherSeatsClearedVerdict fails when
		// this and rowCarriedHold's recognized set are both weakened.
		return audienceParticipants, reasonOr(why, ReasonPosture)
	}
	return audienceWorkspace, ""
}

// writeDerivedAudienceTx lands the change: the column, the audit row and the
// activity.updated event in one transaction, the same shape SetAudience writes
// so that one consumer serves both writers.
func writeDerivedAudienceTx(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	stored string, storedReason *string, derived, reason string,
) error {
	before := map[string]any{auditFieldAudience: stored, auditFieldAudienceReason: deref(storedReason)}
	// The audience pin is the concurrency guard, re-stating what the caller read
	// under FOR UPDATE.
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET audience = $2, audience_reason = NULLIF($3, '')
		 WHERE id = $1 AND audience = $4 AND restricted_at IS NULL`,
		activityID, derived, reason, stored)
	if err != nil {
		return fmt.Errorf("activities: writing the derived audience of %s: %w", activityID, err)
	}
	if tag.RowsAffected() != 1 {
		// The row left the derivable world under the lock (see the read above).
		// Writing an audit row about a change that did not happen would put a
		// lie in the compliance trail.
		return nil
	}
	after := map[string]any{auditFieldAudience: derived, auditFieldAudienceReason: reason}
	changedBefore, changedAfter := storekit.ChangedColumns(before, after)
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", activityID.UUID, changedBefore, changedAfter)
	if err != nil {
		return err
	}
	// The same changed_fields shape SetAudience emits. The rescope consumer
	// reads the ROW's audience rather than this payload, so a derivation and a
	// human narrowing are one kind of event to everything downstream — which is
	// what lets the retraction of derived data have exactly one trigger.
	audience := crmcontracts.PublicEventActivityChangedFieldsAudience(derived)
	return storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Audience: &audience},
	})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func reasonOr(s *string, fallback string) string {
	if v := deref(s); v != "" {
		return v
	}
	return fallback
}

func sameReason(stored *string, derived string) bool {
	return deref(stored) == derived
}

// manualDecisionStands answers whether a human's audience decision survives
// what the contributors now ask for.
//
// It stands whenever the human's answer is at least as strict as the derived
// one: a person who narrowed cannot be widened, and a person who opened is
// overruled only by a contribution that genuinely holds the message. Deriving
// here rather than trusting the stored reason is what makes the second half
// true — a seat's own posture must still be able to hold a message a colleague
// opened by hand.
func manualDecisionStands(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, stored string,
) (bool, error) {
	derived, _, ok, err := deriveAudienceTx(ctx, tx, activityID)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return audienceRank[stored] >= audienceRank[derived], nil
}

// ClearCounterpartyHoldTx removes a row-level counterparty hold from the
// activities a seat's widening pass re-opened, and only from those.
//
// It lives here because activities owns the activity table; capture reaches it
// through the same function-typed seam it already uses for the recompute, which
// compose wires. A copy of this UPDATE inside capture is what the
// table-ownership gate refuses, and refuses for the reason this function's
// predicate demonstrates: the column has more than one writer and each one has
// to know which values are its own to clear.
//
// Scoped by VALUE, not by id alone. audience_reason also records a workspace
// floor, a sender's confidential marker, and a human's manual narrowing through
// SetAudience — clearing it unconditionally would republish messages nobody
// asked about. The predicate is what makes this a counterparty-only widening
// rather than a general un-hold.
func ClearCounterpartyHoldTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.ActivityID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity
		   SET audience_reason = NULL
		 WHERE id = ANY($1) AND audience_reason = $2`,
		activityIDs, ReasonCounterparty); err != nil {
		return fmt.Errorf("activities: clearing the counterparty hold from re-opened rows: %w", err)
	}
	return nil
}

// ClearConfidentialityVerdictHoldTx removes the one row-carried hold an owner
// is entitled to lift: a CLASSIFIER's confidentiality answer.
//
// Its sibling above clears a counterparty hold; this exists for the same reason
// and answers a different question. A row-carried hold outranks an opening
// contribution, which is right for the holds a recipient may not lift and wrong
// for the one they may. The owner pressed Share, the ledger recorded
// `shared_by_owner`, the derivation ran, read the row's reason and left the
// message held — the share worked and the hold ignored it.
//
// The predicate is the subtle part. `explicitly_confidential` on a row means
// two things that cannot be told apart by the word:
//
//   - the SENDER marked their own subject line. Not a recipient's to lift.
//   - a CLASSIFIER concluded the text asks for confidence. A judgement, and the
//     owner may disagree with it.
//
// So the CALLER decides which rows qualify and this writes them. capture owns
// the thread ledger and can see that a row's hold came from a classifier — a
// verdict recorded a `kind`, a sink marking never does — while this module owns
// `activity` and is the only one entitled to write the column. Reading the
// ledger here would be a module reaching into a sibling's table, which the
// ownership gate refuses and which would put the same rule in two places.
//
// Newer rows do not need any of this: capture's rowReasonForKind now sends a
// classifier's answer to the row as the generic `verdict`, which is not
// row-carried at all. The rows judged before that landed still carry the word,
// and they are the ones a rep cannot share today.
//
// Every other reason is left where it is: `counterparty` is this seat's standing
// decision about a PERSON rather than this message, `workspace_floor` is an
// admin's decision one seat cannot overrule, and `no_record` / `no_counterparty`
// are the capture ladder's filing facts rather than judgements about
// confidentiality. The value predicate stays in the UPDATE even though the
// caller has already chosen the ids, for the reason its sibling states: the
// column has more than one writer, and each has to say which values are its own
// to clear.
func ClearConfidentialityVerdictHoldTx(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.ActivityID,
) error {
	if len(activityIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity
		   SET audience_reason = NULL
		 WHERE id = ANY($1) AND audience_reason = $2`,
		activityIDs, ReasonConfidentialMarker); err != nil {
		return fmt.Errorf("activities: clearing a confidentiality verdict the owner has shared past: %w", err)
	}
	return nil
}

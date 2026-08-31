// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// limitLinkLessAudience keeps a captured message that will link to NO record
// inside its participants. The row scope makes a link-less activity
// workspace-shared — right for a hand-written note, wrong for a mailbox
// owner's correspondence with a sender the ladder just judged noise or
// infrastructure: nobody but the people on it has a reason to read it. Only
// the TERMINAL no-record outcomes qualify (captured without a counterparty,
// suppressed); a deferred sender may still be admitted and linked later, and
// a connector-supplied link keeps the row readable through that record.
func limitLinkLessAudience(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, decision counterpartyDecision) error {
	if decision.create || len(rec.Links) > 0 {
		return nil
	}
	if decision.traceOutcome != TraceCaptured && decision.traceOutcome != TraceSuppressed {
		return nil
	}
	// The row was inserted in this transaction, so the only way it is not
	// exactly one row is a bug in the insert above; saying so is cheaper than
	// a silent no-op.
	// No inequality guard: with mail sharing off the row was BORN
	// participants-only, and a pin that then matched zero rows would abort
	// the capture of a message that is already exactly as held as asked.
	// The reason travels with the hold. The audience derivation runs after this
	// and would otherwise see a participants-only row that no import row asks
	// for, and widen it back — the reason is how a hold placed for something
	// other than a mailbox's posture survives being recomputed.
	tag, err := tx.Exec(ctx,
		`UPDATE activity SET audience = $2, audience_reason = $3 WHERE id = $1`,
		id, audienceParticipants, audienceReasonNoRecord)
	if err != nil {
		return fmt.Errorf("capture: limiting a link-less message to its participants: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("capture: limiting a link-less message to its participants: %d rows, want 1", tag.RowsAffected())
	}
	return nil
}

// audienceParticipants is the activity audience a link-less captured message
// is held in (platform/auth ActivityContentClause names the arms).
const audienceParticipants = "participants"

// The reasons this module stamps on a held message, mirroring the constants of
// the same names in activities.
//
// Two copies because a module never imports a sibling: capture WRITES these
// words and activities READS them, and a drift on either side silently un-holds
// mail — the derivation stops recognising the hold, finds no contributor asking
// for one, and widens the row. TestTheTwoModulesSpellTheRowCarriedReasonsTheSameWay
// drives the sink and compares what it stamped against the activities constants,
// so it fails from either side.
const (
	// The mailbox asked for it, and a verdict on the thread can clear it.
	audienceReasonPosture = "posture"
	// Nothing has judged the thread yet.
	audienceReasonPendingVerdict = "pending_verdict"
	// The message is filed under no record at all.
	audienceReasonNoRecord = "no_record"
	// The workspace turned mail sharing off. No verdict clears this one.
	audienceReasonWorkspaceFloor = "workspace_floor"
	// This seat holds mail with one of the parties, whatever it is about.
	audienceReasonCounterparty = "counterparty"
	// The sender said so in the subject line.
	audienceReasonConfidentialMarker = "explicitly_confidential"
)

const audienceWorkspace = "workspace"

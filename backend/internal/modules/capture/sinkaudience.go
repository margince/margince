// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
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

// audienceReasonNoRecord mirrors activities.ReasonNoRecord, which this module
// cannot import (a module never imports a sibling). The two are one vocabulary
// and TestTheTwoModulesSpellTheRowCarriedReasonsTheSameWay fails if they drift.
const audienceReasonNoRecord = "no_record"

// audienceReasonWorkspaceFloor mirrors activities.ReasonWorkspaceFloor, for the
// same reason and held by the same test: the workspace turned mail sharing off,
// which is not any mailbox's posture and which no verdict clears.
const audienceReasonWorkspaceFloor = "workspace_floor"

// capturedAudience answers the audience a freshly captured activity is born
// with. Mail sharing ON (the default) births an email workspace-readable —
// the point of capturing into a shared CRM. Switched OFF, an email is held to
// its participants and the capturing mailbox owner from the moment it lands;
// the setting moves the default for NEW mail only, and non-mail kinds
// (meetings, channel messages) keep the workspace default either way.
// It answers a REASON with the audience. The workspace floor is a decision no
// capture_import row records — it is the workspace's, not any mailbox's — so a
// derivation reading import rows alone would find nothing asking for a hold and
// widen the message back on the very capture that just held it. The reason on
// the row is what carries the floor into every later recompute.
func capturedAudience(ctx context.Context, tx pgx.Tx, kind string) (audience, reason string, err error) {
	if kind != "email" {
		return audienceWorkspace, "", nil
	}
	sharing, err := settings.ApplyTx(ctx, tx, MailSharing)
	if err != nil {
		return "", "", fmt.Errorf("capture: reading the mail-sharing posture: %w", err)
	}
	if sharing {
		return audienceWorkspace, "", nil
	}
	return audienceParticipants, audienceReasonWorkspaceFloor, nil
}

const audienceWorkspace = "workspace"

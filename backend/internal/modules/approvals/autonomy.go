// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The track record a rep builds by deciding, one kind of proposal at a time.
//
// Trust is not a property of the software. It is one person's experience of one
// kind of proposal: a rep who has approved fourteen close-date confirmations
// unchanged has evidence about close dates and none about outbound mail. So the
// grain is (rep, kind), and approval_autonomy_policy holds one row per pair.
//
// WHAT IS HERE AND WHAT IS NOT. This counts decisions as they are made, so the
// record exists by the time there is something to weigh it for. It reads
// nothing back and it decides nothing: the mode column the table carries is
// written by no code in this package, because auto-apply has no decider —
// decidingactor.go refuses a system principal outright, approvals are decided
// by people. A reader and a writer for the mode ahead of that answer would be
// the surface for a promise the product cannot yet keep.
//
// The counters are stored rather than counted from the approval table, which
// was the first design. Approvals expire and are swept, and a retention policy
// will eventually delete decided rows — so a record derived from a table that
// forgets would reset a rep's earned standing whenever housekeeping ran.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// countDecisionTx records one decision against the deciding rep's track record
// for this kind.
//
// Called from inside the decision transaction, so a counted decision and the
// decision itself are one commit: a counter that could survive a rolled-back
// approval would offer autonomy on evidence of a decision nobody made.
//
// The row is created on first decision rather than when a rep is created. A
// policy row means "this rep has some history with this kind", so seeding one
// per rep per kind would fill the table with rows saying nothing and make
// "never decided" unreadable.
func countDecisionTx(ctx context.Context, tx pgx.Tx, userID ids.UUID, kind string, outcome decisionOutcome) error {
	column, err := outcome.column()
	if err != nil {
		return err
	}
	// The column is chosen from the closed set below, never taken from a
	// caller's string, so it is safe to format in — and it must be, because a
	// counter name is an identifier rather than a value.
	_, err = tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO approval_autonomy_policy (user_id, kind, %[1]s)
		 VALUES ($1, $2, 1)
		 ON CONFLICT (user_id, kind) DO UPDATE
		   SET %[1]s = approval_autonomy_policy.%[1]s + 1`, column),
		userID, kind)
	return err
}

// decisionOutcome is what a rep did, in the three shapes the ladder counts.
type decisionOutcome int

const (
	// outcomeApprovedClean is an approval that changed nothing. It is the one
	// that will earn a promotion offer: a rep who rewrites the payload every
	// time has told the software the opposite of "stop asking me".
	outcomeApprovedClean decisionOutcome = iota
	// outcomeApprovedEdited is an approval whose payload the rep rewrote:
	// agreement with the intent, disagreement with the detail. Counted apart
	// from a clean approval because folding the two together would promote
	// fastest exactly the kind that should keep asking.
	outcomeApprovedEdited
	outcomeRejected
)

// column names the counter this outcome increments.
func (o decisionOutcome) column() (string, error) {
	switch o {
	case outcomeApprovedClean:
		return "approved_clean", nil
	case outcomeApprovedEdited:
		return "approved_edited", nil
	case outcomeRejected:
		return "rejected", nil
	}
	return "", errors.New("crmapprovals: no counter for this decision outcome")
}

// decisionOutcomeOf reads what the decision already knows into the shape the
// ladder counts: the verdict, and the edited payload the caller either has or
// does not. It takes the payload rather than a second flag so the two cannot be
// swapped at a call site where both spellings would compile.
//
// Named apart from bundle.go's outcomeOf, which answers a different question —
// what a bulk call did to a row it could not decide.
func decisionOutcomeOf(approve bool, edited json.RawMessage) decisionOutcome {
	if !approve {
		return outcomeRejected
	}
	if len(edited) > 0 {
		return outcomeApprovedEdited
	}
	return outcomeApprovedClean
}

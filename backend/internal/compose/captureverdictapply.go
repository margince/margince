// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Committing a verdict: who said it, how sure they were, and what it costs to
// be wrong in each direction.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// applyOwnerDecision commits what a PERSON said about a sender.
//
// Split from applyJudged rather than sharing a bool at the call site, so the
// two authorities are visible as two entry points: a reader asking "what can an
// owner's click do" finds one function and the answer beside it.
func (e *CounterpartyVerdictEngine) applyOwnerDecision(ctx context.Context, row capture.PendingCounterparty, kind string) (int, error) {
	// No measurement: a person decided, and no model was asked.
	done, err := e.apply(ctx, row, kind, true, capture.VerdictMeasurement{})
	if err != nil {
		return 0, err
	}
	if done {
		return 1, nil
	}
	return 0, nil
}

// applyJudged commits one above-floor answer and reports whether this caller was
// the one that resolved the row.
//
// measured carries what the model said, so the ledger keeps how sure the answer
// was and which model gave it. A deterministic answer — a role mailbox, read off
// the address — passes the zero value, and the columns stay NULL rather than
// claiming a certainty nobody measured.
func (e *CounterpartyVerdictEngine) applyJudged(
	ctx context.Context, row capture.PendingCounterparty, verdict string, measured capture.VerdictMeasurement,
) (int, error) {
	done, err := e.apply(ctx, row, verdict, false, measured)
	if err != nil {
		return 0, err
	}
	if done {
		return 1, nil
	}
	return 0, nil
}

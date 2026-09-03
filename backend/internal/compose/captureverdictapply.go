// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Committing a verdict: who said it, how sure they were, and what it costs to
// be wrong in each direction.

import (
	"context"
	"fmt"

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
	// The create floor is enforced HERE and not only at the call sites, because
	// this is the chokepoint every model-made answer passes through on its way
	// to a record. A caller that checked the floor and a caller that forgot look
	// identical from the outside, and the one that forgot creates a contact on
	// evidence the product decided was too weak.
	//
	// A deterministic answer carries no measurement and is not subject to a
	// floor: a role mailbox is read off the address, and there is no confidence
	// to be below.
	if measured.Asked && createsARecord(verdict) && measured.Confidence < verdictCreateFloor {
		return 0, fmt.Errorf(
			"verdict: refusing to create a %s record below the create floor", verdict)
	}
	done, err := e.apply(ctx, row, verdict, false, measured)
	if err != nil {
		return 0, err
	}
	if done {
		return 1, nil
	}
	return 0, nil
}

// lastMeasurement is what the model actually said, taken from the re-ask when
// it produced an answer and from the first ask otherwise.
//
// A retirement records the LAST opinion rather than the first, because that is
// the one the floor rejected. Either ask can come back empty — a malformed
// reply is dropped by the validator before it reaches here — so the fallback is
// not decoration: without it a retirement after a failed re-ask would record
// nothing, which reads as "no model was asked" when two were.
func lastMeasurement(
	retry []verdictResult, retryModel string, first []verdictResult, firstModel string,
) capture.VerdictMeasurement {
	if len(retry) == 1 {
		return capture.MeasuredVerdict(float64(retry[0].Confidence), retryModel)
	}
	if len(first) == 1 {
		return capture.MeasuredVerdict(float64(first[0].Confidence), firstModel)
	}
	return capture.VerdictMeasurement{}
}

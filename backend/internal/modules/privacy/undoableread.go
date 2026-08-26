// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Whether each history entry can be put back. The ANSWER is computed by the
// reversal seam — it needs the record's update shapes and its own table, which
// this module may not reach — so this file is the question's shape and the
// attachment, and nothing else.
//
// recordhistory.go is at its file ceiling, which is why this is its own file
// rather than three more functions there.

import (
	"context"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// UndoabilityAnswer is the evaluator's verdict for one audit row.
type UndoabilityAnswer struct {
	Undoable bool
	// Reason is empty exactly when Undoable.
	Reason string
	Detail string
}

// UndoabilityReader answers a whole page at once. Flat in page size is the
// contract, not an optimisation: a per-entry lookup produces a button whose
// state is unknown until the user interacts with it, which is the greyed-
// button-with-no-reason shape this feature exists to remove.
//
// It takes audit row IDS and reads the rows itself. Handing it the images this
// module already holds would save a query and create two readers of one row —
// and the two projections here render those images differently, so the
// evaluator would be judging a shape that depends on which screen asked.
type UndoabilityReader interface {
	ForRecord(ctx context.Context, entityType string, entityID ids.UUID, auditIDs []ids.UUID) (map[ids.UUID]UndoabilityAnswer, error)
}

// WithUndoabilityReader wires the evaluator. Without one the history reads
// answer undoable=false with no reason, which is what an install that cannot
// evaluate the question honestly knows.
func (h Handlers) WithUndoabilityReader(reader UndoabilityReader) Handlers {
	h.undoability = reader
	return h
}

// undoabilityFor judges the distinct audit rows a page touches. Undoability is
// a property of the audit ROW, so a field-history page whose entries share an
// id asks about it once and they all read the same answer.
//
// A reader that fails does NOT fail the page: a history a person can read is
// more useful than an error, and every entry falls back to the answer an
// unevaluated entry honestly has.
func (h Handlers) undoabilityFor(ctx context.Context, entityType string, entityID ids.UUID, auditIDs []ids.UUID) map[ids.UUID]UndoabilityAnswer {
	if h.undoability == nil || len(auditIDs) == 0 {
		return nil
	}
	seen := make(map[ids.UUID]bool, len(auditIDs))
	distinct := make([]ids.UUID, 0, len(auditIDs))
	for _, id := range auditIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		distinct = append(distinct, id)
	}
	answers, err := h.undoability.ForRecord(ctx, entityType, entityID, distinct)
	if err != nil {
		return nil
	}
	return answers
}

// undoabilityToWire renders one answer. An entry nobody judged is undoable=false
// with no reason — the honest shape for "not evaluated", and the one the
// surface already renders as a plain disabled entry.
func undoabilityToWire(answer UndoabilityAnswer, judged bool) *crmcontracts.Undoability {
	if !judged {
		return &crmcontracts.Undoability{Undoable: false}
	}
	out := &crmcontracts.Undoability{Undoable: answer.Undoable}
	if answer.Reason != "" {
		reason := crmcontracts.UndoabilityReason(answer.Reason)
		out.Reason = &reason
	}
	if answer.Detail != "" {
		detail := answer.Detail
		out.Detail = &detail
	}
	return out
}

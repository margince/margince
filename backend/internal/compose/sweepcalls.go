// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the three batched sweeps — capture labels, owed verdicts, request
// settlement — share about paying for model calls: one bound on the calls a pass
// makes, and one answer to "did the models decline this message".

import (
	"errors"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// sweepCalls bounds one pass by the MODEL CALLS it makes, solo re-asks and a
// declined batch's split included. A bound on batches read or verdicts written
// does not end a pass whose batches keep splitting: one message the models
// decline turns its batch into a call per message, and a batch of abstentions
// writes nothing and is read again.
type sweepCalls struct{ left int }

// newSweepCalls allows each batch the verdict cap needs one call of its own and
// one re-ask on average, and one call more so a small cap still makes progress.
func newSweepCalls(maxVerdicts, batchSize int) *sweepCalls {
	return &sweepCalls{left: 2*(maxVerdicts/batchSize) + 1}
}

// take spends one call, reporting false once the pass has none left.
func (c *sweepCalls) take() bool {
	if c.left <= 0 {
		return false
	}
	c.left--
	return true
}

// remain reports whether the pass may make another call.
func (c *sweepCalls) remain() bool { return c.left > 0 }

// declinedByTheModels reports whether every rung declined to answer about the
// message itself, which is that message's outcome and recorded as such. The
// offline stand-in is excluded: it declines every message because no provider
// is bound, and recording that would strand the whole backlog for the day one
// is.
func declinedByTheModels(err error) bool {
	return ai.ModelDeclined(err) && !errors.Is(err, ai.ErrUnconfiguredModel)
}

// sweepPaused reports whether err ends a pass cleanly rather than failing it:
// the budget band that defers background work, or no provider bound at all.
// Either way what is committed stands and the rest waits for a later cycle.
func sweepPaused(err error) bool {
	return errors.Is(err, ai.ErrBudgetDeferred) || errors.Is(err, ai.ErrUnconfiguredModel)
}

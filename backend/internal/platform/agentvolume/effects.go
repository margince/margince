// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import "context"

// One call is one act, which is what the write counter has always been charged
// per call. A bulk change is the exception: it changes many records in one call,
// and charging it as one write would let an agent change five hundred records
// for the price of one. So a call that changes records in bulk SAYS how many,
// and the surface charges that number instead.

type effectKey struct{}

type effectTally struct {
	records int
	noted   bool
}

// WithEffectTally opens the place one call writes how many records it changed.
// Each door opens it around the handler it runs and reads it back with
// ActsCharged once the handler has answered.
func WithEffectTally(ctx context.Context) context.Context {
	return context.WithValue(ctx, effectKey{}, &effectTally{})
}

// NoteEffects records that this call changed n records. It is a no-op outside a
// tally, which is every call a human makes.
func NoteEffects(ctx context.Context, n int) {
	if tally, ok := ctx.Value(effectKey{}).(*effectTally); ok {
		tally.records, tally.noted = n, true
	}
}

// ActsCharged answers what this call costs on the counter its act is charged
// against: the records it said it changed, or one act when it said nothing.
func ActsCharged(ctx context.Context) int {
	if tally, ok := ctx.Value(effectKey{}).(*effectTally); ok && tally.noted {
		return tally.records
	}
	return 1
}

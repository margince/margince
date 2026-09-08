// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether a request that answered a refusal changed anything.
//
// A refusal normally means nothing happened, and every layer above a handler
// reads it that way — the idempotency middleware most of all, which gives a
// key back so the caller may retry it. That is right for a request refused
// before it wrote, and wrong for one refused after: the retry runs the applied
// half a second time under the same key, which is exactly what the key exists
// to prevent.
//
// One door reaches that state today. The per-field split applies the
// agent-owned fields and then stages the human-owned residue, and the staging
// call runs after the first half has committed — so a failure there answers a
// refusal for a request that already wrote. Everything the split can refuse on
// BEFORE the write is settled before the write (agentsplit.go); this is the
// remainder, which is a genuine failure rather than a refusal.
//
// The signal travels DOWN as a pointer and is read on the way back up, because
// a context value set by a handler is invisible to the middleware that
// dispatched it — the request's context is the one the middleware built.

import "context"

// writeEffect records whether the handler under it committed anything.
//
// A plain bool behind a pointer, with no mutex: a request is served on one
// goroutine, and a handler that fanned out and wrote from another would be
// outside every other assumption on this path too.
type writeEffect struct{ committed bool }

type writeEffectKey struct{}

// withWriteEffect gives a request a place for its handler to say that it wrote.
// Called by the layer that will read the answer, so a door with nobody above it
// to care simply has no marker and costs nothing.
func withWriteEffect(ctx context.Context) (context.Context, *writeEffect) {
	effect := &writeEffect{}
	return context.WithValue(ctx, writeEffectKey{}, effect), effect
}

// markWriteCommitted records that this request has already changed the record,
// whatever it answers next.
//
// Silent when nothing is listening: the marker exists only under a layer that
// asked for one, and a handler that reports its own writes honestly should not
// have to know which of its callers did.
func markWriteCommitted(ctx context.Context) {
	if effect, ok := ctx.Value(writeEffectKey{}).(*writeEffect); ok {
		effect.committed = true
	}
}

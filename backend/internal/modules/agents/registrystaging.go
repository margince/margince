// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What the registry can say about a verb's STAGING, kept together and apart
// from the registry's own bookkeeping.
//
// These answer one question in two shapes — can this verb put a call in front
// of a human, and what is the thing that would do it — and both are load
// bearing now that the consequential verbs execute by default: an installation
// tightens one back to confirm-first with a tier floor, and a verb that could
// not then stage would be advertised and unredeemable.

import (
	"context"
	"encoding/json"
)

// Stager is a tool that can describe the call it would put in front of a human.
// Named so callers outside this package can hold one — the composition root's
// two-door gate compares the SUBJECT each door would stage, which it can no
// longer obtain by invoking the verb and reading what got recorded.
type Stager interface {
	StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error)
}

// Stageable reports whether a refused 🟡 call on this verb has somewhere to
// land. Exported for the composition root's gate: a tier the contract tightens
// onto a verb that cannot stage turns an approval question into a dead end, so
// the gate has to be able to ask this rather than assume it.
func (r *Registry) Stageable(name string) bool {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	_, stageable := t.(stageableTool)
	return stageable
}

// StagerFor answers the tool that describes this verb's staging, or false when
// it describes none. Exported alongside Stageable because the composition
// root's two-door gate needs the SUBJECT, not merely the fact that one exists:
// the REST and tool doors must put the same sentence in front of a human, and
// since these verbs execute directly the comparison can no longer be made by
// invoking them and reading what got staged.
//
//nolint:ireturn // the interface IS the answer: a caller outside this package needs the staging behaviour, and the concrete tool type behind it is deliberately not exported
func (r *Registry) StagerFor(name string) (Stager, bool) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	stager, stageable := t.(stageableTool)
	return stager, stageable
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the rest of the tree may ask about the set of stageable kinds.
//
// The grant maps in authority.go are this package's own business — which
// grants a decision needs is nobody else's to read. Which kinds EXIST is a
// different question, and two callers outside this module ask it: the
// composition census, checking that every 🟡 tool has somebody mapped to
// decide it, and the frontend label gate, checking that every kind a reader
// can meet has words. Both derive from the maps rather than restating them,
// which is the whole point — a list kept beside its subject drifts, and the
// drift shows up as a wire enum in a German queue.

package approvals

import "slices"

// KindHasDecisionGrants reports whether a stageable kind carries a
// decision-grant mapping — its own, or one resolved from the staged target's
// entity type. The composition layer's fitness test calls it for every
// 🟡/dynamic tool in the registry: a tool that can stage an approval nobody is
// mapped to decide would strand its stagings in a queue no inbox shows
// (decidable fails closed on unknown kinds).
func KindHasDecisionGrants(kind string) bool {
	if _, ok := decisionGrants[kind]; ok {
		return true
	}
	_, resolvedFromTarget := targetResolvedGrants[kind]
	return resolvedFromTarget
}

// StageableKinds lists the kinds a proposal can be staged under, sorted.
//
// Derived from the same two maps KindHasDecisionGrants reads, so a kind added
// to either is in this list the moment it can be staged.
//
// Held by: TestEveryStageableKindHasAFrontendLabel
// (backend/gates/frontendapprovalkinds_test.go), which fails in both directions
// against the frontend's label map — a kind this list gains with no label, and
// a label naming a kind this list does not carry.
func StageableKinds() []string {
	kinds := make([]string, 0, len(decisionGrants)+len(targetResolvedGrants))
	for kind := range decisionGrants {
		kinds = append(kinds, kind)
	}
	for kind := range targetResolvedGrants {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return slices.Compact(kinds)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// WHICH rule answers for a staged target type, kept apart from the rules
// themselves.
//
// Both questions below now have two sources — the static classification this
// module writes, and the units a composed extension set registers at boot
// (extensionkinds.go) — and the ORDER between them is the whole content of
// this file. Static first, in both, so a unit can never reclassify a core
// target or add itself to the vocabulary under a core name. That is the second
// of two reasons rather than the only one: RegisterExtensionKinds refuses a
// core name outright, and a rule that rests on one guard is a rule that rests
// on nobody noticing when it moves.

import (
	"maps"
	"slices"
)

// probeFor classifies one staged target type. An unlisted type answers
// probeNoRule (the zero value) and fails closed.
//
// A unit's table is existence-probed, for the reason every workspace-shared
// target is: the unit's store applies no row scope of its own, so the row half
// is existence and the authority is the object-read floor plus the decision
// grants. Consulted after the static table, so a unit that named a core type
// could not reclassify it — and RegisterExtensionKinds refuses that name
// outright, so this ordering is the second of two reasons rather than the only
// one.
func probeFor(targetType string) targetProbe {
	if probe, classified := targetProbes[targetType]; classified {
		return probe
	}
	if _, registered := extensionTarget(targetType); registered {
		return probeExistence
	}
	return probeNoRule
}

// ClassifiedTargetTypes returns every staged target type this package has a
// visibility rule for, sorted.
//
// Exported for the composition layer's parity gate against the webhook fan-out's
// own classification of the same vocabulary. That gate cannot take its subject
// set from the contract's agent-policy table alone: a target type staged by a
// server-side proposal flow rather than by an agent's call — an effective-dated
// rate sheet is one — appears in no agent policy at all, so a gate reading only
// that table reads green over exactly the drift it exists to catch.
func ClassifiedTargetTypes() []string {
	// Both halves, because the parity gate asks what this module can decide
	// about — and a vocabulary that reported only the static half would read
	// green over a unit's staged target the fan-out has never classified.
	types := append(slices.Collect(maps.Keys(targetProbes)), extensionTargetTypes()...)
	slices.Sort(types)
	return types
}

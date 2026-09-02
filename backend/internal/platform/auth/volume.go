// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The quantitative third of "scope ∧ tier ∧ volume" (interfaces.md §2), and the
// §2.4 ladder's refusing half.
//
// The first two terms are BOOLEAN — may this caller run this verb at all — and
// they are the only ones that catch an agent doing something it was never
// allowed to do. This one is the only one that catches an agent doing something
// it IS allowed to do, at a volume nobody intended (api-rate-limits §2.5). It is
// why an in-scope, correctly-tiered, read-only Passport reading the whole
// workspace is a gated event rather than a Tuesday.
//
// This half only ever REFUSES. Nothing here charges a counter: the surface pays
// where records and effects leave it (modules/agents), and a gate that could
// both admit and charge could admit itself.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// VolumeMeter answers what the calling agent has spent this window on one counter. It
// is an interface rather than the concrete meter so this package — the one
// admission point — stays testable without a Redis client, and so a deployment
// that has composed no bound is a visible nil rather than a meter that silently
// answers "plenty".
type VolumeMeter interface {
	Read(ctx context.Context, c agentvolume.Counter) agentvolume.Reading
}

// VolumeExceededError is a volume refusal, carrying the reading it was made from.
//
// It is a TYPE rather than a message because the two halves of the ladder are
// told apart by it: a refusal on a releasable counter has somewhere to go — the
// surface stages the question for the human who lent the Passport — and one on a
// hard stop does not. A caller matching on prose would eventually stage a
// release for a counter nothing can release.
//
// It unwraps to the sentinel interfaces.md §0 reserves for MCP-SESS-*, so every
// existing errors.Is check and every transport that maps sentinels to wire codes
// keeps working without learning this type.
type VolumeExceededError struct {
	// Tool is the call that was refused. Empty on the REST read path, which has
	// no tool spec to name.
	Tool string
	// Reading is the window as the meter read it: the counter, what was
	// observed, the effective limit, and the window a release must name.
	Reading agentvolume.Reading
}

// Releasable reports whether a human can answer this refusal — the difference
// between BYO-STEP-1/2 (step-up and batch-confirm) and BYO-STEP-3/4 (hard stop
// and suspension).
func (e *VolumeExceededError) Releasable() bool { return e.Reading.Counter.Releasable() }

// Unwrap makes every existing budget check see this as what it is.
func (e *VolumeExceededError) Unwrap() error { return apperrors.ErrBudgetExceeded }

// Error states the numbers and what ends the refusal, because those are two
// different things per rung and an agent's next move depends on which it got.
//
// A releasable refusal says a release is POSSIBLE, not that one has been asked
// for. This type is answered on both doors, and only the MCP one stages the
// question (StepUpStagedError says so, and is what an agent reads there); the
// REST door has no tool to name and stages nothing. Promising here that
// somebody is looking at it would leave a REST caller waiting on an approval
// that was never created.
func (e *VolumeExceededError) Error() string {
	what := "this agent"
	if e.Tool != "" {
		what = e.Tool
	}
	if e.Releasable() {
		return fmt.Sprintf(
			"%s: this agent has spent %d of its %d %s for this window; it may continue once the person who connected it releases the window, or when the window rolls",
			what, e.Reading.Observed, e.Reading.Limit, e.Reading.Counter)
	}
	return fmt.Sprintf(
		"%s: this agent has spent %d of its %d %s for this window, and that limit holds until the window rolls; no approval lifts it",
		what, e.Reading.Observed, e.Reading.Limit, e.Reading.Counter)
}

// AdmitReplay is the admission a RECORDED answer takes before it is served
// again. It is the volume half of Admit and nothing else, and each omission is
// deliberate rather than an economy:
//
//   - SCOPE and TIER are not re-asked because a replay makes no new call. Worse,
//     asking would break the thing it is meant to protect: a confirm-first tool
//     answers ErrRequiresApproval to every Admit, so a full admission would
//     refuse to re-serve the receipt of an act a human already approved.
//   - The SEAT and the granting human's RBAC bind where they always did — the
//     records are re-read live through the datasource seam, which is what
//     applies object grants and row scope.
//   - REVOCATION binds one layer up: the transport re-authenticates the
//     passport on every exchange.
//
// What is left is the part nothing else re-asks: the ceilings. A passport past
// its call ceiling is refused for every verb, and a receipt is a verb — without
// this, a suspended caller could keep drawing record documents out of answers it
// produced before the ceiling closed.
func (g *Gate) AdmitReplay(ctx context.Context, spec mcp.ToolSpec) error {
	if g == nil {
		return nil
	}
	return g.refuseOnVolume(ctx, spec)
}

// refuseOnVolume applies every volume bound one tool call is subject to, and
// answers the first one it crosses.
//
// CALLS IS ASKED FIRST, and the order is a decision. It is the ceiling every
// other counter sits under (BYO-STEP-4's suspension), so a Passport that has
// crossed it is refused for every verb — and answering the per-kind counter
// instead would tell a suspended caller which of its allowances still has
// headroom, which is a map of what to spend next.
//
// Then the counter the call itself belongs to, DERIVED from the spec rather
// than listed, by the same function the charge point uses (agentvolume.CounterFor).
// One derivation means the counter that refuses and the counter that is paid can
// never be two different counters.
func (g *Gate) refuseOnVolume(ctx context.Context, spec mcp.ToolSpec) error {
	if g.volume == nil {
		return nil
	}
	for _, c := range []agentvolume.Counter{agentvolume.Calls, agentvolume.CounterFor(spec)} {
		reading := g.volume.Read(ctx, c)
		if reading.Exceeded {
			return &VolumeExceededError{Tool: spec.Name, Reading: reading}
		}
	}
	return nil
}

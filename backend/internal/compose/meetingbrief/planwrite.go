// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Running the plan's model lane, and falling back field by field.
//
// The floor is not a consolation prize here, it is the coverage guarantee: a
// model that answers three fields well and omits the rest must not produce a
// SHORTER plan than a deployment with no model at all. So anything the reply
// left out is restored from the deterministic plan, exactly as the sections
// restore theirs — the model can change how the plan READS and never what it
// COVERS.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// WritePlan produces the plan. lane may be nil, which is not an error: it is
// the deployment saying this role runs no model.
func WritePlan(
	ctx context.Context, lane Completer, in Input, floor Plan, lang string,
) (Plan, crmcontracts.WrittenBy) {
	if lane == nil {
		return floor, crmcontracts.Deterministic
	}
	written, err := writePlanWithModel(ctx, lane, in, floor, lang)
	if err != nil {
		// The declared degrade posture, not a swallowed error. A model that is
		// unavailable, over budget or answering unparseable JSON must not take
		// the preparation down with it: the reader gets the floor, and
		// generated_by tells them which of the two they are reading.
		return floor, crmcontracts.Deterministic
	}
	return withPlanFloor(written, floor), crmcontracts.Model
}

func writePlanWithModel(
	ctx context.Context, lane Completer, in Input, floor Plan, lang string,
) (Plan, error) {
	reply, err := lane.Complete(ctx, PlanRequest(planPromptOf(in, floor), lang))
	if err != nil {
		return Plan{}, err
	}
	return ParsePlan(reply.Text, in, floor)
}

// withPlanFloor puts back every field the model did not answer.
func withPlanFloor(written, floor Plan) Plan {
	merged := written
	// Type and unknowns are already the floor's — ParsePlan refuses to read
	// either from a reply — and the arc and the advance are not asked for at
	// all: one is a record of what happened and the other is a fixed ladder,
	// and neither reads better for being rephrased.
	merged.Type = floor.Type
	merged.Unknowns = floor.Unknowns
	merged.Arc = floor.Arc
	merged.Advance = floor.Advance
	if merged.Objective == nil {
		merged.Objective = floor.Objective
	}
	if merged.Opening == nil {
		merged.Opening = floor.Opening
	}
	if merged.TopRisk == nil {
		merged.TopRisk = floor.TopRisk
	}
	if len(merged.LikelyAsks) == 0 {
		merged.LikelyAsks = floor.LikelyAsks
	}
	if len(merged.Questions) == 0 {
		merged.Questions = floor.Questions
	}
	if len(merged.Scenarios) == 0 {
		merged.Scenarios = floor.Scenarios
	}
	return merged
}

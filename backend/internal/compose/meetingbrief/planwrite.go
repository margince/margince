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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	reply, err := lane.Complete(principal.WithWorkSubject(ctx, in.Company), PlanRequest(planPromptOf(in, floor), lang))
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
	// A list is TOPPED UP from the floor, not merely replaced when empty.
	//
	// Restoring only an empty list read as coverage and was not: a floor with
	// five questions and a model that answered one left the reader with one,
	// which is a shorter plan than the same deployment would have produced
	// with no model at all. The model's own entries lead — they are the
	// better-written ones — and the floor's fill the rest of the room.
	merged.LikelyAsks = topUpAsks(merged.LikelyAsks, floor.LikelyAsks, askCap)
	merged.Questions = topUpQuestions(merged.Questions, floor.Questions, questionCap)
	merged.Scenarios = topUpScenarios(merged.Scenarios, floor.Scenarios, scenarioCap)
	return merged
}

// topUpAsks appends the floor's asks the model did not already cover, up to the
// cap. Sameness is the QUESTION: two hypotheses about the same buyer question
// are one hypothesis however differently they are worded.
func topUpAsks(written, floor []Ask, bound int) []Ask {
	seen := map[string]bool{}
	for _, ask := range written {
		seen[normalisedSubject(ask.Question)] = true
	}
	for _, ask := range floor {
		if len(written) >= bound {
			break
		}
		if seen[normalisedSubject(ask.Question)] {
			continue
		}
		written = append(written, ask)
	}
	return written
}

func topUpQuestions(written, floor []Question, bound int) []Question {
	seen := map[string]bool{}
	for _, question := range written {
		seen[normalisedSubject(question.Ask)] = true
	}
	for _, question := range floor {
		if len(written) >= bound {
			break
		}
		if seen[normalisedSubject(question.Ask)] {
			continue
		}
		written = append(written, question)
	}
	return written
}

func topUpScenarios(written, floor []Scenario, bound int) []Scenario {
	seen := map[string]bool{}
	for _, scenario := range written {
		seen[normalisedSubject(scenario.Label)] = true
	}
	for _, scenario := range floor {
		if len(written) >= bound {
			break
		}
		if seen[normalisedSubject(scenario.Label)] {
			continue
		}
		written = append(written, scenario)
	}
	return written
}

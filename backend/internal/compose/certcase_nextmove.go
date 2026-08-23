// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for deal_health/next_move — the deal card's one
// concrete task.
//
// It certifies the shipped path: the request is built by nextaction's own
// writer and the reply is read by nextaction's own filter, because that
// filter is what stands between a rep and a task grounded in nothing. A case
// that rebuilt either would measure a copy, and a copy stays green through
// the change that breaks the original.
//
// What the expectation MEANS here: the timeline entries a correct proposal
// has to rest on. Not the wording — the subject is prose, and pinning it
// would fail a good task for choosing different words. What production cannot
// guarantee, and this therefore measures, is whether the model proposed the
// move the timeline points at and cited it, rather than a generic checklist
// item nobody can check.
//
// The fixture names its records by LABEL. Prepare mints the ids, so an id in
// the reply is an id the model was handed rather than one the corpus author
// could have written into the expected answer.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gradionhq/margince/backend/internal/compose/aitasks"
	"github.com/gradionhq/margince/backend/internal/compose/nextaction"
	"github.com/gradionhq/margince/backend/internal/modules/ai"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// nextMoveFixture is one deal and its timeline as the lane reads them.
type nextMoveFixture struct {
	Deal     nextMoveDealFixture  `json:"deal"`
	Timeline []nextMoveActFixture `json:"timeline"`
}

type nextMoveDealFixture struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Amount        string `json:"amount"`
	ExpectedClose string `json:"expected_close"`
}

type nextMoveActFixture struct {
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	Subject   string `json:"subject"`
	At        string `json:"at"`
	Excerpt   string `json:"excerpt"`
}

type nextMoveCases struct{}

func (nextMoveCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskDealHealth, Variant: "next_move", Kind: ai.SiteKindOneShot}
}

// Prepare turns one deal, its timeline and the entries a correct proposal
// must cite into a runnable case, minting an id per labelled record.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (nextMoveCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f nextMoveFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("deal_health/next_move: the fixture is not the shape this site takes: %w", err)
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"deal_health/next_move: the expected answer is not a list of timeline labels the proposal must cite: %w", err)
	}
	in, label, err := nextMoveInput(f)
	if err != nil {
		return nil, fmt.Errorf("deal_health/next_move: %w", err)
	}
	// An empty-timeline scenario expects no citation — the deal's own fields
	// are the grounding there, and the filter owes none — so the ungroundable
	// refusal applies only when the fixture carries a timeline.
	if len(f.Timeline) > 0 {
		if err := refuseUngroundableBrief(want, label); err != nil {
			return nil, fmt.Errorf("deal_health/next_move: %w", err)
		}
	}
	return &nextMoveCase{in: in, label: label, expected: want}, nil
}

// nextMoveInput builds the production input, minting one id per labelled
// entry so no id in the reply can have come from the corpus.
func nextMoveInput(f nextMoveFixture) (nextaction.Input, map[string]string, error) {
	in := nextaction.Input{Deal: nextaction.DealIn{
		ID:   ids.NewV7().String(),
		Name: f.Deal.Name, Status: f.Deal.Status,
		Amount: f.Deal.Amount, ExpectedClose: f.Deal.ExpectedClose,
	}}
	label := map[string]string{}
	for _, act := range f.Timeline {
		if err := refuseUnnameable(act.Label, "timeline entry", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[act.Label] = id
		in.Timeline = append(in.Timeline, nextaction.ActIn{
			ID: id, Kind: act.Kind, Direction: act.Direction,
			Subject: act.Subject, At: act.At, Excerpt: act.Excerpt,
		})
	}
	return in, label, nil
}

// nextMoveCase certifies one proposed task for one deal.
type nextMoveCase struct {
	in       nextaction.Input
	label    map[string]string
	expected []string
}

// Run issues the one request this site sends, through the production
// writer's own request builder.
func (c *nextMoveCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := nextaction.NextMoveRequest(c.in)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("deal_health/next_move: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production filter and asks whether the surviving
// proposal cites the timeline entries the scenario says the move rests on.
func (c *nextMoveCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	move, err := nextaction.ParseNextMove(trace.Output, c.in)
	if err != nil {
		// The filter refuses for exactly the reasons production would serve
		// the deterministic fallback: unparseable, out of bounds, an id in
		// reader text, or a citation pointing outside the timeline.
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	cited := map[string]bool{}
	for _, id := range move.Evidence {
		cited[id] = true
	}
	var missing []string
	for _, name := range c.expected {
		if !cited[c.label[name]] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: "never cited: " + strings.Join(missing, ", "),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for deal_health/deal_status — the deal page's one
// written card.
//
// It certifies the shipped path: the request is built by dealstatus's own
// writer and the reply is read by dealstatus's own filter, because that filter
// is what stands between a reader and a status grounded in nothing. A case
// that rebuilt either would measure a copy, and a copy stays green through the
// change that breaks the original.
//
// What the expectation MEANS here: the timeline entries a correct card has to
// rest on. Not the wording — the sentences are prose, and pinning them would
// fail a good card for choosing different words. What production cannot
// guarantee, and this therefore measures, is whether the model read the facts
// that matter and cited them, rather than writing a plausible paragraph about
// a deal it did not look at.
//
// The fixture names its records by LABEL. Prepare mints the ids, so an id in
// the reply is an id the model was handed rather than one the corpus author
// could have written into the expected answer.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// dealStatusFixture is one deal and its timeline as the lane reads them.
type dealStatusFixture struct {
	Deal     dealStatusDealFixture  `json:"deal"`
	Timeline []dealStatusActFixture `json:"timeline"`
	// Move is the verb the rules chose. It rides in the fixture because the
	// model explains the move rather than choosing one, so a case that omitted
	// it would certify the lane against a prompt production never sends.
	Move string `json:"move"`
	// OpenTasks is work nobody has done yet, which production always sends
	// alongside the timeline. A fixture that omitted it could not certify the
	// rule that matters most here — that an overdue task is a promise still
	// owed rather than evidence the work happened.
	OpenTasks []dealStatusTaskFixture `json:"open_tasks"`
}

type dealStatusTaskFixture struct {
	Label   string `json:"label"`
	Subject string `json:"subject"`
	Due     string `json:"due"`
	// State is "open" or "overdue"; empty reads as open, the ordinary case.
	State string `json:"state"`
}

type dealStatusDealFixture struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Amount        string `json:"amount"`
	ExpectedClose string `json:"expected_close"`
}

type dealStatusActFixture struct {
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	Subject   string `json:"subject"`
	At        string `json:"at"`
	// When is "past" or "scheduled". Production always sets it, so a fixture
	// that left it blank would send the model a prompt production never sends
	// — and the case that certifies a booked meeting is not read as one that
	// already happened would score green with the field it tests absent.
	When    string `json:"when"`
	Excerpt string `json:"excerpt"`
}

type dealStatusCases struct{}

func (dealStatusCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskDealHealth, Variant: "deal_status", Kind: ai.SiteKindOneShot}
}

// Prepare turns one deal, its timeline and the entries a correct card must
// cite into a runnable case, minting an id per labelled record.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (dealStatusCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f dealStatusFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("deal_health/deal_status: the fixture is not the shape this site takes: %w", err)
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"deal_health/deal_status: the expected answer is not a list of timeline labels the card must cite: %w", err)
	}
	in, label, err := dealStatusInput(f)
	if err != nil {
		return nil, fmt.Errorf("deal_health/deal_status: %w", err)
	}
	if err := refuseUngroundableBrief(want, label); err != nil {
		return nil, fmt.Errorf("deal_health/deal_status: %w", err)
	}
	return &dealStatusCase{in: in, label: label, expected: want}, nil
}

// dealStatusInput builds the production input, minting one id per labelled
// entry so no id in the reply can have come from the corpus.
func dealStatusInput(f dealStatusFixture) (dealstatus.StatusInput, map[string]string, error) {
	in := dealstatus.StatusInput{
		Deal: dealstatus.DealIn{
			ID:   ids.NewV7().String(),
			Name: f.Deal.Name, Status: f.Deal.Status,
			Amount: f.Deal.Amount, ExpectedClose: f.Deal.ExpectedClose,
		},
		RecommendedMove: f.Move,
	}
	label := map[string]string{}
	for _, act := range f.Timeline {
		if err := refuseUnnameable(act.Label, "timeline entry", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[act.Label] = id
		when := act.When
		if when == "" {
			when = "past"
		}
		in.Timeline = append(in.Timeline, dealstatus.ActIn{
			ID: id, Kind: act.Kind, Direction: act.Direction,
			Subject: act.Subject, At: act.At, When: when, Excerpt: act.Excerpt,
		})
	}
	for _, task := range f.OpenTasks {
		if err := refuseUnnameable(task.Label, "open task", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[task.Label] = id
		state := task.State
		if state == "" {
			state = dealstatus.TaskStateOpen
		}
		in.OpenTasks = append(in.OpenTasks, dealstatus.TaskIn{
			ID: id, Subject: task.Subject, Due: task.Due, State: state,
		})
	}
	return in, label, nil
}

// dealStatusCase certifies one written card for one deal.
type dealStatusCase struct {
	in       dealstatus.StatusInput
	label    map[string]string
	expected []string
}

// Run issues the one request this site sends, through the production writer's
// own request builder.
func (c *dealStatusCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	// English, pinned, rather than the installation's base language: a
	// certification record grades a fixed corpus, and a score that moved with a
	// settings row would not be comparable between two installations or across
	// one that changed its mind. The rule is PRESENT in the graded request for
	// the same reason — production sends one, so a case that left it out would
	// grade a prompt the product does not send.
	req := dealstatus.StatusRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("deal_health/deal_status: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production filter and asks whether the surviving card
// cites the timeline entries the scenario says it rests on.
func (c *dealStatusCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	written, err := dealstatus.ParseStatus(trace.Output, c.in)
	if err != nil {
		// The filter refuses for exactly the reasons production would compose
		// the deterministic card: unparseable, out of bounds, an id in reader
		// text, or a card saying nothing about where the deal stands.
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	cited := map[string]bool{}
	// Every cited section counts. A scenario's expected record may be what the
	// verdict rests on rather than what the story names, and a measurement that
	// read only some sections would call that card wrong for citing it well.
	for _, section := range [][]dealstatus.WrittenLine{
		written.Story, written.Blocker, written.Buyer, written.Verdict.Because,
	} {
		for _, line := range section {
			for _, id := range line.Evidence {
				cited[id] = true
			}
		}
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

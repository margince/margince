// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The sales assessment's certification case. A scenario names the cells a
// right reading must cite and the deal its decision must name, or "none".
// Run calls salesassessment.Request and Evaluate calls salesassessment.Parse,
// so a change to either is a change to what is certified.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/salesassessment"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const salesAssessmentSite = "report_interpret/sales_assessment"

// salesAssessmentFixture is one assessment's input with labels in place of
// ids: the case mints the ids, so no scenario carries one.
type salesAssessmentFixture struct {
	Frame      salesassessment.FrameIn      `json:"frame"`
	Cells      []salesAssessmentCellFixture `json:"cells"`
	Candidates []salesAssessmentDealFixture `json:"candidate_deals"`
	Decisions  []salesAssessmentDecisionFix `json:"decisions"`
}

type salesAssessmentCellFixture struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Unit     string `json:"unit"`
	Window   string `json:"window"`
	Coverage string `json:"coverage"`
}

type salesAssessmentDealFixture struct {
	Label  string `json:"label"`
	Name   string `json:"name"`
	Amount string `json:"amount"`
	Stage  string `json:"stage"`
}

type salesAssessmentDecisionFix struct {
	Deal     string `json:"deal"`
	Text     string `json:"text"`
	ReviewOn string `json:"review_on"`
	Status   string `json:"status"`
	Result   string `json:"result"`
}

// salesAssessmentExpectation names the cells a right reading cites and the
// deal its next decision names, or "none" when no decision is supported.
type salesAssessmentExpectation struct {
	Cites        []string `json:"cites"`
	NextDecision string   `json:"next_decision"`
}

type salesAssessmentCases struct{}

func (salesAssessmentCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskReportInterpret, Variant: "sales_assessment", Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no reply could pass: an expected cell or deal
// the fixture does not carry.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (salesAssessmentCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f salesAssessmentFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", salesAssessmentSite, err)
	}
	var want salesAssessmentExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected value is not {cites, next_decision}: %w", salesAssessmentSite, err)
	}
	if len(f.Cells) == 0 {
		return nil, fmt.Errorf("%s: the fixture needs at least one cell, as production never asks without one", salesAssessmentSite)
	}
	in, labels, err := salesAssessmentInput(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", salesAssessmentSite, err)
	}
	for _, label := range want.Cites {
		if _, ok := labels[label]; !ok {
			return nil, fmt.Errorf("%s: the expected citation %q names no cell of the fixture", salesAssessmentSite, label)
		}
	}
	if want.NextDecision != "none" {
		if _, ok := labels[want.NextDecision]; !ok {
			return nil, fmt.Errorf("%s: the expected decision names %q, which is no candidate deal of the fixture", salesAssessmentSite, want.NextDecision)
		}
	}
	return &salesAssessmentCase{in: in, labels: labels, expected: want}, nil
}

func salesAssessmentInput(f salesAssessmentFixture) (salesassessment.Input, map[string]string, error) {
	in := salesassessment.Input{Frame: f.Frame}
	labels := map[string]string{}
	for _, cell := range f.Cells {
		if err := refuseUnnameable(cell.Label, "cell", labels); err != nil {
			return in, nil, err
		}
		id := "metric:" + cell.Label
		labels[cell.Label] = id
		in.Cells = append(in.Cells, salesassessment.CellIn{ID: id, Label: cell.Label, Value: cell.Value, Unit: cell.Unit, Window: cell.Window, Coverage: cell.Coverage})
	}
	owners := map[string]string{}
	for _, deal := range f.Candidates {
		if err := refuseUnnameable(deal.Label, "candidate deal", labels); err != nil {
			return in, nil, err
		}
		id, owner := ids.NewV7().String(), ids.NewV7().String()
		labels[deal.Label], owners[deal.Label] = id, owner
		in.Candidates = append(in.Candidates, salesassessment.CandidateIn{ID: id, Name: deal.Name, Amount: deal.Amount, Stage: deal.Stage, OwnerID: owner})
	}
	for _, decision := range f.Decisions {
		deal, ok := labels[decision.Deal]
		if !ok {
			return in, nil, fmt.Errorf("a decision names %q, which is no candidate deal", decision.Deal)
		}
		in.Decisions = append(in.Decisions, salesassessment.DecisionIn{ID: ids.NewV7().String(), Text: decision.Text, Deal: deal, OwnerID: owners[decision.Deal], ReviewOn: decision.ReviewOn, Status: decision.Status, Result: decision.Result})
	}
	return in, labels, nil
}

type salesAssessmentCase struct {
	in       salesassessment.Input
	labels   map[string]string
	expected salesAssessmentExpectation
}

func (c *salesAssessmentCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := salesassessment.Request(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", salesAssessmentSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate reads the reply through the production parser and asks whether
// what survived cites the expected cells and names the expected deal.
func (c *salesAssessmentCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	written, err := salesassessment.Parse(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	cited := map[string]bool{}
	for _, sentence := range written.Sentences {
		for _, id := range sentence.Cells {
			cited[id] = true
		}
	}
	var missing []string
	for _, label := range c.expected.Cites {
		if !cited[c.labels[label]] {
			missing = append(missing, label)
		}
	}
	if len(missing) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: "no kept sentence cites " + strings.Join(missing, ", ")}
	}
	switch {
	case c.expected.NextDecision == "none" && written.Next != nil:
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: "a next decision was named where none is supported"}
	case c.expected.NextDecision != "none" && (written.Next == nil || written.Next.DealID != c.labels[c.expected.NextDecision]):
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the next decision does not name %s", c.expected.NextDecision)}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

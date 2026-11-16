// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package goalproposal

// The goal proposal's certification case. A scenario names months that must
// get a larger goal than others, and the yearly level a right proposal lands
// in. Run calls Request and Evaluate calls Parse, so a change to either is a
// change to what is certified.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const caseSite = "report_setup/sales_goal_months"

// caseExpectation is what a right proposal holds. Every month in Above gets
// a larger goal than every month in Below, months counted from 1. The total
// bounds are whole major units, and zero leaves a bound unasked.
type caseExpectation struct {
	Above    []int `json:"above"`
	Below    []int `json:"below"`
	TotalMin int64 `json:"total_min"`
	TotalMax int64 `json:"total_max"`
}

// Cases serves the sales_goal_months site of the report_setup task.
type Cases struct{}

// Site names the one site these cases certify.
func (Cases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskReportSetup, Variant: "sales_goal_months", Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no reply could pass or every reply would: no
// closed revenue to read, a month outside the year, or nothing expected.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (Cases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var in Input
	if err := json.Unmarshal(fixture, &in); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", caseSite, err)
	}
	var want caseExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected value is not {above, below, total_min, total_max}: %w", caseSite, err)
	}
	if !in.HasHistory() {
		return nil, fmt.Errorf("%s: the fixture closes no revenue, and production never asks without any", caseSite)
	}
	if err := want.answerable(); err != nil {
		return nil, fmt.Errorf("%s: %w", caseSite, err)
	}
	in.Lang = "en"
	if in.YearlyGoal != nil {
		in.YearlyGoalMinor = *in.YearlyGoal * values.MinorUnitScale(in.Currency)
	}
	return &preparedCase{in: in, expected: want}, nil
}

func (e caseExpectation) answerable() error {
	for _, month := range append(append([]int{}, e.Above...), e.Below...) {
		if month < 1 || month > monthsInYear {
			return fmt.Errorf("the expectation names month %d, which no year holds", month)
		}
	}
	if (len(e.Above) == 0) != (len(e.Below) == 0) {
		return errors.New("the expectation names months above or below, and a comparison needs both")
	}
	if len(e.Above) == 0 && e.TotalMin == 0 && e.TotalMax == 0 {
		return errors.New("the expectation asks nothing, so every proposal would pass")
	}
	return nil
}

type preparedCase struct {
	in       Input
	expected caseExpectation
}

func (c *preparedCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := Request(c.in)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", caseSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate reads the reply through the production gate and asks whether the
// months follow the expected pattern at the expected yearly level.
func (c *preparedCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	got, err := Parse(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	for _, high := range c.expected.Above {
		for _, low := range c.expected.Below {
			if got.Months[high-1] <= got.Months[low-1] {
				return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("month %d gets no larger goal than month %d", high, low)}
			}
		}
	}
	total := values.WholeMajorUnits(got.Total(), c.in.Currency)
	if total < c.expected.TotalMin || (c.expected.TotalMax > 0 && total > c.expected.TotalMax) {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the year adds up to %d, outside %d to %d", total, c.expected.TotalMin, c.expected.TotalMax)}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

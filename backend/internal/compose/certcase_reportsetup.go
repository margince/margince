// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The setup proposal's certification cases, one per drawer. A scenario names
// the values a right proposal resolves, by the labels of its fixture, and
// the least number of phrases it must name back. Run calls
// reportsetup.Request and Evaluate calls reportsetup.Parse, so a change to
// either is a change to what is certified.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/reportsetup"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// reportSetupFixture is one proposal's input with product labels in place
// of ids: the case mints the ids, so no scenario carries one.
type reportSetupFixture struct {
	Description string               `json:"description"`
	Today       string               `json:"today"`
	Lang        string               `json:"lang"`
	Products    []reportSetupProduct `json:"products"`
	Catalog     reportsetup.Catalog  `json:"catalog"`
}

type reportSetupProduct struct {
	Label string `json:"label"`
	Name  string `json:"name"`
}

// reportSetupExpectation is what a right proposal resolves. A product is
// named by its fixture label; the rest by id. Absent means the words do not
// name it, and a proposal that does is wrong.
type reportSetupExpectation struct {
	Product     string   `json:"product"`
	Industry    string   `json:"industry"`
	Country     string   `json:"country"`
	Period      string   `json:"period"`
	Metrics     []string `json:"metrics"`
	Charts      []string `json:"charts"`
	Objective   string   `json:"objective"`
	Target      int64    `json:"target_value"`
	Deadline    string   `json:"deadline"`
	Unsupported int      `json:"unsupported"`
}

type reportSetupCases struct{ site string }

func (c reportSetupCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskReportSetup, Variant: c.site, Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no reply could pass: an expected product the
// fixture does not carry, or an expectation this drawer cannot take.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (c reportSetupCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	site := "report_setup/" + c.site
	var f reportSetupFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", site, err)
	}
	var want reportSetupExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected value is not {product, industry, country, period, metrics, objective, unsupported}: %w", site, err)
	}
	if strings.TrimSpace(f.Description) == "" {
		return nil, fmt.Errorf("%s: the fixture needs a description, as production never asks without one", site)
	}
	in := reportsetup.Input{Site: c.site, Description: f.Description, Today: f.Today, Lang: f.Lang, Catalog: f.Catalog}
	in.Catalog.Countries = contacts.AssignedCountryCodes()
	labels := map[string]string{}
	for _, product := range f.Products {
		if err := refuseUnnameable(product.Label, "product", labels); err != nil {
			return nil, fmt.Errorf("%s: %w", site, err)
		}
		id := ids.NewV7().String()
		labels[product.Label] = id
		in.Catalog.Products = append(in.Catalog.Products, reportsetup.Option{ID: id, Label: product.Name})
	}
	if want.Product != "" {
		if _, ok := labels[want.Product]; !ok {
			return nil, fmt.Errorf("%s: the expected product %q names no product of the fixture", site, want.Product)
		}
	}
	if c.site == reportsetup.SiteInitiative && len(want.Metrics) > 0 {
		return nil, fmt.Errorf("%s: an initiative takes no metrics", site)
	}
	if c.site == reportsetup.SiteCustomReport && want.Objective != "" {
		return nil, fmt.Errorf("%s: a custom report takes no objective", site)
	}
	return &reportSetupCase{site: site, in: in, labels: labels, expected: want}, nil
}

type reportSetupCase struct {
	site     string
	in       reportsetup.Input
	labels   map[string]string
	expected reportSetupExpectation
}

// judgeObjectiveFigures asks whether the target and the deadline are the
// ones the words gave: none when the words gave none.
func judgeObjectiveFigures(got *reportsetup.Objective, want reportSetupExpectation) (aitasks.Outcome, bool) {
	var target int64
	var deadline string
	if got != nil {
		target, deadline = got.TargetValue, got.Deadline
	}
	if target != want.Target {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the target is %d, want %d", target, want.Target)}, true
	}
	if deadline != want.Deadline {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the deadline is %q, want %q", deadline, want.Deadline)}, true
	}
	return aitasks.Outcome{}, false
}

func (c *reportSetupCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := reportsetup.Request(c.in)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", c.site, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate reads the reply through the production gate and asks whether
// what survived is the expected setup, field by field.
func (c *reportSetupCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	got, err := reportsetup.Parse(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	want := c.expected
	objective := ""
	if got.Objective != nil {
		objective = got.Objective.Metric
	}
	for _, field := range []struct{ name, got, want string }{
		{"product", got.Product, c.labels[want.Product]},
		{"industry", got.Industry, want.Industry},
		{"country", got.Country, want.Country},
		{"period", got.Period, want.Period},
		{"objective", objective, want.Objective},
	} {
		if field.got != field.want {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the %s is %q, want %q", field.name, field.got, field.want)}
		}
	}
	if strings.Join(got.Metrics, ",") != strings.Join(want.Metrics, ",") {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the metrics are %v, want %v", got.Metrics, want.Metrics)}
	}
	if strings.Join(got.Blocks, ",") != strings.Join(want.Charts, ",") {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the charts are %v, want %v", got.Blocks, want.Charts)}
	}
	if outcome, wrong := judgeObjectiveFigures(got.Objective, want); wrong {
		return outcome
	}
	if len(got.Unsupported) < want.Unsupported {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("%d phrases named back, want at least %d", len(got.Unsupported), want.Unsupported)}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

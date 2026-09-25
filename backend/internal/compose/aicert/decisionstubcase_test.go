// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The stand-in decision case: the widget case with a decision form beside its
// LLM prompt. Its criterion, floor and LLM system prompt are fields, so a test
// can move exactly one of them and watch which stamp follows.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The two labels the stand-in question offers; the widget scenarios expect
// the first.
const (
	labelWidget = "widget"
	labelGadget = "gadget"
)

// widgetForm is what a stand-in decision case asks and how its gate reads.
type widgetForm struct {
	criterion string  // the rule that makes labelWidget right
	floor     float64 // the one floor for every label
	system    string  // the LLM prompt's system text
	askAt     string  // the site the decision is asked at; "" is the case's own
}

func defaultWidgetForm() widgetForm {
	return widgetForm{criterion: "the subject is a widget", floor: 0.8, system: "Describe the subject in one sentence."}
}

// decidingWidgetCases binds the stand-in decision case to a site.
type decidingWidgetCases struct {
	site aitasks.Site
	form widgetForm
}

func (c decidingWidgetCases) Site() aitasks.Site { return c.site }

func (c decidingWidgetCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	prepared, err := widgetCases{site: c.site}.Prepare(fixture, expected)
	if err != nil {
		return nil, err
	}
	widget, ok := prepared.(widgetCase)
	if !ok {
		return nil, errNotAWidget
	}
	return decidingWidgetCase{widgetCase: widget, form: c.form, variant: c.site.Variant}, nil
}

type stubError string

func (e stubError) Error() string { return string(e) }

const errNotAWidget = stubError("the widget factory prepared something else")

type decidingWidgetCase struct {
	widgetCase
	form    widgetForm
	variant string
}

// Run is the widget case's run under the form's own system prompt, so a test
// can move the LLM prompt without touching the decision form.
func (c decidingWidgetCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := model.Request{System: c.form.system, Messages: []model.Message{{Role: "user", Content: c.subject}}, MaxTokens: 1024}
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, err
	}
	trace.Output = resp.Text
	return trace, nil
}

func (c decidingWidgetCase) DecisionSite() string {
	if c.form.askAt != "" {
		return c.form.askAt
	}
	return c.variant
}

func (c decidingWidgetCase) DecisionRequest() decision.Request {
	state, err := json.Marshal(map[string]string{"subject": c.subject})
	if err != nil {
		panic(err) // a map of strings always marshals
	}
	return decision.Request{State: state, Questions: map[string]decision.Question{
		"kind": {Type: decision.Choice, Instructions: "What is the subject?", Criteria: map[string]string{
			labelWidget: c.form.criterion, labelGadget: "the subject is a gadget",
		}},
	}}
}

func (c decidingWidgetCase) GateDecision(a decision.Answer) ai.DecisionVerdict {
	if a.Choice != labelWidget && a.Choice != labelGadget {
		return ai.DecisionOffEnum
	}
	if a.Confidence < c.form.floor {
		return ai.DecisionBelowFloor
	}
	return ai.DecisionAccepted
}

func (c decidingWidgetCase) Floors() map[string]float64 {
	return map[string]float64{labelWidget: c.form.floor, labelGadget: c.form.floor}
}

func (c decidingWidgetCase) EvaluateDecision(a decision.Answer) aitasks.Outcome {
	if a.Choice != c.want {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: "the decision chose " + a.Choice}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// decidingCensus registers the widget site on task with the decision case
// bound, under form.
func decidingCensus(t *testing.T, task ai.Task, form widgetForm) *aitasks.Registry {
	t.Helper()
	site := aitasks.Site{Task: task, Variant: widgetVariant, Kind: ai.SiteKindOneShot}
	r := aitasks.NewRegistry()
	r.Register(site)
	r.BindCase(site, decidingWidgetCases{site: site, form: form})
	return r
}

// decidingScenario is a widget scenario on task, which expects labelWidget.
func decidingScenario(name string, task ai.Task) Scenario {
	sc := testScenario(name, wideBands)
	sc.Task = string(task)
	return sc
}

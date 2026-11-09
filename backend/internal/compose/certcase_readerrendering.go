// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The reader rendering's certification case. A scenario names the spans that
// must survive (names, amounts, dates, links) and the ones that must not appear.
// Run and Evaluate call the production Request, Parse and Check, so a change
// to any of them changes what is certified.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/readerrendering"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const readerRenderingSite = "reader_rendering/activity_text"

// readerRenderingFixture is one activity's text as it is stored. Prepare cuts
// it the way the service does, so a scenario with quoted history certifies
// the cut too.
type readerRenderingFixture struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Source  string `json:"source"`
	Target  string `json:"target"`
}

// readerRenderingExpectation is what a passing rendering carries and leaves out.
type readerRenderingExpectation struct {
	AlreadyInTarget bool     `json:"already_in_target"`
	Keeps           []string `json:"keeps"`
	Omits           []string `json:"omits"`
}

type readerRenderingCases struct{}

func (readerRenderingCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskReaderRendering, Variant: "activity_text", Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no model answer could pass. That is a span to keep
// that the source does not carry, or a target no shipped language matches.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (readerRenderingCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f readerRenderingFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", readerRenderingSite, err)
	}
	if !textlang.Known(f.Target) {
		return nil, fmt.Errorf("%s: target %q is not a shipped language", readerRenderingSite, f.Target)
	}
	if f.Source != "" && !textlang.Known(f.Source) {
		return nil, fmt.Errorf("%s: source %q is not a shipped language; leave it empty for an unknown one", readerRenderingSite, f.Source)
	}
	body := readerrendering.ReadableBody(crmcontracts.ActivityKind(f.Kind), f.Body)
	if strings.TrimSpace(f.Subject+body) == "" {
		return nil, fmt.Errorf("%s: the fixture has no text; the endpoint answers 404 for that", readerRenderingSite)
	}
	var want readerRenderingExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected value is not {already_in_target, keeps, omits}: %w", readerRenderingSite, err)
	}
	for _, span := range want.Keeps {
		if !strings.Contains(f.Subject+"\n"+body, span) {
			return nil, fmt.Errorf("%s: the expected span %q is not in the source, so no rendering can keep it", readerRenderingSite, span)
		}
	}
	source := textlang.Unknown
	if f.Source != "" {
		source = textlang.Lang(f.Source)
	}
	return &readerRenderingCase{
		in:       readerrendering.Input{Subject: f.Subject, Body: body, Source: source, Target: textlang.Lang(f.Target)},
		expected: want,
	}, nil
}

type readerRenderingCase struct {
	in       readerrendering.Input
	expected readerRenderingExpectation
}

func (c *readerRenderingCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := readerrendering.Request(c.in)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", readerRenderingSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate reads the reply through the production parser and validator, then
// asks whether it kept and left out what the scenario names.
func (c *readerRenderingCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	reply, err := readerrendering.Parse(trace.Output)
	if err == nil {
		err = readerrendering.Check(c.in, reply)
	}
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if reply.AlreadyInTarget != c.expected.AlreadyInTarget {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: fmt.Sprintf("already_in_target is %t, wanted %t", reply.AlreadyInTarget, c.expected.AlreadyInTarget),
		}
	}
	rendered := reply.Subject + "\n" + reply.Body
	for _, span := range c.expected.Keeps {
		if !strings.Contains(rendered, span) {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the rendering lost %q", span)}
		}
	}
	lowered := strings.ToLower(rendered)
	for _, span := range c.expected.Omits {
		if strings.Contains(lowered, strings.ToLower(span)) {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the rendering carries %q", span)}
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

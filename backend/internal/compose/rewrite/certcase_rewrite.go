// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rewrite

// The rewrite site's certification case. A scenario names its envelope and
// verb, the spans that must survive and the ones that must not appear. Run and Evaluate call the
// production Request, Parse and Check, so a change to any of them changes what
// is certified.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const certSite = "rewrite/rewrite"

// certFixture is one press of a rewrite verb: the rep's words and the reader.
type certFixture struct {
	Envelope string `json:"envelope"`
	Verb     string `json:"verb"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Reader   string `json:"reader"`
}

// certExpectation is what a passing polish carries and leaves out.
type certExpectation struct {
	Keeps []string `json:"keeps"`
	Omits []string `json:"omits"`
}

// CertCases serves the rewrite/rewrite site to the certification lane.
type CertCases struct{}

// Site names the rewrite/rewrite site this case certifies.
func (CertCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskRewrite, Variant: "rewrite", Kind: ai.SiteKindOneShot}
}

// Prepare refuses a scenario no answer could pass: no text, an unshipped
// reader language, or a span to keep that the rep never typed.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (CertCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f certFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", certSite, err)
	}
	if !textlang.Known(f.Reader) {
		return nil, fmt.Errorf("%s: reader %q is not a shipped language", certSite, f.Reader)
	}
	if !Envelope(f.Envelope).Admits(Verb(f.Verb)) {
		return nil, fmt.Errorf("%s: envelope %q does not offer verb %q; the endpoint refuses it", certSite, f.Envelope, f.Verb)
	}
	var want certExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("%s: the expected value is not {keeps, omits}: %w", certSite, err)
	}
	in := InputFor(Ask{
		Envelope: Envelope(f.Envelope), Verb: Verb(f.Verb), Subject: f.Subject, Body: f.Body, Reader: textlang.Lang(f.Reader),
	})
	if in.Subject+in.Body == "" {
		return nil, fmt.Errorf("%s: the fixture has no words the envelope rewrites; the endpoint refuses that", certSite)
	}
	for _, span := range want.Keeps {
		if !strings.Contains(in.Subject+"\n"+in.Body, span) {
			return nil, fmt.Errorf("%s: the expected span %q is not in the text, so no rewrite can keep it", certSite, span)
		}
	}
	return &certCase{in: in, expected: want}, nil
}

type certCase struct {
	in       Input
	expected certExpectation
}

func (c *certCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := Request(c.in, model.AboutNoContact("certification corpus"), draftvoice.Context{})
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", certSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate reads the reply through the production parser and validator, then
// asks whether it kept and left out what the scenario names.
func (c *certCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	reply, err := Parse(trace.Output)
	if err == nil {
		err = Check(c.in, reply)
	}
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	polished := reply.Subject + "\n" + reply.Body
	for _, span := range c.expected.Keeps {
		if !strings.Contains(polished, span) {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the rewrite lost %q", span)}
		}
	}
	lowered := strings.ToLower(polished)
	for _, span := range c.expected.Omits {
		if strings.Contains(lowered, strings.ToLower(span)) {
			return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: fmt.Sprintf("the rewrite carries %q", span)}
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

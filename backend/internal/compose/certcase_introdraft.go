// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The introduction-request site's certification case.
//
// What this site must get right is RESTRAINT about a relationship it did not
// observe. The facts are few — who knows whom, how warm, when they last spoke —
// and the expensive mistake is a sentence claiming more closeness than the
// record holds, because the colleague reading it can falsify it instantly and
// the rep who sent it looks careless.
//
// Run calls org360.IntroRequestFor and Evaluate calls org360.ParseIntroDraft,
// both the production path. A case that rebuilt either would measure a copy.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/org360"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// introDraftSite is the site name, as ai-tasks.yaml declares it.
const introDraftSite = "draft_reply/intro"

// introDraftCases serves the ask to a colleague.
type introDraftCases struct{}

func (introDraftCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskDraftReply,
		Variant: "intro",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare reads the fixture as the facts this site is handed, and refuses a
// scenario it cannot answer.
//
// The refusals are the ones that would certify a call the product never makes:
// the endpoint requires a colleague WITH a recorded route, so a fixture missing
// either name describes a request that is refused before any model is asked.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (introDraftCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var in org360.IntroFixture
	if err := json.Unmarshal(fixture, &in); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", introDraftSite, err)
	}
	for what, supplied := range map[string]string{
		"the colleague being asked": in.Colleague,
		"the contact to be met":     in.Contact,
	} {
		if strings.TrimSpace(supplied) == "" {
			return nil, fmt.Errorf(
				"%s: the fixture names %s nowhere, and the endpoint refuses such a request "+
					"before any model is asked — a scenario without it certifies a call the "+
					"product never makes", introDraftSite, what)
		}
	}
	// What the draft must NOT say. An empty list is a real expectation: it
	// means the scenario is checking only that a sendable message came back.
	var forbidden []string
	if err := json.Unmarshal(expected, &forbidden); err != nil {
		return nil, fmt.Errorf(
			"%s: the expected value is not a list of phrases the draft must avoid: %w",
			introDraftSite, err)
	}
	return &introDraftCase{in: in, forbidden: forbidden}, nil
}

// introDraftCase is one introduction request ready to be answered.
type introDraftCase struct {
	in        org360.IntroFixture
	forbidden []string
}

// Run issues the one request this site sends, through the production builder.
func (c *introDraftCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := org360.IntroRequestFor(c.in)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", introDraftSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate runs the production check, then asks whether the draft overclaimed.
//
// A reply the checker refuses is INVALID — the reader would have got the
// template instead, and scoring the template would certify text no model wrote.
// A reply it accepts is wrong only when it says something the scenario names as
// a claim the record does not support.
func (c *introDraftCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	_, body, err := org360.CheckIntroDraft(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	folded := strings.ToLower(body)
	var claimed []string
	for _, phrase := range c.forbidden {
		if strings.Contains(folded, strings.ToLower(phrase)) {
			claimed = append(claimed, phrase)
		}
	}
	if len(claimed) > 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: "claimed what the record does not hold: " + strings.Join(claimed, ", "),
		}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for capture_classify/deal_topic. The request comes
// from dealTopicRequest and the reply is judged by validateDealTopicPayload,
// the builder and validator the engine uses. The expectation is one verdict
// per fixture deal, in fixture order: the ids are minted here, so position is
// the identity a scenario can name.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// dealTopicFixture is one mail and the deals it might be about, in the fields
// the prompt consumes and with no ids.
type dealTopicFixture struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Deals   []struct {
		Name    string   `json:"name"`
		Project string   `json:"project"`
		Recent  []string `json:"recent"`
	} `json:"deals"`
}

// dealTopicCases serves the site that confirms a suggested deal.
type dealTopicCases struct{}

func (dealTopicCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskCaptureClassify, Variant: "deal_topic", Kind: ai.SiteKindOneShot}
}

// Prepare turns one mail, its candidate deals and the verdicts the scenario
// expects into a runnable case. It mints a deal id per candidate, as the read
// in production supplies them.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (dealTopicCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f dealTopicFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("capture_classify/deal_topic: the fixture is not the shape this site takes: %w", err)
	}
	if err := refuseUnreadableDealTopic(f); err != nil {
		return nil, err
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("capture_classify/deal_topic: the expected answer is not a list of verdicts: %w", err)
	}
	if len(want) != len(f.Deals) {
		return nil, fmt.Errorf("capture_classify/deal_topic: the scenario expects %d verdicts for %d deals", len(want), len(f.Deals))
	}
	for i, verdict := range want {
		if !dealTopicVerdicts[verdict] {
			return nil, fmt.Errorf("capture_classify/deal_topic: the scenario expects %q for deal %d, which is not about|not_about|unsure", verdict, i+1)
		}
	}
	in := dealTopicInput{Subject: f.Subject, Body: f.Body}
	for _, deal := range f.Deals {
		in.Deals = append(in.Deals, dealTopicDeal{ID: ids.NewV7(), Name: deal.Name, Project: deal.Project, Recent: deal.Recent})
	}
	return &dealTopicCase{in: in, expected: want}, nil
}

// refuseUnreadableDealTopic names a fixture the production read could never
// return. That is no deal, more candidates than one check weighs, a body
// longer than the read keeps, or more filed subjects than it reads.
func refuseUnreadableDealTopic(f dealTopicFixture) error {
	switch {
	case len(f.Deals) == 0:
		return errors.New("capture_classify/deal_topic: the fixture names no deal, so there is nothing to judge")
	case len(f.Deals) > capture.DealTopicCandidates:
		return fmt.Errorf("capture_classify/deal_topic: the fixture names %d deals, but one check weighs at most %d", len(f.Deals), capture.DealTopicCandidates)
	case utf8.RuneCountInString(f.Body) > dealTopicBodyLimit:
		return fmt.Errorf("capture_classify/deal_topic: the body is longer than the %d characters the read keeps", dealTopicBodyLimit)
	}
	for i, deal := range f.Deals {
		if len(deal.Recent) > dealTopicRecentSubjects {
			return fmt.Errorf("capture_classify/deal_topic: deal %d lists %d filed subjects, but the read keeps %d", i+1, len(deal.Recent), dealTopicRecentSubjects)
		}
	}
	return nil
}

// dealTopicCase is one mail ready to be judged.
type dealTopicCase struct {
	in       dealTopicInput
	expected []string
}

// Run issues the one request this site sends, bare. Production wraps it in the
// shape re-ask. A case that did so would certify the answer a model gives
// after being told to try again.
func (c *dealTopicCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := dealTopicRequest(c.in)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("capture_classify/deal_topic: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the engine's checks in the engine's order, then compares
// each verdict with the scenario's. The confidence floor is the engine's
// decision about an answer it already believes, so it is not applied here.
func (c *dealTopicCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var payload dealTopicPayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: fmt.Sprintf("unparseable model output: %v", err)}
	}
	if msg := validateDealTopicPayload(payload, c.in); msg != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: msg}
	}
	judged := make(map[string]string, len(payload.Results))
	for _, r := range payload.Results {
		judged[r.DealID] = r.Verdict
	}
	var wrong []string
	for i, deal := range c.in.Deals {
		if got := judged[deal.ID.String()]; got != c.expected[i] {
			wrong = append(wrong, fmt.Sprintf("deal %d is judged %q where the scenario expects %q", i+1, got, c.expected[i]))
		}
	}
	if len(wrong) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(wrong, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

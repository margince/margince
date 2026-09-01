// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the meeting-plan case owes the certification lane: it issues the request
// the service issues, it reads the reply with the service's own parser, and it
// separates the three things a reply can be.
//
// The separation carries the meaning of a run here. ParsePlan DROPS what it
// cannot ground rather than refusing the reply, so a plan about the wrong
// conversation and a plan the parser rejected both arrive with fields missing —
// and they are opposite events. The first is a model that read the newest thing
// instead of the relevant one; the second is a model that produced something
// unusable. A case that graded them alike would report the first as a parser
// problem and never measure the judgement it exists to measure.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// meetingPlanStub answers from the ids the REQUEST carries, which is the only
// place they exist: Prepare mints them and hands them to nobody. A stub told
// the ids up front would prove less than a model reading them out of the
// prompt, which is exactly what this site asks a model to do.
type meetingPlanStub struct {
	answer func(prompt string) string
	seen   []model.Request
}

func (s *meetingPlanStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.seen = append(s.seen, req)
	return model.Response{Text: s.answer(req.Messages[0].Content)}, nil
}

// idFor pulls one message's activity id out of the prompt, by the subject it
// carries. This is the stub standing in for a model that read the briefing.
func idFor(t *testing.T, prompt, subject string) string {
	t.Helper()
	var shown struct {
		Moments []struct {
			Messages []struct {
				ActivityID string `json:"activity_id"`
				Subject    string `json:"subject"`
			} `json:"messages"`
		} `json:"account_moments"`
	}
	fenced := prompt[strings.Index(prompt, "{") : strings.LastIndex(prompt, "}")+1]
	if err := json.Unmarshal([]byte(fenced), &shown); err != nil {
		t.Fatalf("the prompt is not readable JSON: %v", err)
	}
	for _, moment := range shown.Moments {
		for _, message := range moment.Messages {
			if message.Subject == subject {
				return message.ActivityID
			}
		}
	}
	t.Fatalf("the prompt carries no message subjected %q — the projection dropped it", subject)
	return ""
}

const planFixture = `{
	"subject": "Coffee with Rainer",
	"company": "Asia Flight Services",
	"attendee": "Rainer Vogt",
	"messages": [
		{"label":"wish_list","days_ago":62,"direction":"inbound","subject":"CRM requirements",
		 "body":"We need issue tracking and quote tracking in one place. How do we get started?"},
		{"label":"newsletter","days_ago":1,"direction":"outbound","subject":"Product news",
		 "body":"This month's release notes."}
	],
	"claims": [{"kind":"open_question","body":"How do we get started?","from_label":"wish_list"}]
}`

const planExpectation = `{"cites_label":"wish_list","names_token":"quote tracking"}`

func preparedPlanCase(t *testing.T) aitasks.PreparedCase {
	t.Helper()
	prepared, err := meetingPlanCases{}.Prepare(
		json.RawMessage(planFixture), json.RawMessage(planExpectation))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	return prepared
}

func runPlanCase(t *testing.T, answer func(prompt string) string) aitasks.Outcome {
	t.Helper()
	prepared := preparedPlanCase(t)
	stub := &meetingPlanStub{answer: answer}
	trace, err := prepared.Run(context.Background(), stub)
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace)
}

// A plan that read the right conversation and said what they asked for.
func TestTheMeetingPlanCaseAcceptsAPlanThatReadTheRightThread(t *testing.T) {
	got := runPlanCase(t, func(prompt string) string {
		wishList := idFor(t, prompt, "CRM requirements")
		return `{"questions":[{"ask":"Of issue tracking and quote tracking, which would change more?",
			"why":"They named both and neither has started.",
			"listen_for":"Which one costs them today.",
			"evidence":[{"entity_type":"activity","entity_id":"` + wishList + `"}]}]}`
	})
	if got.Result != aitasks.OutcomeAccepted {
		t.Errorf("outcome = %q (%s), want accepted", got.Result, got.Detail)
	}
}

// A plan built on the newest message rather than the relevant one. Well-formed,
// grounded, and about the wrong conversation — which is a measurement of the
// model, not a defect in the product.
func TestTheMeetingPlanCaseNamesAPlanThatReadTheNewestThing(t *testing.T) {
	got := runPlanCase(t, func(prompt string) string {
		newsletter := idFor(t, prompt, "Product news")
		return `{"questions":[{"ask":"Did you see the release notes?",
			"why":"We sent them yesterday.","listen_for":"Whether they read it.",
			"evidence":[{"entity_type":"activity","entity_id":"` + newsletter + `"}]}]}`
	})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Errorf("outcome = %q (%s), want wrong_answer for a plan about the newsletter",
			got.Result, got.Detail)
	}
}

// A plan whose questions would read the same about any prospect. Grounded in
// the right thread and still not preparation.
func TestTheMeetingPlanCaseNamesAGenericPlan(t *testing.T) {
	got := runPlanCase(t, func(prompt string) string {
		wishList := idFor(t, prompt, "CRM requirements")
		return `{"questions":[{"ask":"What are your priorities this quarter?",
			"why":"It is good to know.","listen_for":"Anything useful.",
			"evidence":[{"entity_type":"activity","entity_id":"` + wishList + `"}]}]}`
	})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Errorf("outcome = %q (%s), want wrong_answer for a plan naming nothing this account said",
			got.Result, got.Detail)
	}
}

// A reply the parser could not use at all. Different from the two above: the
// model produced nothing usable, rather than something usable about the wrong
// thing.
func TestTheMeetingPlanCaseSeparatesAnUnusableReply(t *testing.T) {
	got := runPlanCase(t, func(string) string { return "I'd be happy to help!" })
	if got.Result != aitasks.OutcomeInvalid {
		t.Errorf("outcome = %q (%s), want invalid for a reply that is not JSON", got.Result, got.Detail)
	}
}

// The fixture guards, which cost a parse here and a paid run if they are not
// here.
func TestTheMeetingPlanCaseRefusesAFixtureThatMeasuresNothing(t *testing.T) {
	for _, tc := range []struct{ name, fixture, expected, wants string }{
		{
			name:     "one conversation leaves no wrong one to cite",
			fixture:  `{"subject":"x","messages":[{"label":"only","subject":"a","body":"quote tracking"}]}`,
			expected: planExpectation,
			wants:    "fewer than two",
		},
		{
			name:     "a token no message carries could only be invented",
			fixture:  planFixture,
			expected: `{"cites_label":"wish_list","names_token":"nobody said this"}`,
			wants:    "appears in no message",
		},
		{
			name:     "an expectation naming a label the fixture lacks",
			fixture:  planFixture,
			expected: `{"cites_label":"absent","names_token":"quote tracking"}`,
			wants:    "which the fixture does not carry",
		},
		{
			name:     "no token at all would accept a generic plan",
			fixture:  planFixture,
			expected: `{"cites_label":"wish_list","names_token":""}`,
			wants:    "names no account-specific token",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := meetingPlanCases{}.Prepare(
				json.RawMessage(tc.fixture), json.RawMessage(tc.expected))
			if err == nil {
				t.Fatal("the case accepted a fixture that could measure nothing")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("refusal = %q, want it to name %q", err, tc.wants)
			}
		})
	}
}

// The request must be the one production sends, fenced, or the run measures a
// copy.
func TestTheMeetingPlanCaseIssuesTheRequestProductionIssues(t *testing.T) {
	prepared := preparedPlanCase(t)
	stub := &meetingPlanStub{answer: func(string) string { return "{}" }}
	trace, err := prepared.Run(context.Background(), stub)
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(trace.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(trace.Requests))
	}
	req := trace.Requests[0]
	if !strings.Contains(req.Messages[0].Content, "quote tracking") {
		t.Error("the prompt does not carry what the conversation said")
	}
	if req.SecretStripper == nil {
		t.Error("the request carries no secret stripper; production's does")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the person-brief case owes the certification lane: it separates the
// three things a reply can be, and it refuses a scenario that would measure
// nothing before anybody pays for a run.
//
// The separation carries the meaning of a run here. ParseBrief DROPS what it
// cannot ground rather than refusing the reply, so a brief about the wrong
// message and a brief the parser rejected both arrive with sentences missing —
// and they are opposite events. The first is a model that read the newest thing
// instead of the consequential one; the second is a model that produced
// something unusable, and a case grading them alike would report the first as a
// parser problem and never measure the judgement it exists to measure.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// personBriefStub answers from the ids the REQUEST carries, which is the only
// place they exist: Prepare mints them and hands them to nobody. A stub told
// the ids up front would prove less than a model reading them out of the
// prompt, which is exactly what this site asks a model to do.
type personBriefStub struct{ answer func(prompt string) string }

func (s *personBriefStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	return model.Response{Text: s.answer(req.Messages[0].Content)}, nil
}

// activityIDFor pulls one timeline row's id out of the prompt, by the subject
// it carries. This is the stub standing in for a model that read the summary.
func activityIDFor(t *testing.T, prompt, subject string) string {
	t.Helper()
	var shown struct {
		Recent []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
		} `json:"recent"`
	}
	fenced := prompt[strings.Index(prompt, "{") : strings.LastIndex(prompt, "}")+1]
	if err := json.Unmarshal([]byte(fenced), &shown); err != nil {
		t.Fatalf("the prompt is not readable JSON: %v", err)
	}
	for _, row := range shown.Recent {
		if row.Subject == subject {
			return row.ID
		}
	}
	t.Fatalf("the prompt carries no message subjected %q", subject)
	return ""
}

// The fixture the cases below vary: two messages, and the older one is the
// consequential one.
func personBriefScenario(t *testing.T) (json.RawMessage, json.RawMessage) {
	t.Helper()
	fixture, err := json.Marshal(personBriefFixture{
		Name: "Anna Weber", Title: "Head of Operations", Employer: "Acme Logistik GmbH",
		Messages: []personBriefMessage{
			{
				Label: "scheduling", DaysAgo: 1, Direction: fixtureInbound, Subject: "Re: renewal call",
				Preview: "Thursday at ten works for me.",
			},
			{
				Label: "objection", DaysAgo: 6, Direction: fixtureInbound, Subject: "Sub-processor list",
				Preview: "We cannot go ahead while the analytics vendor is on it.", Move: "needs_reply",
			},
		},
	})
	if err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	expected, err := json.Marshal(personBriefExpectation{
		CitesLabel: "objection", NamesToken: "analytics", Avoids: []string{"pipeline"},
	})
	if err != nil {
		t.Fatalf("encoding the expectation: %v", err)
	}
	return fixture, expected
}

func TestThePersonBriefCaseSeparatesTheThreeThingsAReplyCanBe(t *testing.T) {
	t.Parallel()
	fixture, expected := personBriefScenario(t)
	for _, tc := range []struct {
		name   string
		answer func(t *testing.T, prompt string) string
		want   string
	}{
		{
			name: "a brief about the consequential message",
			answer: func(t *testing.T, prompt string) string {
				return sentenceCiting("They will not proceed while the analytics vendor is listed.",
					activityIDFor(t, prompt, "Sub-processor list"))
			},
			want: aitasks.OutcomeAccepted,
		},
		{
			name: "a brief about the newest message instead",
			answer: func(t *testing.T, prompt string) string {
				return sentenceCiting("They confirmed Thursday at ten.",
					activityIDFor(t, prompt, "Re: renewal call"))
			},
			want: aitasks.OutcomeWrongAnswer,
		},
		{
			name:   "a reply that is not the shape this site reads",
			answer: func(*testing.T, string) string { return `{"sentences":"nothing to say"}` },
			want:   aitasks.OutcomeInvalid,
		},
		{
			// Parseable, and citing a record this input never held — so every
			// sentence is dropped, which production shows as a card with no
			// prose. That is not a wrong answer.
			name: "a reply citing nothing of this relationship",
			answer: func(*testing.T, string) string {
				return sentenceCiting("They spoke to your CFO.", "019fd000-0000-7000-8000-0000000000ff")
			},
			want: aitasks.OutcomeAbstained,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prepared, err := personBriefCases{}.Prepare(fixture, expected)
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			stub := &personBriefStub{answer: func(prompt string) string { return tc.answer(t, prompt) }}
			trace, err := prepared.Run(t.Context(), stub)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := prepared.Evaluate(trace); got.Result != tc.want {
				t.Errorf("outcome = %q (%s), want %q", got.Result, got.Detail, tc.want)
			}
		})
	}
}

// A forbidden phrase is the deterministic half of a silence expectation: a
// rubric alone can only ask a judge whether the brief stayed quiet.
func TestThePersonBriefCaseCatchesAPhraseTheReaderWasNotShown(t *testing.T) {
	t.Parallel()
	fixture, expected := personBriefScenario(t)
	prepared, err := personBriefCases{}.Prepare(fixture, expected)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	stub := &personBriefStub{answer: func(prompt string) string {
		return sentenceCiting("The analytics vendor blocks this account's Pipeline.",
			activityIDFor(t, prompt, "Sub-processor list"))
	}}
	trace, err := prepared.Run(t.Context(), stub)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := prepared.Evaluate(trace)
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("outcome = %q (%s), want a wrong answer", got.Result, got.Detail)
	}
	// Folded on both sides, so a capitalised spelling of the forbidden phrase
	// is still the phrase the reader was not shown.
	if !strings.Contains(got.Detail, "pipeline") {
		t.Errorf("detail = %q, want it to name the phrase the brief was not allowed to write", got.Detail)
	}
}

// A corpus author writes a conversation the way a conversation happens — oldest
// first — and the floor reads Recent[0] as the newest message. The harness
// imposes production's own order so that authoring choice cannot quietly hand
// the floor the wrong message and shift what the scenario is measuring.
func TestThePersonBriefCaseOrdersFixtureMessagesNewestFirst(t *testing.T) {
	t.Parallel()
	// Written oldest first, which is the order the same two messages carry in
	// person_brief_reads_what_was_said_01.yaml reversed.
	fixture, err := json.Marshal(personBriefFixture{
		Name: "Anna Weber",
		Messages: []personBriefMessage{
			{
				Label: "objection", DaysAgo: 6, Direction: fixtureInbound, Subject: "Sub-processor list",
				Preview: "We cannot go ahead while the analytics vendor is on it.",
			},
			{
				Label: "scheduling", DaysAgo: 1, Direction: fixtureInbound, Subject: "Re: renewal call",
				Preview: "Thursday at ten works for me.",
			},
		},
	})
	if err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	expected, err := json.Marshal(personBriefExpectation{
		CitesLabel: "objection", NamesToken: "analytics",
	})
	if err != nil {
		t.Fatalf("encoding the expectation: %v", err)
	}
	prepared, err := personBriefCases{}.Prepare(fixture, expected)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// The prompt is where the assembled order is observable, and it is the same
	// JSON production sends.
	stub := &personBriefStub{answer: func(prompt string) string {
		return sentenceCiting("They are waiting.", activityIDFor(t, prompt, "Sub-processor list"))
	}}
	trace, err := prepared.Run(t.Context(), stub)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	newest, oldest := subjectPositions(t, trace.Requests[0].Messages[0].Content)
	if newest > oldest {
		t.Errorf("the fixture reached the model oldest-first, so the floor would quote %q as the newest message",
			"Sub-processor list")
	}
}

// subjectPositions reports where each of the two subjects sits in the assembled
// timeline the prompt carries.
func subjectPositions(t *testing.T, prompt string) (newest, oldest int) {
	t.Helper()
	var shown struct {
		Recent []struct {
			Subject string `json:"subject"`
		} `json:"recent"`
	}
	fenced := prompt[strings.Index(prompt, "{") : strings.LastIndex(prompt, "}")+1]
	if err := json.Unmarshal([]byte(fenced), &shown); err != nil {
		t.Fatalf("the prompt is not readable JSON: %v", err)
	}
	newest, oldest = -1, -1
	for i, row := range shown.Recent {
		switch row.Subject {
		case "Re: renewal call":
			newest = i
		case "Sub-processor list":
			oldest = i
		}
	}
	if newest < 0 || oldest < 0 {
		t.Fatalf("the prompt carries %d timeline row(s); both fixture messages should be there", len(shown.Recent))
	}
	return newest, oldest
}

// A scenario that cannot fail is one that reports PASS forever while measuring
// nothing, and there is no failing assertion to notice it.
func TestThePersonBriefCaseRefusesAScenarioThatCouldNotFail(t *testing.T) {
	t.Parallel()
	base, _ := personBriefScenario(t)
	for _, tc := range []struct {
		name   string
		want   personBriefExpectation
		detail string
	}{
		{
			name:   "a token the floor already prints",
			want:   personBriefExpectation{CitesLabel: "objection", NamesToken: "Thursday at ten"},
			detail: "deterministic floor",
		},
		{
			name:   "a token nothing in the summary carries",
			want:   personBriefExpectation{CitesLabel: "objection", NamesToken: "quantum"},
			detail: "appears in nothing the summary carries",
		},
		{
			name:   "a message the fixture does not carry",
			want:   personBriefExpectation{CitesLabel: "invoice", NamesToken: "analytics"},
			detail: "which the fixture does not carry",
		},
		{
			name:   "no token at all",
			want:   personBriefExpectation{CitesLabel: "objection"},
			detail: "generic prose would satisfy it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expected, err := json.Marshal(tc.want)
			if err != nil {
				t.Fatalf("encoding the expectation: %v", err)
			}
			_, err = personBriefCases{}.Prepare(base, expected)
			if err == nil {
				t.Fatal("Prepare accepted a scenario no reply could fail")
			}
			if !strings.Contains(err.Error(), tc.detail) {
				t.Errorf("refusal = %q, want it to name %q", err, tc.detail)
			}
		})
	}
}

// sentenceCiting is one grounded reply in the shape this site reads.
func sentenceCiting(text, activityID string) string {
	return `{"sentences":[{"text":"` + text + `","evidence":[` +
		`{"entity_type":"activity","entity_id":"` + activityID + `"}]}]}`
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// refOfSubject reads the ref the prompt gave the message with this subject,
// as a model reading the summary would.
func refOfSubject(t *testing.T, prompt, subject string) string {
	t.Helper()
	found := regexp.MustCompile(`"id":"(a\d+)","kind":"[a-z_]+","subject":"` + regexp.QuoteMeta(subject) + `"`).FindStringSubmatch(prompt)
	if found == nil {
		t.Fatalf("the prompt carries no message subjected %q", subject)
	}
	return found[1]
}

func taskExplainScenario(t *testing.T) aitasks.PreparedCase {
	t.Helper()
	fixture, err := json.Marshal(taskExplainFixture{
		Task:        taskExplainTask{Type: "reach_out", Title: "Follow up on the acceptance", Origin: "commitment"},
		SourceLabel: "promise", SourceText: "I will send the signed acceptance once the shift leads sign off on the rota export.",
		Relationship: contactBriefFixture{
			Name: "Jonas Keller", Title: "Head of Operations", Employer: "Acme Logistik GmbH",
			Messages: []contactBriefMessage{
				{Label: "parking", DaysAgo: 1, Direction: fixtureInbound, Subject: "Parking", Preview: "Park behind building B."},
				{Label: "promise", DaysAgo: 4, Direction: fixtureInbound, Subject: "Re: Acceptance", Preview: "I will send the signed acceptance once the shift leads sign off on the rota export."},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := taskExplainCases{}.Prepare(fixture,
		json.RawMessage(`{"cites_label":"promise","names_token":"rota export","avoids":["salary"]}`))
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

// The case tells an explanation that rests on the promise from one that
// rests elsewhere, names no next step, or says what the reader was not shown.
func TestTheTaskExplanationCaseGradesWhatTheExplanationRestsOn(t *testing.T) {
	cases := []struct {
		name, fact, cites, next, grade string
	}{
		{"rests on the promise", "Jonas promised the signed acceptance once the shift leads sign off on the rota export.", "Re: Acceptance", "Ask Jonas whether the shift leads signed off.", aitasks.OutcomeAccepted},
		{"cites the wrong message", "Jonas promised the signed acceptance once the shift leads sign off on the rota export.", "Parking", "Ask Jonas whether the shift leads signed off.", aitasks.OutcomeWrongAnswer},
		{"says what was withheld", "Jonas tied the acceptance to the rota export and his salary review.", "Re: Acceptance", "Ask Jonas whether the shift leads signed off.", aitasks.OutcomeWrongAnswer},
		{"no next step", "Jonas promised the signed acceptance once the shift leads sign off on the rota export.", "Re: Acceptance", "", aitasks.OutcomeWrongAnswer},
	}
	for _, tc := range cases {
		prepared := taskExplainScenario(t)
		stub := &contactBriefStub{answer: func(prompt string) string {
			ref := refOfSubject(t, prompt, tc.cites)
			sentences := []map[string]any{{"text": tc.fact, "nature": "fact", "evidence": []map[string]string{{"entity_type": "activity", "entity_id": ref}}}}
			if tc.next != "" {
				sentences = append(sentences, map[string]any{"text": tc.next, "nature": "recommendation", "evidence": []map[string]string{{"entity_type": "activity", "entity_id": ref}}})
			}
			encoded, err := json.Marshal(map[string]any{"sentences": sentences})
			if err != nil {
				t.Fatal(err)
			}
			return string(encoded)
		}}
		trace, err := prepared.Run(context.Background(), stub)
		if err != nil {
			t.Fatal(err)
		}
		if got := prepared.Evaluate(trace); got.Result != tc.grade {
			t.Errorf("%s: graded %s (%s), want %s", tc.name, got.Result, got.Detail, tc.grade)
		}
	}
}

func TestATaskExplanationReplyThatIsNotAnExplanationIsGradedSo(t *testing.T) {
	prepared := taskExplainScenario(t)
	for reply, grade := range map[string]string{
		"I cannot help.": aitasks.OutcomeInvalid,
		`{"sentences":[{"text":"Jonas signed it.","nature":"fact","evidence":[{"entity_type":"activity","entity_id":"a99"}]}]}`: aitasks.OutcomeAbstained,
	} {
		if got := prepared.Evaluate(aitasks.Trace{Output: reply}); got.Result != grade {
			t.Errorf("reply %q graded %s, want %s", reply, got.Result, grade)
		}
	}
	if _, err := (taskExplainCases{}).Prepare(json.RawMessage(`{}`), json.RawMessage(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "token") {
		t.Errorf("a scenario naming nothing to cite was prepared (%v)", err)
	}
}

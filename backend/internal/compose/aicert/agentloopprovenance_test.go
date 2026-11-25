// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The scenario that holds a source value beside a record id, graded from
// the committed corpus file. The grader is proved against the fixture the
// certification lane sends, not a copy of it.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	provenanceScenario = "overnight_sweep_a_provenance_ref_is_not_a_record_id"
	provenanceDeal     = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e61"
	provenanceCalendar = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1f42"
)

type provenanceScript struct {
	replies  []string
	requests int
}

func (s *provenanceScript) Complete(context.Context, model.Request) (model.Response, error) {
	turn := min(s.requests, len(s.replies)-1)
	s.requests++
	return model.Response{Text: s.replies[turn]}, nil
}

func provenanceNote(t *testing.T, linked string) string {
	t.Helper()
	return provenanceNoteBody(t, linked, "No recorded activity for 27 days (deal.last_activity_at).")
}

func provenanceNoteBody(t *testing.T, linked, body string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"tool": "log_activity", "args": map[string]any{
		"kind": "note", "subject": "Gone quiet",
		"body":  body,
		"links": []map[string]string{{"entity_type": "deal", "entity_id": linked}},
	}})
	if err != nil {
		t.Fatalf("encoding the note: %v", err)
	}
	return string(raw)
}

func provenanceCensus(t *testing.T) (*aitasks.Registry, aicert.Scenario) {
	t.Helper()
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	for _, sc := range scenarios {
		if sc.Name == provenanceScenario {
			return census, sc
		}
	}
	t.Fatalf("the corpus holds no scenario %s", provenanceScenario)
	return nil, aicert.Scenario{}
}

func provenanceScenarioNamed(t *testing.T) aicert.Scenario {
	t.Helper()
	_, sc := provenanceCensus(t)
	return sc
}

func runProvenanceScenario(t *testing.T, replies ...string) aitasks.Outcome {
	t.Helper()
	census, sc := provenanceCensus(t)
	factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
	if !bound {
		t.Fatalf("site %s/%s binds no certification case", sc.Task, sc.Site)
	}
	prepared, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
	if err != nil {
		t.Fatalf("preparing %s: %v", sc.Name, err)
	}
	trace, err := prepared.Run(context.Background(), &provenanceScript{replies: replies})
	if err != nil {
		t.Fatalf("running %s: %v", sc.Name, err)
	}
	return prepared.Evaluate(trace)
}

func TestAProvenanceRefIsGradedAsNotARecordId(t *testing.T) {
	done := `{"final":{"summary":"done"}}`
	list := `{"tool":"whats_slipping_this_week","args":{}}`
	reads := []string{
		`{"tool":"read_record","args":{"record_type":"deal","id":"` + provenanceDeal + `"}}`,
		`{"tool":"catch_me_up_on","args":{"record_type":"deal","record_id":"` + provenanceDeal + `"}}`,
	}
	run := func(steps ...string) aitasks.Outcome {
		return runProvenanceScenario(t, append(append([]string{list}, steps...), done)...)
	}
	t.Run("a note on the listed deal", func(t *testing.T) {
		got := run(append(reads, provenanceNote(t, provenanceDeal))...)
		if got.Result != aitasks.OutcomeAccepted {
			t.Fatalf("Result = %q (%s), want accepted", got.Result, got.Detail)
		}
	})
	t.Run("a note linked to the source value", func(t *testing.T) {
		got := run(append(reads, provenanceNote(t, provenanceCalendar))...)
		if got.Result != aitasks.OutcomeWrongAnswer ||
			!strings.Contains(got.Detail, "is not on the whats_slipping_this_week list") {
			t.Fatalf("Result = %q (%s), want wrong_answer naming the list", got.Result, got.Detail)
		}
	})
	// The mechanical grader checks only which deal a note is linked to, so a
	// body that names the calendar value passes it. The rubric is what scores
	// that note low, so the test holds the rubric to saying so.
	t.Run("a note naming the calendar value in its body is left to the rubric", func(t *testing.T) {
		body := "See record " + provenanceCalendar + " for the last contact."
		if got := run(append(reads, provenanceNoteBody(t, provenanceDeal, body))...); got.Result != aitasks.OutcomeAccepted {
			t.Fatalf("Result = %q (%s), want accepted before the judge reads it", got.Result, got.Detail)
		}
		if !strings.Contains(provenanceScenarioNamed(t).Expect.Rubric, "its subject or body names that value") {
			t.Error("the scenario rubric no longer scores a note that names the source value as a record low")
		}
	})
	t.Run("a read of the source value is a 404 the run recovers from", func(t *testing.T) {
		read := `{"tool":"read_record","args":{"record_type":"deal","id":"` + provenanceCalendar + `"}}`
		got := run(append(append([]string{}, reads...), read, provenanceNote(t, provenanceDeal))...)
		if got.Result != aitasks.OutcomeAccepted {
			t.Fatalf("Result = %q (%s), want accepted", got.Result, got.Detail)
		}
	})
}

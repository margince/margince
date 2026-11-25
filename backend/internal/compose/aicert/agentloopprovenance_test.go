// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The scenario that holds a captured_by value beside a record id, graded from
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
	raw, err := json.Marshal(map[string]any{"tool": "log_activity", "args": map[string]any{
		"kind": "note", "subject": "Gone quiet",
		"body":  "No recorded activity for 27 days (deal.last_activity_at).",
		"links": []map[string]string{{"entity_type": "deal", "entity_id": linked}},
	}})
	if err != nil {
		t.Fatalf("encoding the note: %v", err)
	}
	return string(raw)
}

func runProvenanceScenario(t *testing.T, replies ...string) aitasks.Outcome {
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
		if sc.Name != provenanceScenario {
			continue
		}
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
	t.Fatalf("the corpus holds no scenario %s", provenanceScenario)
	return aitasks.Outcome{}
}

func TestAProvenanceRefIsGradedAsNotARecordId(t *testing.T) {
	list := `{"tool":"whats_slipping_this_week","args":{}}`
	done := `{"final":{"summary":"done"}}`
	t.Run("a note on the listed deal", func(t *testing.T) {
		got := runProvenanceScenario(t, list, provenanceNote(t, provenanceDeal), done)
		if got.Result != aitasks.OutcomeAccepted {
			t.Fatalf("Result = %q (%s), want accepted", got.Result, got.Detail)
		}
	})
	t.Run("a note linked to the captured_by value", func(t *testing.T) {
		got := runProvenanceScenario(t, list, provenanceNote(t, provenanceCalendar), done)
		if got.Result != aitasks.OutcomeWrongAnswer ||
			!strings.Contains(got.Detail, "is not on the whats_slipping_this_week list") {
			t.Fatalf("Result = %q (%s), want wrong_answer naming the list", got.Result, got.Detail)
		}
	})
	t.Run("a read of the captured_by value is a 404 the run recovers from", func(t *testing.T) {
		read := `{"tool":"read_record","args":{"record_type":"deal","id":"` + provenanceCalendar + `"}}`
		got := runProvenanceScenario(t, list, read, provenanceNote(t, provenanceDeal), done)
		if got.Result != aitasks.OutcomeAccepted {
			t.Fatalf("Result = %q (%s), want accepted", got.Result, got.Detail)
		}
	})
}

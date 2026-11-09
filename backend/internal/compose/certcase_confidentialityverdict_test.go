// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// heldCase is a thread whose scenario expects it held, ready to grade replies.
func heldCase(t *testing.T) *confidentialityCase {
	t.Helper()
	prepared, err := confidentialityCases{}.Prepare(json.RawMessage(`{"subject":"Statement","body":"Attached."}`), json.RawMessage(`"held"`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*confidentialityCase)
	if !ok {
		t.Fatalf("Prepare returned %T", prepared)
	}
	return c
}

// A "held" scenario passes the replies the engine would not open, and no other.
func TestAHeldScenarioPassesWhatTheEngineKeepsPrivate(t *testing.T) {
	c := heldCase(t)
	for reply, want := range map[string]string{
		`{"results":[{"id":"t1","verdict":"ordinary","confidence":0.95}]}`:  aitasks.OutcomeWrongAnswer,
		`{"results":[{"id":"t1","verdict":"ordinary","confidence":0.6}]}`:   aitasks.OutcomeAccepted,
		`{"results":[{"id":"t1","verdict":"personnel","confidence":0.95}]}`: aitasks.OutcomeAccepted,
		`{"results":[{"id":"t1","verdict":"personnel","confidence":0.3}]}`:  aitasks.OutcomeAccepted,
	} {
		if got := c.Evaluate(aitasks.Trace{Output: reply}).Result; got != want {
			t.Errorf("%s graded %v, want %v", reply, got, want)
		}
	}
}

// The document scenarios measure the model only if no rule holds the thread
// before the model is asked. The engine asks the rule first.
func TestEveryDocumentScenarioReachesTheModel(t *testing.T) {
	paths, err := filepath.Glob("aicert/corpus/capture_confidentiality_verdict/*.yaml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no confidentiality scenarios found (%v)", err)
	}
	checked := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path) // #nosec G304 -- a corpus file this test globbed
		if err != nil {
			t.Fatal(err)
		}
		var sc struct {
			Fixture map[string]any `yaml:"fixture"`
			Expect  struct {
				Answer any `yaml:"answer"`
			} `yaml:"expect"`
		}
		if err := yaml.Unmarshal(raw, &sc); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if sc.Fixture["documents"] == nil {
			continue
		}
		fixture, err := json.Marshal(sc.Fixture)
		if err != nil {
			t.Fatal(err)
		}
		answer, err := json.Marshal(sc.Expect.Answer)
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := confidentialityCases{}.Prepare(fixture, answer)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		c, ok := prepared.(*confidentialityCase)
		if !ok {
			t.Fatalf("Prepare returned %T", prepared)
		}
		checked++
		if isPersonnelWithoutAModel(c.row) {
			t.Errorf("%s: a rule holds this thread before the model is asked, so the scenario measures no model",
				filepath.Base(path))
		}
	}
	if checked < 4 {
		t.Fatalf("checked %d scenarios with documents, want every one of the four", checked)
	}
}

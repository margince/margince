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

// ruleHeldIncidents are scenarios kept as the incident arrived, though a rule
// now holds them before the model: they prove the prompt reads the document.
var ruleHeldIncidents = map[string]string{
	"personnel_02.yaml": "the salary sheet behind an invoice note, which the staff-cost rule now holds",
}

// The document scenarios measure the model only if no rule holds the thread
// before the model is asked. The engine marks each read document, then asks
// the rules, as here.
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
		for i := range c.row.Documents {
			doc := &c.row.Documents[i]
			doc.Personnel = !doc.Unreadable && documentReadsAsPersonnel(doc.Excerpt, doc.Excerpt)
		}
		_, isIncident := ruleHeldIncidents[filepath.Base(path)]
		if isPersonnelWithoutAModel(c.row) != isIncident {
			t.Errorf("%s: a rule holds it before the model is asked = %v, want %v",
				filepath.Base(path), !isIncident, isIncident)
		}
	}
	if checked < 5 {
		t.Fatalf("checked %d scenarios with documents, want every one of the five", checked)
	}
}

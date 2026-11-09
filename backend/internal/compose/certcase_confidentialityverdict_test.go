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
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/gatekit"
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
		got := c.Evaluate(aitasks.Trace{Output: reply})
		if got.Result != want {
			t.Errorf("%s graded %v, want %v", reply, got.Result, want)
		}
		// A held thread that opened is shown to the workspace: that is the leak.
		if got.Leak != (want == aitasks.OutcomeWrongAnswer) {
			t.Errorf("%s marked leak %v, want it a leak exactly when it opened", reply, got.Leak)
		}
	}
}

// A wrong kind that opens the thread shows it to colleagues, so it is a leak.
// A second holding kind, an unsure ordinary and a refused reply reach no one.
func TestOnlyAWrongKindThatOpensTheThreadIsALeak(t *testing.T) {
	prepared, err := confidentialityCases{}.Prepare(json.RawMessage(`{"subject":"Statement","body":"Attached."}`), json.RawMessage(`"personnel"`))
	if err != nil {
		t.Fatal(err)
	}
	for reply, wantLeak := range map[string]bool{
		`{"results":[{"id":"t1","verdict":"ordinary","confidence":0.95}]}`:  true,
		`{"results":[{"id":"t1","verdict":"personnel","confidence":0.95}]}`: false,
		`{"results":[{"id":"t1","verdict":"legal","confidence":0.95}]}`:     false,
		`{"results":[{"id":"t1","verdict":"ordinary","confidence":0.5}]}`:   false,
		`not json`: false,
	} {
		if got := prepared.Evaluate(aitasks.Trace{Output: reply}); got.Leak != wantLeak {
			t.Errorf("%s graded %q, leak %v, want leak %v", reply, got.Result, got.Leak, wantLeak)
		}
	}
}

// ruleHeldIncidents are scenarios kept as the incident arrived, though a rule
// now holds them before the model: they prove the prompt reads the document.
var ruleHeldIncidents = gatekit.Waive(map[string]string{
	"personnel_02.yaml": "the salary sheet behind an invoice note, which the staff-cost rule now holds",
})

// The document scenarios measure the model only if no rule holds the thread
// before the model is asked. The engine marks each read document, then asks
// the rules, as here.
func TestEveryDocumentScenarioReachesTheModel(t *testing.T) {
	defer ruleHeldIncidents.AssertAllMatched(t)
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
		if isPersonnelWithoutAModel(c.row) && !ruleHeldIncidents.Waived(t, filepath.Base(path)) {
			t.Errorf("%s: a rule holds it before the model is asked, so it measures no model", filepath.Base(path))
		}
	}
	if checked < 4 {
		t.Fatalf("checked %d scenarios with documents, want every one of the four", checked)
	}
}

// A confident "ordinary" opens a thread only when every document in it was
// read: what an unread file holds is unknown, whatever the model believed.
func TestAThreadWithAnUnreadDocumentNeverOpens(t *testing.T) {
	sure := confidentialityResult{Verdict: confidentialityOrdinary, Confidence: 0.95}
	readable := capture.ThreadDocument{Filename: "invoice.pdf", Excerpt: "Total 1,594.60 EUR"}
	unreadable := capture.ThreadDocument{Filename: "scan_0042.pdf", Unreadable: true}
	for name, tc := range map[string]struct {
		thread capture.PendingThread
		opens  bool
	}{
		"every document read":     {capture.PendingThread{Documents: []capture.ThreadDocument{readable}}, true},
		"one could not be read":   {capture.PendingThread{Documents: []capture.ThreadDocument{readable, unreadable}}, false},
		"one was left out unread": {capture.PendingThread{Documents: []capture.ThreadDocument{readable}, DocumentsOmitted: 1}, false},
		"a read file vouches for its own name": {
			capture.PendingThread{Documents: []capture.ThreadDocument{readable}, Attachments: []string{"invoice.pdf"}}, true,
		},
		"an unread image is named private": {
			capture.PendingThread{Documents: []capture.ThreadDocument{readable}, Attachments: []string{"invoice.pdf", "passport_scan.jpg"}}, false,
		},
	} {
		if got := confidentialityOpens(sure, tc.thread); got != tc.opens {
			t.Errorf("%s: opens = %v, want %v", name, got, tc.opens)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// certifyGeminiAt runs one passing scenario on a gemini candidate bound at
// level, served by a fake so no call leaves the process.
func certifyGeminiAt(t *testing.T, level string) Record {
	t.Helper()
	candidateFake := ai.NewFakeClient().Script("the widget is blue", "the widget is blue", "the widget is blue")
	judgeFake := ai.NewFakeClient().Script(scoreJSON(90), scoreJSON(90), scoreJSON(90))
	candidate := ai.ProviderConfig{Provider: "gemini", Model: "gemini-3.1-flash-lite-preview", ThinkingLevel: level}
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		candidate, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(),
		&certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithHarnessClient("gemini", candidateFake)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
		})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	return rec
}

func TestARecordNamesTheThinkingLevelItsCandidateRanAt(t *testing.T) {
	rec := certifyGeminiAt(t, "low")
	if rec.ThinkingLevel != "low" {
		t.Fatalf("thinking_level = %q, want low — the record would read as a run at the model's default", rec.ThinkingLevel)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"thinking_level":"low"`) {
		t.Errorf("the written record does not carry the level: %s", raw)
	}
}

// Every record committed before the field existed was run without a level, so
// omitting the key is what keeps those files byte-identical on a re-run.
func TestARecordRunWithoutAThinkingLevelOmitsTheKey(t *testing.T) {
	rec := certifyGeminiAt(t, "")
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "thinking_level") {
		t.Errorf("a record run at the default names a thinking level: %s", raw)
	}
}

// A site the contract tells to think runs at that level whatever the binding
// says, so the record names it per site: a record naming the binding's level
// alone would claim cold_start's company conversations ran at the default.
func TestARecordNamesTheSitesThatRanAtTheContractsLevel(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	var siteRead []Scenario
	for _, sc := range scenarios {
		if sc.Task == string(ai.TaskColdStart) && sc.Site == "sitereadmessage" {
			siteRead = append(siteRead, sc)
			break
		}
	}
	if len(siteRead) == 0 {
		t.Fatal("the corpus carries no cold_start/sitereadmessage scenario to run")
	}
	candidateFake := ai.NewFakeClient().Script("{}", "{}", "{}")
	candidate := ai.ProviderConfig{Provider: "gemini", Model: "gemini-3.1-flash-lite"}
	rec, err := certifyTask(wsContext(t), ai.TaskColdStart, siteRead, census,
		candidate, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileCloudFrontier, 3, quietLogger(),
		&certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithHarnessClient("gemini", candidateFake)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(ai.NewFakeClient())},
		})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if got := rec.ThinkingLevelAt("sitereadmessage"); got != "low" {
		t.Errorf("sitereadmessage ran at %q on the record, want the contract's low", got)
	}
	if got := rec.ThinkingLevelAt("acts"); got != "" {
		t.Errorf("acts declares no level and reads %q on the record, want the binding's default", got)
	}
}

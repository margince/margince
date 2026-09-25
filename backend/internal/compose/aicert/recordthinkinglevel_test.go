// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"encoding/json"
	"strings"
	"testing"

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

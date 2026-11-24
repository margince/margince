// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/modules/ai"
)

func gemini(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "config", "presets", "gemini_cloud.yaml")
}

// The two bindings the paid pass certifies are read off the preset's own tiers,
// so a preset edit moves what the lane measures.
func TestTheChoiceIsTheModelThePresetBindsAtThatTier(t *testing.T) {
	for tier, want := range map[string]string{"": "premium", "premium": "premium", "cheap_cloud": "cheap_cloud"} {
		got, err := ResolveChoice(gemini(t), tier, "", "")
		if err != nil {
			t.Fatalf("tier %q: %v", tier, err)
		}
		if got.Binding.Provider != "gemini" || got.Binding.Tier != want {
			t.Errorf("tier %q resolved to %+v", tier, got.Binding)
		}
	}
	premium, err := ResolveChoice(gemini(t), "premium", "", "")
	if err != nil {
		t.Fatal(err)
	}
	lower, err := ResolveChoice(gemini(t), "cheap_cloud", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if premium.Binding.Model == lower.Binding.Model || premium.Binding.Model == "" || lower.Binding.Model == "" {
		t.Errorf("the premium and lower tier name the same model: %q and %q", premium.Binding.Model, lower.Binding.Model)
	}
}

// One verdict names one model: every tier answers with the chosen binding, and
// the embeddings stay the preset's so grounding embeds as production does.
func TestEveryTierServesTheChosenModelAndEmbeddingsStayThePresets(t *testing.T) {
	chosen, err := ResolveChoice(gemini(t), "cheap_cloud", "", "")
	if err != nil {
		t.Fatal(err)
	}
	preset, err := ResolveChoice(gemini(t), "premium", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tier := range ai.AllTiers() {
		if got := chosen.routing.Tiers[tier]; got.Model != chosen.Binding.Model || got.Provider != chosen.Binding.Provider {
			t.Errorf("tier %s serves %s:%s, want the chosen %s", tier, got.Provider, got.Model, chosen.Binding.Model)
		}
	}
	if chosen.routing.Embeddings.Model != preset.routing.Embeddings.Model || chosen.routing.Embeddings.Model == chosen.Binding.Model {
		t.Errorf("embeddings = %q, want the preset's own", chosen.routing.Embeddings.Model)
	}
	if chosen.routing.Profile == "" {
		t.Error("the preset's profile was dropped")
	}
}

func TestANamedModelReplacesTheTierAndIsFiledAsNamed(t *testing.T) {
	got, err := ResolveChoice(gemini(t), "", "gemini:gemini-3.5-flash", "")
	if err != nil || got.Binding != (agentloope2e.Binding{Provider: "gemini", Model: "gemini-3.5-flash", Tier: NamedTier}) {
		t.Errorf("resolved %+v, %v", got.Binding, err)
	}
}

func TestAChoiceThatCannotBeMeasuredIsRefusedBeforeAnyCall(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.yaml")
	for name, call := range map[string]func() error{
		"tier and model both": func() error { _, err := ResolveChoice(gemini(t), "premium", "gemini:x", ""); return err },
		"an unknown tier":     func() error { _, err := ResolveChoice(gemini(t), "platinum", "", ""); return err },
		"a model without a provider": func() error {
			_, err := ResolveChoice(gemini(t), "", "just-a-name", "")
			return err
		},
		"a preset that is not there": func() error { _, err := ResolveChoice(missing, "", "", ""); return err },
	} {
		if err := call(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestTheModelPathBuildsOnlyWithTheProvidersKey(t *testing.T) {
	choice, err := ResolveChoice(gemini(t), "premium", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_API_KEY", "")
	if _, err := choice.ModelPath(&Recorder{}); err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("a route with no key built: %v", err)
	}
	t.Setenv("GEMINI_API_KEY", "unit-test-placeholder")
	path, err := choice.ModelPath(&Recorder{})
	if err != nil || path.AgentLoop == nil {
		t.Fatalf("the route did not build with a key set: %v", err)
	}
}

func TestTheLocalDatabaseCheckRefusesAnyOtherHost(t *testing.T) {
	for dsn, ok := range map[string]bool{
		"postgres://u:p@localhost:15432/margince":                    true,
		"postgres://u:p@127.0.0.1:5432/margince":                     true,
		"postgres://u:p@[::1]:5432/margince":                         true,
		"host=/var/run/postgresql dbname=margince":                   true,
		"postgres://u:p@db.internal.example:5432/m":                  false,
		"postgres://u:p@10.0.0.5:5432/margince":                      false,
		"postgres://u:p@localhost.example.com/margince":              false,
		"postgres://u:p@localhost:1,db.shared.example:5432/margince": false,
		"": false,
	} {
		err := RequireLocalDSN(dsn)
		if (err == nil) != ok {
			t.Errorf("%q: err = %v, want accepted = %v", dsn, err, ok)
		}
		if err != nil && strings.Contains(err.Error(), "p@") {
			t.Errorf("%q: the refusal repeats the password: %v", dsn, err)
		}
	}
}

func TestARecorderKeepsCallsAndSaysWhetherTheModelAnswered(t *testing.T) {
	rec := &Recorder{}
	if rec.Answered() {
		t.Error("an empty recorder says the model answered")
	}
	failed := ai.Call{IsTerminal: true, ErrorSentinel: "provider_error"}
	if err := rec.Record(context.Background(), []ai.Call{failed}); err != nil || rec.Answered() {
		t.Fatalf("a failed call counted as an answer: %v", err)
	}
	if err := rec.Record(context.Background(), []ai.Call{{IsTerminal: true}}); err != nil || !rec.Answered() {
		t.Fatalf("a served call did not count: %v", err)
	}
	if len(rec.Calls()) != 2 || rec.EnsureConfig(context.Background(), ai.ConfigSnapshot{}) != nil {
		t.Errorf("calls = %d", len(rec.Calls()))
	}
}

func TestATraceIsFiledByScenarioBindingAndRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trace")
	trace := Trace{Scenario: "s", Binding: testBinding, Run: 2, Status: "completed"}
	path, err := trace.Write(dir)
	if err != nil || filepath.Dir(path) != dir || !strings.HasSuffix(path, ".run2.json") {
		t.Fatalf("path = %q, %v", path, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back Trace
	if err := json.Unmarshal(body, &back); err != nil || back.Scenario != "s" || back.Run != 2 {
		t.Errorf("read back %+v, %v", back, err)
	}
	if _, err := trace.Write(filepath.Join(path, "inside-a-file")); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Errorf("a trace written under a file did not fail: %v", err)
	}
}

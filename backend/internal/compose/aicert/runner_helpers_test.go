// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The runner's small deterministic helpers, tested away from the router: how
// many repeats a run gets, how a per-task ladder override is applied, how the
// corpus is grouped, and the two folds a record's numbers come out of. None of
// them calls a model, so none of them needs one to be wrong in a way a reader
// would notice.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

func TestRepeatsOrDefault(t *testing.T) {
	cases := []struct {
		name    string
		in      int
		want    int
		wantErr bool
	}{
		{"zero defaults to three", 0, 3, false},
		{"valid odd", 5, 5, false},
		{"one is valid", 1, 1, false},
		{"even is refused", 4, 0, true},
		{"negative is refused", -1, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := repeatsOrDefault(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("repeatsOrDefault(%d): want an error, got %d", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("repeatsOrDefault(%d): unexpected error: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("repeatsOrDefault(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestLadderForTaskBindsEveryTierTheTaskCanFallThrough(t *testing.T) {
	binding := ai.ProviderConfig{Provider: "anthropic", Model: "claude-cert-test"}

	bound, err := ladderForTask("candidate", binding, ai.ProfileCloudFrontier, ai.TaskColdStart)
	if err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
	for _, tier := range ai.TaskLadder(ai.TaskColdStart) {
		if got := bound.Tiers[tier]; got.Provider != "anthropic" || got.Model != "claude-cert-test" {
			t.Errorf("tier %s = %+v, want the run's binding", tier, got)
		}
	}
	// EVERY tier, not just the ladder's rungs. The router demotes under budget
	// pressure onto tiers the ladder does not name, and an unbound one fails as
	// "no bound tier can serve" — which reads like the task being unsupported
	// rather than the binding being partial.
	if len(bound.Tiers) != len(ai.AllTiers()) {
		t.Errorf("bound %d tiers, want every one of the %d a binding may declare", len(bound.Tiers), len(ai.AllTiers()))
	}
	for _, tier := range ai.AllTiers() {
		if got := bound.Tiers[tier]; got.Provider == "" {
			t.Errorf("tier %s left unbound; a budget demote onto it would read as an unsupported task", tier)
		}
	}
}

func TestLadderForTaskRefusesATaskWithNoLadder(t *testing.T) {
	_, err := ladderForTask("candidate", ai.ProviderConfig{Provider: "anthropic", Model: "x"}, ai.ProfileCloudFrontier, ai.Task("not_a_real_task"))
	if err == nil || !strings.Contains(err.Error(), "no routing ladder") {
		t.Fatalf("want a no-routing-ladder complaint, got %v", err)
	}
}

// The broker case. SelectBrain fails closed on openai_compatible without a
// base_url, so the endpoint has to reach every rung of the ladder — this is the
// one the retired routing file used to carry, and losing it would make every
// OpenRouter certification unrunnable rather than merely wrong.
func TestLadderForTaskCarriesTheEndpointToEveryRung(t *testing.T) {
	binding := ai.ProviderConfig{
		Provider: "openai_compatible",
		Model:    "z-ai/glm-5.2",
		BaseURL:  "https://openrouter.ai/api",
	}
	bound, err := ladderForTask("candidate", binding, ai.ProfileCloudFrontier, ai.TaskColdStart)
	if err != nil {
		t.Fatalf("broker binding rejected: %v", err)
	}
	for _, tier := range ai.TaskLadder(ai.TaskColdStart) {
		if got := bound.Tiers[tier].BaseURL; got != "https://openrouter.ai/api" {
			t.Errorf("tier %s base_url = %q, want the binding's endpoint on every rung", tier, got)
		}
	}
}

// An override rebinds the tiers the profile rule is ABOUT, so the loaded
// file's guarantees do not survive it. Certifying a cloud model is a
// legitimate thing to want; doing it against a config that still says
// sovereign, with nothing said about it, produces numbers describing a
// deployment nobody has.
func TestLadderForTaskRefusesACloudModelUnderASovereignProfile(t *testing.T) {
	_, err := ladderForTask(
		"candidate",
		ai.ProviderConfig{Provider: "anthropic", Model: "claude-cert-test"},
		ai.ProfileSovereign, ai.TaskColdStart)
	if err == nil {
		t.Fatal("a cloud binding under profile sovereign was accepted; the run would have built the client and called api.anthropic.com")
	}
	// The refusal has to name three things or it is unactionable: WHICH SIDE is
	// misbound, which binding it is, and WHAT rule it broke. The side matters
	// because the candidate and the judge are set by different variables and are
	// validated through this same function — a refusal that named only the
	// binding sent a reader to fix whichever one they thought of first.
	if !strings.Contains(err.Error(), "candidate") {
		t.Errorf("the refusal must say which side is misbound, got %q", err)
	}
	if !strings.Contains(err.Error(), "anthropic:claude-cert-test") {
		t.Errorf("the refusal must name the binding that caused it, got %q", err)
	}
	if !strings.Contains(err.Error(), "profile sovereign forbids cloud provider") {
		t.Errorf("the refusal must carry the rule the config's own boot would have given, got %q", err)
	}
}

// The other arm, and the one that stops the check above from passing for the
// wrong reason: validating must not refuse a binding the profile allows.
func TestLadderForTaskAcceptsALocalModelUnderASovereignProfile(t *testing.T) {
	bound, err := ladderForTask(
		"candidate",
		ai.ProviderConfig{Provider: "ollama", Model: "qwen3:14b"},
		ai.ProfileSovereign, ai.TaskColdStart)
	if err != nil {
		t.Fatalf("a local binding under profile sovereign must run, got %v", err)
	}
	for _, tier := range ai.TaskLadder(ai.TaskColdStart) {
		if got := bound.Tiers[tier].Model; got != "qwen3:14b" {
			t.Errorf("tier %s model = %q, want the binding's model", tier, got)
		}
	}
}

func TestGroupByTaskFiltersAndSortedTasksOrdersDeterministically(t *testing.T) {
	scenarios := []Scenario{
		{Name: "a", Task: string(ai.TaskSummarize)},
		{Name: "b", Task: string(ai.TaskColdStart)},
		{Name: "c", Task: string(ai.TaskSummarize)},
	}
	all := groupByTask(scenarios, "")
	if len(all[ai.TaskSummarize]) != 2 || len(all[ai.TaskColdStart]) != 1 {
		t.Fatalf("unfiltered grouping = %+v", all)
	}
	filtered := groupByTask(scenarios, string(ai.TaskColdStart))
	if len(filtered) != 1 || len(filtered[ai.TaskColdStart]) != 1 {
		t.Fatalf("filtered grouping = %+v", filtered)
	}
	order := sortedTasks(all)
	if len(order) != 2 || order[0] != ai.TaskColdStart || order[1] != ai.TaskSummarize {
		t.Fatalf("sortedTasks = %v, want [cold_start summarize]", order)
	}
}

func TestWorstVerdictRanksNotSupportedBelowDegradedBelowCertified(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{VerdictCertified, VerdictNotSupported, VerdictNotSupported},
		{VerdictCertified, VerdictSupportedDegraded, VerdictSupportedDegraded},
		{VerdictSupportedDegraded, VerdictNotSupported, VerdictNotSupported},
		{VerdictCertified, VerdictCertified, VerdictCertified},
	}
	for _, c := range cases {
		if got := worstVerdict(c.a, c.b); got != c.want {
			t.Errorf("worstVerdict(%s, %s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

func TestPercentileNearestRank(t *testing.T) {
	sorted := []int64{10, 20, 30}
	if got := percentile(sorted, 0.50); got != 20 {
		t.Errorf("p50 of %v = %d, want 20", sorted, got)
	}
	if got := percentile(sorted, 0.95); got != 30 {
		t.Errorf("p95 of %v = %d, want 30", sorted, got)
	}
	if got := percentile(nil, 0.50); got != 0 {
		t.Errorf("percentile of an empty slice = %d, want 0", got)
	}
}

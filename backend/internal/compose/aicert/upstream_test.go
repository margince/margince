// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// errNoHostMatches is a broker's answer when its upstream filter leaves no host,
// as the adapter raises it; the ai package pins that against the wire.
var errNoHostMatches = fmt.Errorf("%w: ai: openai-compat: : No endpoints found that can handle the requested parameters. (http 404)",
	ai.ErrNoUpstreamHost)

// A preference no host can meet fails the same way on every attempt, so it is
// not re-driven as an outage, and the refusal names the variable that lifts it.
func TestAnUpstreamNoHostMeetsAbortsNamingTheVariable(t *testing.T) {
	for name, tc := range map[string]struct {
		candidate, judge *ai.FakeClient
		names            string
	}{
		"candidate": {candidate: candidateFailingEveryCall(t, errNoHostMatches), judge: ai.NewFakeClient(), names: "UPSTREAM="},
		"judge": {
			candidate: ai.NewFakeClient().Script(containsWidget, containsWidget, containsWidget),
			judge:     candidateFailingEveryCall(t, errNoHostMatches), names: "JUDGE_UPSTREAM=",
		},
	} {
		t.Run(name, func(t *testing.T) {
			waited := recordSleeps(t)
			_, err := certifyAgainst(t, tc.candidate, tc.judge)
			if err == nil || !strings.Contains(err.Error(), tc.names) {
				t.Fatalf("err = %v, want a refusal naming %s", err, tc.names)
			}
			if len(*waited) != 0 {
				t.Errorf("waited %v — no host appears for a filter that matched none", *waited)
			}
		})
	}
}

// The pre-flight asks each binding one small question before the corpus, so a
// binding that cannot be served costs seconds rather than a paid scenario.
func TestThePreflightAsksEachBindingOnceBeforeTheCorpus(t *testing.T) {
	cfg := RunnerConfig{
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		Profile:      ai.ProfileEUHosted,
	}
	tasks := []ai.Task{ai.TaskSummarize, ai.TaskColdStart}
	t.Run("healthy", func(t *testing.T) {
		candidate, judge := ai.NewFakeClient(), ai.NewFakeClient()
		if err := preflight(wsContext(t), cfg, tasks, fakeHooks(candidate, judge), quietLogger()); err != nil {
			t.Fatalf("a servable pair failed its pre-flight: %v", err)
		}
		if len(candidate.Calls()) != 1 || len(judge.Calls()) != 1 {
			t.Errorf("candidate=%d judge=%d calls, want one each — one candidate binding serves both tasks",
				len(candidate.Calls()), len(judge.Calls()))
		}
	})
	for name, tc := range map[string]struct {
		candidate, judge *ai.FakeClient
		names            []string
	}{
		"judge has no host": {ai.NewFakeClient(), candidateFailingEveryCall(t, errNoHostMatches), []string{"judge", "JUDGE_UPSTREAM="}},
		"candidate rejected": {
			candidateFailingEveryCall(t, fmt.Errorf("%w: bad key", model.ErrRequestRejected)), ai.NewFakeClient(),
			[]string{"candidate", "rejected"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := preflight(wsContext(t), cfg, tasks, fakeHooks(tc.candidate, tc.judge), quietLogger())
			if err == nil {
				t.Fatal("an unservable binding passed its pre-flight")
			}
			for _, want := range tc.names {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to name %q", err, want)
				}
			}
		})
	}
}

func fakeHooks(candidate, judge *ai.FakeClient) *certifyHooks {
	return &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
	}
}

// A preference block on a binding it cannot reach would be accepted and then
// applied to nothing, so the run refuses it naming the variable that set it.
func TestUpstreamPreferencesOnANonBrokerBindingAreRefused(t *testing.T) {
	t.Parallel()
	cfg := RunnerConfig{
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge", Routing: &ai.OpenRouterRouting{}},
		Profile:      ai.ProfileEUHosted,
	}
	err := validateBindings(cfg, []ai.Task{ai.TaskSummarize}, quietLogger())
	if err == nil || !strings.Contains(err.Error(), "MARGINCE_AICERT_JUDGE_UPSTREAM") {
		t.Fatalf("err = %v, want the judge's preferences refused by name", err)
	}
}

// A MODEL= run files its records under the profile it names, so a broker
// candidate under eu_hosted has to be pinned to EU hosts the way a parsed
// config is — otherwise the record claims EU inference for text the broker
// sent wherever it liked.
func TestAnEUHostedBrokerCandidateMustPinAnEURegion(t *testing.T) {
	t.Parallel()
	broker := func(routing *ai.OpenRouterRouting) ai.ProviderConfig {
		return ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/m", BaseURL: "https://openrouter.ai/api", Routing: routing}
	}
	judge := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}
	for name, tc := range map[string]struct {
		candidate ai.ProviderConfig
		profile   ai.Profile
		refused   bool
	}{
		"unpinned under eu_hosted":      {broker(nil), ai.ProfileEUHosted, true},
		"a non-EU pin under eu_hosted":  {broker(&ai.OpenRouterRouting{Only: []string{"mistral"}}), ai.ProfileEUHosted, true},
		"pinned to the EU":              {broker(&ai.OpenRouterRouting{Only: []string{"mistral/eu"}}), ai.ProfileEUHosted, false},
		"unpinned under cloud_frontier": {broker(nil), ai.ProfileCloudFrontier, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := RunnerConfig{Binding: tc.candidate, JudgeBinding: judge, Profile: tc.profile}
			err := validateBindings(cfg, []ai.Task{ai.TaskSummarize}, quietLogger())
			if !tc.refused {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "PROFILE=cloud_frontier") {
				t.Fatalf("err = %v, want the eu_hosted refusal naming the way out", err)
			}
		})
	}
}

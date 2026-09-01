// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The routed lane's two decisions, both made BEFORE any paid call: which model a
// task is certified against, and whether the run could produce a trustworthy
// verdict at all. Both are cheap to get wrong in a way that only shows up as a
// bill.

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

func devLikeRouting() ai.RoutingConfig {
	return ai.RoutingConfig{
		Profile: ai.ProfileEUHosted,
		Tiers: map[ai.Tier]ai.ProviderConfig{
			ai.TierLocalSmall: {Provider: "openai_compatible", Model: "openai/gpt-oss-120b", BaseURL: "https://broker.example/api"},
			ai.TierCheapCloud: {Provider: "openai_compatible", Model: "openai/gpt-oss-120b", BaseURL: "https://broker.example/api"},
			ai.TierPremium:    {Provider: "openai_compatible", Model: "vendor/big-1", BaseURL: "https://broker.example/api"},
		},
	}
}

// A task is certified against the model on its LEADING rung, because that is the
// model that serves it when nothing has gone wrong. Two tasks leading on
// different rungs therefore resolve to different models from ONE config, which
// is the whole reason the routed lane exists.
func TestResolveBindingTakesTheLeadingRungsModel(t *testing.T) {
	routing := devLikeRouting()

	local, _, ok := resolveBinding(routing, ai.TaskCaptureConfidentialityVerdict)
	if !ok {
		t.Fatal("the confidentiality verdict leads on local_small, which this config binds")
	}
	if local.Model != "openai/gpt-oss-120b" {
		t.Errorf("confidentiality verdict resolved to %q, want the local_small model", local.Model)
	}
	premium, _, ok := resolveBinding(routing, ai.TaskDocumentExtract)
	if !ok {
		t.Fatal("document_extract leads on premium, which this config binds")
	}
	if premium.Model != "vendor/big-1" {
		t.Errorf("document_extract resolved to %q, want the premium model", premium.Model)
	}
	if local.Model == premium.Model {
		t.Error("two tasks leading on different rungs resolved to the same model — the leading rung is not being read")
	}
}

// An unbound leading rung falls THROUGH to the next bound rung, because that is
// what production does: the router filters the ladder to bound rungs and serves
// the leading survivor. Refusing here would leave a task uncertified that the
// same deployment serves every day.
func TestResolveBindingFallsThroughToTheFirstBoundRung(t *testing.T) {
	routing := devLikeRouting()
	// brief_ranking's ladder is [premium, cheap_cloud]. Unbind its lead.
	delete(routing.Tiers, ai.TierPremium)
	binding, rung, ok := resolveBinding(routing, ai.TaskBriefRanking)
	if !ok {
		t.Fatal("cheap_cloud is bound and is on brief_ranking's ladder, so production would serve it there and certification must measure it")
	}
	if rung != ai.TierCheapCloud {
		t.Errorf("resolved rung = %q, want cheap_cloud — the first BOUND rung of the ladder", rung)
	}
	if binding.Model != "openai/gpt-oss-120b" {
		t.Errorf("resolved model = %q, want the cheap_cloud model", binding.Model)
	}
	// A rung present but incomplete is not a binding: a provider with no model
	// cannot be called, so the walk must keep going rather than resolve to it.
	routing.Tiers[ai.TierPremium] = ai.ProviderConfig{Provider: "openai_compatible"}
	_, rung, ok = resolveBinding(routing, ai.TaskBriefRanking)
	if !ok || rung != ai.TierCheapCloud {
		t.Errorf("a provider with no model is not a binding; resolved (%q, %v), want cheap_cloud", rung, ok)
	}
}

// No rung bound at all is the honest refusal — production could not serve the
// task either.
func TestResolveBindingRefusesWhenNoRungIsBound(t *testing.T) {
	routing := devLikeRouting()
	for _, tier := range ai.AllTiers() {
		delete(routing.Tiers, tier)
	}
	if _, _, ok := resolveBinding(routing, ai.TaskDocumentExtract); ok {
		t.Error("no rung is bound, so there is no model to certify against")
	}
}

// The candidate-is-not-the-judge check must run for EVERY task the routing
// resolves, before the first call. cert_judge leads on premium, so a config
// binding the judge's own model there collides with every premium-led candidate
// — and caught per task instead, it would surface after the earlier tasks had
// been paid for.
func TestValidateRoutedBindingsCatchesAJudgeCollisionUpFront(t *testing.T) {
	cfg := RunnerConfig{
		Routing: ptr(devLikeRouting()),
		// Exactly the premium binding above: the judge would be grading itself on
		// every premium-led task.
		JudgeBinding: ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/big-1", BaseURL: "https://broker.example/api"},
		Profile:      ai.ProfileEUHosted,
	}
	err := validateRoutedBindings(cfg, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("a judge bound to the same model as a premium-led candidate was accepted; the run would have paid for tasks before reaching the collision")
	}
	// The message has to name the colliding tasks, or an operator cannot tell
	// whether to move the judge or rebind a rung.
	for _, want := range []string{"vendor/big-1", "document_extract"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must name %q, got %q", want, err)
		}
	}
}

// A judge on a model no task leads on is fine, however many rungs the config
// binds — that is the ordinary case and must not be refused.
func TestValidateRoutedBindingsAcceptsADistinctJudge(t *testing.T) {
	cfg := RunnerConfig{
		Routing:      ptr(devLikeRouting()),
		JudgeBinding: ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/grader-9", BaseURL: "https://broker.example/api"},
		Profile:      ai.ProfileEUHosted,
	}
	if err := validateRoutedBindings(cfg, slog.New(slog.DiscardHandler)); err != nil {
		t.Errorf("a judge no task leads on must be accepted, got %v", err)
	}
}

// No judge at all is refused BEFORE the run, and the message says why the
// routing cannot supply one: cert_judge's own rung would collide.
func TestValidateRoutedBindingsRequiresAJudge(t *testing.T) {
	cfg := RunnerConfig{Routing: ptr(devLikeRouting()), Profile: ai.ProfileEUHosted}
	err := validateRoutedBindings(cfg, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("a routed run with no judge was accepted; nothing would have graded the candidate")
	}
	if !strings.Contains(err.Error(), "MARGINCE_AICERT_JUDGE_MODEL") {
		t.Errorf("the refusal must name the variable that fixes it, got %q", err)
	}
}

// An environment class the profile vocabulary does not carry is refused: a
// record is FILED under it, so a run that accepted one would write a record
// nobody can find by deployment.
func TestValidateRoutedBindingsRefusesAnUnknownProfile(t *testing.T) {
	routing := devLikeRouting()
	routing.Profile = ai.Profile("not_an_environment_class")
	cfg := RunnerConfig{
		Routing:      &routing,
		JudgeBinding: ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/grader-9", BaseURL: "https://broker.example/api"},
		Profile:      routing.Profile,
	}
	if err := validateRoutedBindings(cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Error("a profile outside the vocabulary was accepted; the record would name an environment class that does not exist")
	}
}

func ptr(r ai.RoutingConfig) *ai.RoutingConfig { return &r }

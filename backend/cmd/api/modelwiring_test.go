// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// resolveModelPath is the one place the api process runs the declared-
// routing/--ai-fake/neither switch (modelwiring.go); these tests exercise
// all three arms without a database. NewModelPath's construction path
// (ai.NewRouter → ai.NewMeter/compose.NewSeatBudget/ai.NewCallMeter) only
// stores the *pgxpool.Pool in each collaborator — it issues no query
// until a lane actually completes a call — so a nil pool is safe here.
// Proving the fake arm's Router actually TRACES a completed call through
// the real pool needs a live database; that lives in
// internal/compose/integration/ai_fake_modelpath_integration_test.go.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestResolveModelPathNeitherFlagIsUnconfigured proves the honest-absent
// case: no declared routing file and no --ai-fake resolves to a nil
// path and the unconfigured state, never a silent default provider.
func TestResolveModelPathNeitherFlagIsUnconfigured(t *testing.T) {
	modelPath, state, profile, _, err := modelPathFor(context.Background(), ai.RoutingConfig{}, modelPathSpec{}, nil, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if modelPath != nil {
		t.Fatalf("modelPath = %+v, want nil", modelPath)
	}
	if state != compose.AIStateUnconfigured {
		t.Fatalf("state = %q, want %q", state, compose.AIStateUnconfigured)
	}
	if profile.State != "unconfigured" || profile.InferenceMode != "none" || len(profile.Providers) != 0 {
		t.Fatalf("profile = %+v, want honest unconfigured posture", profile)
	}
}

// TestResolveModelPathFakeArmBindsEveryLane proves --ai-fake resolves a
// real *compose.ModelPath (built over ai.FakeRoutingConfig() through
// compose.NewModelPath, the same constructor the declared-routing arm
// uses) rather than bypassing the Router — every lane must be non-nil,
// or a consumer wired against it would nil-panic on first use.
func TestResolveModelPathFakeArmBindsEveryLane(t *testing.T) {
	modelPath, state, profile, _, err := modelPathFor(context.Background(), ai.RoutingConfig{}, modelPathSpec{fakeBrain: true}, nil, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if modelPath == nil {
		t.Fatal("modelPath is nil, want a bound path")
	}
	if state != compose.AIStateFake {
		t.Fatalf("state = %q, want %q", state, compose.AIStateFake)
	}
	if profile.State != "development" || profile.InferenceMode != "development" || len(profile.Providers) != 0 {
		t.Fatalf("profile = %+v, want development posture without a fake provider label", profile)
	}
	if modelPath.AgentLoop == nil {
		t.Error("AgentLoop lane is nil")
	}
	if modelPath.ColdStart == nil {
		t.Error("ColdStart lane is nil")
	}
	if modelPath.SiteExtract == nil {
		t.Error("SiteExtract lane is nil")
	}
	if modelPath.BriefRanking == nil {
		t.Error("BriefRanking lane is nil")
	}
	if modelPath.DraftReply == nil {
		t.Error("DraftReply lane is nil")
	}
	if modelPath.OfferDraft == nil {
		t.Error("OfferDraft lane is nil")
	}
	if modelPath.Embedder == nil {
		t.Error("Embedder lane is nil")
	}
}

// A stored binding this process cannot BUILD refuses the boot — unless the
// operator asked for the offline fake, which only a dev or test lane does.
//
// The combination is ordinary rather than exotic: a dev stack's bootstrap seeds a
// cloud binding from `seeds.ai_routing`, and the engineer running it may hold no
// key for that vendor. Because a servable stored binding outranks --ai-fake (the
// case above), the flag cannot rescue an unbuildable one either without this —
// which made `make dev` refuse to start over an AI lane nobody was using, while
// the flag on its own command line said what was wanted instead.
//
// Both halves are asserted, because the value is in the asymmetry: production
// never passes --ai-fake and must still fail closed on a binding it cannot serve.
func TestAnUnbuildableBindingFallsBackOnlyWhenTheFakeWasAskedFor(t *testing.T) {
	// Bound to a cloud vendor with no credential resolvable, which is exactly
	// what a keyless dev stack carries after bootstrap.
	cfg, err := ai.ParseRouting([]byte(`profile: cloud_frontier
tiers:
  local_small: {provider: gemini, model: gemini-3.1-flash-lite}
  cheap_cloud: {provider: gemini, model: gemini-3.1-flash-lite}
  premium: {provider: gemini, model: gemini-3.5-flash}
  frontier: {provider: gemini, model: gemini-3.5-flash}
embeddings: {provider: gemini, model: gemini-embedding-001, dimensions: 8}
`))
	if err != nil {
		t.Fatalf("parsing the keyless cloud binding: %v", err)
	}

	// Without the flag: a boot error, and it must name the missing credential
	// rather than starting on something the operator did not choose.
	if _, _, _, _, err := modelPathFor(
		context.Background(), cfg, modelPathSpec{}, nil, discardLogger()); err == nil {
		t.Fatal("a binding with no resolvable credential booted; a deployment must fail closed on it")
	}

	// With it: the fake serves, and the state SAYS fake so nothing downstream
	// reports canned text as a configured installation.
	path, state, profile, _, err := modelPathFor(
		context.Background(), cfg, modelPathSpec{fakeBrain: true}, nil, discardLogger())
	if err != nil {
		t.Fatalf("--ai-fake did not rescue an unbuildable binding: %v", err)
	}
	if path == nil {
		t.Fatal("no model path was wired")
	}
	if state != compose.AIStateFake {
		t.Errorf("state = %q, want %q: a fallback that reports itself as configured hides canned answers", state, compose.AIStateFake)
	}
	// The same development posture the plain fake arm reports, and no provider
	// label: the profile a reader sees must not name the vendor whose binding
	// could not be built.
	if profile.State != "development" || profile.InferenceMode != "development" || len(profile.Providers) != 0 {
		t.Errorf("profile = %+v, want the development posture without a provider label", profile)
	}
}

// A STORED binding outranks --ai-fake, and nothing else in this package says so.
//
// The desktop launcher leans on exactly this: it passes --ai-fake on every boot
// so a fresh install's AI surfaces answer instead of reading as broken, and it
// relies on a binding the user sets in Settings -> AI overriding that without a
// restart. Were the precedence the other way round, the launcher would pin every
// desktop installation to canned answers and the settings screen would look
// broken while being obeyed.
func TestBoundRoutingOutranksTheFakeBrain(t *testing.T) {
	cfg := fakeRouting(t)
	_, state, profile, _, err := modelPathFor(
		context.Background(), cfg, modelPathSpec{fakeBrain: true}, nil, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != compose.AIStateConfigured {
		t.Fatalf("state = %q, want %q: --ai-fake must not displace a stored binding", state, compose.AIStateConfigured)
	}
	if profile.State != "configured" {
		t.Fatalf("profile.State = %q, want configured", profile.State)
	}
}

// A bound installation resolves the same shape the fake arm does — one Router,
// every lane bound — over an offline (provider: fake) binding, so the case needs
// no external credential and no network access.
func TestResolveModelPathBoundArmBindsEveryLane(t *testing.T) {
	cfg := fakeRouting(t)
	modelPath, state, profile, _, err := modelPathFor(context.Background(), cfg, modelPathSpec{}, nil, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if modelPath == nil {
		t.Fatal("modelPath is nil, want a bound path")
	}
	if state != compose.AIStateConfigured {
		t.Fatalf("state = %q, want %q", state, compose.AIStateConfigured)
	}
	if profile.State != "configured" || profile.InferenceMode != "development" || len(profile.Providers) != 0 {
		t.Fatalf("profile = %+v, want configured fake-file posture without a provider label", profile)
	}
	// Every lane, because that is what the comment claims and what the fake arm
	// checks — a bound installation that wires six of seven is the defect this
	// case exists to catch, and asserting one of them would not see it.
	for name, lane := range map[string]any{
		"AgentLoop":    modelPath.AgentLoop,
		"ColdStart":    modelPath.ColdStart,
		"SiteExtract":  modelPath.SiteExtract,
		"BriefRanking": modelPath.BriefRanking,
		"DraftReply":   modelPath.DraftReply,
		"OfferDraft":   modelPath.OfferDraft,
		"Embedder":     modelPath.Embedder,
	} {
		if lane == nil {
			t.Errorf("%s lane is nil", name)
		}
	}
}

// TestColdStartOptionsRespectsResolvedPath proves coldStartOptions is a
// pure consumer of the resolved path now: nil in, nil out (the 501
// posture); a bound path in, the cold-start/scrape/brief/dossier/growth-fit/reply
// set out, plus the account-started and person-side drafts and the buying-role
// reading.
func TestColdStartOptionsRespectsResolvedPath(t *testing.T) {
	if got := coldStartOptions(nil, ""); got != nil {
		t.Fatalf("coldStartOptions(nil) = %d options, want 0", len(got))
	}
	modelPath, _, _, _, err := modelPathFor(context.Background(), ai.RoutingConfig{}, modelPathSpec{fakeBrain: true}, nil, discardLogger())
	if err != nil {
		t.Fatalf("resolveModelPath: %v", err)
	}
	if got := coldStartOptions(modelPath, ""); len(got) != 14 {
		t.Fatalf(
			"coldStartOptions(bound path) = %d options, want 14 (cold-start, scrape, morning brief, "+
				"account brief, company dossier, growth fit, reply draft, account draft, person draft, "+
				"next move, role proposals, intro request, intro note)",
			len(got))
	}
}

// TestOfferDraftOptionsRespectsResolvedPath mirrors
// TestColdStartOptionsRespectsResolvedPath for the offer-draft surface.
func TestOfferDraftOptionsRespectsResolvedPath(t *testing.T) {
	if got := offerDraftOptions(nil, nil); got != nil {
		t.Fatalf("offerDraftOptions(nil) = %d options, want 0", len(got))
	}
	modelPath, _, _, _, err := modelPathFor(context.Background(), ai.RoutingConfig{}, modelPathSpec{fakeBrain: true}, nil, discardLogger())
	if err != nil {
		t.Fatalf("resolveModelPath: %v", err)
	}
	if got := offerDraftOptions(nil, modelPath); len(got) != 1 {
		t.Fatalf("offerDraftOptions(bound path) = %d options, want 1 (offer draft)", len(got))
	}
}

// fakeRouting parses a fully offline binding (every tier + embeddings on the
// fake provider) so the bound arm can be exercised with no credential and no
// network. Parsed from bytes rather than written to a file and read back: the
// routing file is retired, and the shape is what these cases are about.
func fakeRouting(t *testing.T) ai.RoutingConfig {
	t.Helper()
	const routing = `profile: eu_hosted
tiers:
  local_small: { provider: fake }
  cheap_cloud: { provider: fake }
  premium: { provider: fake }
embeddings:
  provider: fake
`
	cfg, err := ai.ParseRouting([]byte(routing))
	if err != nil {
		t.Fatalf("parsing the offline binding: %v", err)
	}
	return cfg
}

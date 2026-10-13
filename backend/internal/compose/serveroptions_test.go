// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Option's own wiring-only unit-level proof: each Option is a plain
// field assignment/rebuild that never touches the pool at construction
// time (the module constructors it calls — approvals.NewService,
// contacts.NewStore, briefs.Handlers.WithL2Ranker — all just store the
// pool reference for later use), so a nil pool and nil PageFetcher/Brain
// are safe here: this test never calls the wired handler, only proves
// the Option actually set what it documents.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestWithBusReadySetsTheProbe(t *testing.T) {
	s := &Server{}
	var called bool
	WithBusReady(func(ctx context.Context) error { called = true; return nil })(s, nil)

	if s.busReady == nil {
		t.Fatal("WithBusReady did not set busReady")
	}
	if err := s.busReady(context.Background()); err != nil {
		t.Fatalf("busReady: unexpected error: %v", err)
	}
	if !called {
		t.Fatal("busReady did not invoke the check function WithBusReady was given")
	}
}

func TestWithCaptureConfigRecordsTheDeployedLists(t *testing.T) {
	s := &Server{}
	cfg := CaptureConfig{
		TransactionalExtra: []string{"esp.example"},
		TransactionalNever: []string{"realco.example"},
	}
	WithCaptureConfig(cfg)(s, nil)

	if len(s.captureConfig.TransactionalExtra) != 1 || s.captureConfig.TransactionalExtra[0] != "esp.example" {
		t.Fatalf("captureConfig = %+v, want the deployed lists recorded so the vault/graph registries apply them", s.captureConfig)
	}
	if len(s.captureConfig.TransactionalNever) != 1 {
		t.Fatalf("captureConfig dropped a list: %+v", s.captureConfig)
	}
}

func TestWithColdStartWiresTheEngine(t *testing.T) {
	s := &Server{}
	WithColdStart(nil, nil)(s, nil)
	if s.coldstartHandlers.engine == nil {
		t.Fatal("WithColdStart did not wire a coldStartEngine")
	}
}

func TestWithScrapeWiresTheEngine(t *testing.T) {
	s := &Server{}
	WithScrape(nil, nil)(s, nil)
	if s.scrapeHandlers.engine == nil {
		t.Fatal("WithScrape did not wire a scrapeEngine")
	}
}

// insertOnlyRunnerForTest builds a real *jobs.Runner over a nil pool —
// jobs.NewInserter only stores the pool reference (River's client
// construction never dials), so this is safe at Option-construction time,
// the same "never touches the pool at construction" precedent this file's
// header comment already documents for the module constructors.
func insertOnlyRunnerForTest(t *testing.T) *jobs.Runner {
	t.Helper()
	inserter, err := jobs.NewInserter(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	return inserter
}

// An unbound embed lane (--ai-fake, or routing with no embeddings model) answers
// 501, not a 500 on a marker seedEmbedBinding never planted. /me offers nothing.
func TestAnUnboundLaneAnswers501AndOffersNoReindexSurface(t *testing.T) {
	s := &Server{}
	cfg := ai.RoutingConfig{
		Profile: ai.ProfileEUHosted,
		Tiers:   map[ai.Tier]ai.ProviderConfig{ai.TierLocalSmall: {Provider: ai.ProviderFake}},
		// Embeddings.Model deliberately left empty: the unbound-lane shape
		// (routing_bind.go only stamps TierEmbedLane when Model is
		// non-empty), mirroring ai.FakeRoutingConfig()'s own --ai-fake
		// posture.
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: ai.ProviderConfig{Provider: ai.ProviderFake}},
	}
	router, err := ai.NewLocalRouter(cfg)
	if err != nil {
		t.Fatalf("NewLocalRouter: %v", err)
	}
	if identity, _ := router.EmbedIdentity(); identity != "" {
		t.Fatalf("test setup did not produce an unbound router, got identity %q", identity)
	}

	WithEmbedReindex(router, insertOnlyRunnerForTest(t))(s, nil)

	w := httptest.NewRecorder()
	s.EmbedReindexStatus(w, httptest.NewRequest(http.MethodGet, "/v1/embeddings/reindex/status", nil))
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status on an unbound lane = %d, want 501: %s", w.Code, w.Body.String())
	}
	if s.EmbedReindexAvailable() {
		t.Error("/me advertises the reindex surface on an unbound embed lane")
	}
	assertMeAgreesWithTheReindexEngine(t, s)
}

// TestWithEmbedReindexWiresTheEngineWhenBound is the control: a router
// whose embed lane IS bound must still wire the engine as before — the
// new identity=="" gate must not swallow the legitimate configured case.
func TestWithEmbedReindexWiresTheEngineWhenBound(t *testing.T) {
	s := &Server{}
	cfg := ai.RoutingConfig{
		Profile: ai.ProfileEUHosted,
		Tiers:   map[ai.Tier]ai.ProviderConfig{ai.TierLocalSmall: {Provider: ai.ProviderFake}},
		Embeddings: ai.EmbeddingsConfig{
			ProviderConfig: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "bound-model"},
			Dimensions:     1024,
		},
	}
	router, err := ai.NewLocalRouter(cfg)
	if err != nil {
		t.Fatalf("NewLocalRouter: %v", err)
	}
	if identity, _ := router.EmbedIdentity(); identity == "" {
		t.Fatal("test setup did not produce a bound router")
	}

	WithEmbedReindex(router, insertOnlyRunnerForTest(t))(s, nil)

	if s.embedReindexHandlers.engine == nil {
		t.Fatal("WithEmbedReindex must wire the engine when the embed lane is bound")
	}
	assertMeAgreesWithTheReindexEngine(t, s)
}

// /me offers the reindex surface when, and only when, its routes serve. Either
// disagreement is silent: a settings entry that 501s, or an unreachable lane.
func assertMeAgreesWithTheReindexEngine(t *testing.T, s *Server) {
	t.Helper()
	advertised, serving := s.EmbedReindexAvailable(), s.serving()
	if advertised != serving {
		t.Errorf("/me advertises embedding_reindex=%v while the reindex routes serve=%v", advertised, serving)
	}
}

// A role with no router takes the option's first early return and reports the
// surface absent, the same answer its generated 501 routes give.
func TestMeReportsNoReindexSurfaceWithoutARouter(t *testing.T) {
	s := &Server{}
	WithEmbedReindex(nil, insertOnlyRunnerForTest(t))(s, nil)

	if s.EmbedReindexAvailable() {
		t.Error("/me advertises the reindex surface on a server with no router")
	}
	assertMeAgreesWithTheReindexEngine(t, s)
}

// A server that never ran the option at all agrees with itself too: the zero
// value is absent and so is the engine, with no post-loop publish needed.
func TestMeReportsNoReindexSurfaceWhenTheOptionNeverRan(t *testing.T) {
	s := &Server{}

	if s.EmbedReindexAvailable() {
		t.Error("/me advertises the reindex surface on a server that never wired it")
	}
	assertMeAgreesWithTheReindexEngine(t, s)
}

// TestReadyzEmbedStateUnboundLaneReportsUnknownWithoutReadingTheMarker is
// the /readyz half of the F1 fix: an engine wired over an unbound embed
// lane (identity == "") must report "unknown" — the same line an
// engine-nil role reports — WITHOUT calling PopulatedIdentity at all.
// store is deliberately backed by a nil pool: if the identity=="" early
// return ever regresses, the subsequent marker read panics on the nil
// pool instead of quietly returning the wrong status, so this test fails
// loudly rather than passing on a coincidentally-empty error path.
func TestReadyzEmbedStateUnboundLaneReportsUnknownWithoutReadingTheMarker(t *testing.T) {
	s := Server{embedReindexHandlers: embedReindexHandlers{engine: &embedReindexEngine{
		store:    search.NewStore(nil),
		embedder: unboundTestEmbedder{},
	}}}

	got := s.readyzEmbedState()(context.Background())
	if got != embedStateUnknown {
		t.Fatalf("readyzEmbedState on an unbound lane = %q, want %q", got, embedStateUnknown)
	}
}

// unboundTestEmbedder is a minimal search.Embedder reporting the unbound
// ("", 0) identity — Embed is never expected to be called by anything
// this file exercises, so it is left unimplemented via a panic rather
// than a fuller fake.
type unboundTestEmbedder struct{}

func (unboundTestEmbedder) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	panic("Embed must never be called by readyzEmbedState")
}

func (unboundTestEmbedder) EmbedIdentity() (string, int) { return "", 0 }

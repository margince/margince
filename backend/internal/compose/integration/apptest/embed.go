// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// FakeEmbedDims is the width of every named test binding unless a test wants a
// mismatch. It matches ai.FakeClient's own default.
const FakeEmbedDims = 1024

// EmbedReindexRouter builds a DB-less local router over the offline fake, bound
// to the embeddings model modelName. An unnamed model reports the unbound "", 0
// identity, and two names yield the two identities a drift scenario needs.
func EmbedReindexRouter(t *testing.T, modelName string) *ai.Router {
	t.Helper()
	modelPath, err := compose.NewLocalModelPath(BoundEmbedRouting(modelName), ai.WithoutResultCache())
	if err != nil {
		t.Fatalf("NewLocalModelPath: %v", err)
	}
	return modelPath.Router()
}

// BoundEmbedRouting is the --ai-fake routing with modelName bound on the embed
// lane.
func BoundEmbedRouting(modelName string) ai.RoutingConfig {
	cfg := ai.FakeRoutingConfig()
	cfg.Embeddings = ai.EmbeddingsConfig{
		Provider: ai.ProviderFake, Model: modelName,
		Dimensions: FakeEmbedDims,
	}
	return cfg
}

// SeedRouterBinding plants the embed binding marker at router's identity, as
// the api's boot does; the harness boots without that step.
func SeedRouterBinding(t *testing.T, e *AppEnv, router *ai.Router) {
	t.Helper()
	identity, _ := router.EmbedIdentity()
	if err := search.NewStore(e.DB()).SeedBinding(context.Background(), identity); err != nil {
		t.Fatalf("SeedBinding: %v", err)
	}
}

// InsertOnlyRunner is the api role's River client: it enqueues and works
// nothing, on a pool of its own reaching the same Postgres.
func InsertOnlyRunner(t *testing.T) *jobs.Runner {
	t.Helper()
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if appDSN == "" {
		t.Fatal("MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	// Not the pool SetupAppWithOptions opens: jobs.NewInserter needs only
	// some pool reaching the same Postgres.
	wirePool, err := testdb.OwnPool(context.Background(), appDSN)
	if err != nil {
		t.Fatalf("opening the insert-only wiring pool: %v", err)
	}
	t.Cleanup(wirePool.Close)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	inserter, err := jobs.NewInserter(wirePool, quiet)
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	return inserter
}

// AskableEmbedder reports a binding, so retrieval is possible and the outcome
// is decided by the corpus rather than by the installation.
type AskableEmbedder struct{}

// EmbedIdentity reports a bound four-wide model.
func (AskableEmbedder) EmbedIdentity() (string, int) { return "fake/http@4", 4 }

// Embed answers one unit vector per input.
func (AskableEmbedder) Embed(_ context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	vectors := make([][]float32, len(req.Inputs))
	for i := range vectors {
		vectors[i] = []float32{1, 0, 0, 0}
	}
	return model.Embeddings{Vectors: vectors, Dims: 4}, nil
}

// QuietAskLog is the logger the engine degrades through. It discards: the
// degrade line reports a fallback, and a suite that printed it would bury its
// own assertion.
func QuietAskLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

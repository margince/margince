// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// seedRecorder is the production retriever with each query and its error kept.
// The error tells search's own refusal from the database a unit test lacks.
type seedRecorder struct {
	retrieval.Retriever
	queries []string
	errs    []error
}

func (r *seedRecorder) Search(ctx context.Context, q retrieval.Query) (retrieval.Result, error) {
	r.queries = append(r.queries, q.Text)
	found, err := r.Retriever.Search(ctx, q)
	r.errs = append(r.errs, err)
	return found, err
}

// A run whose seed query search refuses still completes, so only a log line
// shows it ran ungrounded. Each scheduled agent is held here against
// the real search refusals, through seedGrounding as executeJob calls it.
func TestEachScheduledAgentSeedsWithAQuerySearchAccepts(t *testing.T) {
	agents := mustScheduledAgents()
	if len(agents) == 0 {
		t.Fatal("no scheduled agents — this gate checked nothing")
	}
	for _, spec := range agents {
		t.Run(spec.Name, func(t *testing.T) {
			recorder := &seedRecorder{Retriever: search.NewRetriever(search.NewStore(nil), nil)}
			svc := &RunnerService{retriever: recorder, log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))}

			svc.seedGrounding(context.Background(), spec)

			if len(recorder.queries) != 1 {
				t.Fatalf("seeding searched %d times, want once", len(recorder.queries))
			}
			if refused, ok := errors.AsType[*search.BadQueryError](recorder.errs[0]); ok {
				t.Errorf("search refuses the seed query %q: %v — every run of %s would start ungrounded",
					recorder.queries[0], refused, spec.Name)
			}
		})
	}
}

// A spec with no seed query runs ungrounded by choice. It must not reach search,
// which refuses an empty query and logs it.
func TestASpecWithoutASeedQueryDoesNotSearch(t *testing.T) {
	recorder := &seedRecorder{Retriever: search.NewRetriever(search.NewStore(nil), nil)}
	svc := &RunnerService{retriever: recorder, log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))}

	if got := svc.seedGrounding(context.Background(), runner.AgentSpec{Name: "unseeded", Goal: "do a thing"}); got != nil {
		t.Errorf("an unseeded spec was grounded with %v", got)
	}
	if len(recorder.queries) != 0 {
		t.Errorf("an unseeded spec searched for %q", recorder.queries)
	}
}

// foundRetriever answers every search with one fixed hit.
type foundRetriever struct {
	retrieval.Retriever
	found retrieval.Result
}

func (r foundRetriever) Search(context.Context, retrieval.Query) (retrieval.Result, error) {
	return r.found, nil
}

// Each piece of evidence a hit carries becomes one T2 seed, under its source.
func TestEachEvidenceOfAHitBecomesAT2Seed(t *testing.T) {
	found := retrieval.Result{Hits: []retrieval.Hit{{Evidence: []retrieval.Evidence{
		{Source: "activity:a", Snippet: "Follow up with the new lead"},
		{Source: "company:b", Snippet: "Stalled Harbor Logistics"},
	}}}}
	svc := &RunnerService{retriever: foundRetriever{found: found}}

	got := svc.seedGrounding(context.Background(), runner.AgentSpec{SeedQuery: "follow up"})

	want := []runner.Grounding{
		{SourceID: "activity:a", TrustTier: "T2", Content: "Follow up with the new lead"},
		{SourceID: "company:b", TrustTier: "T2", Content: "Stalled Harbor Logistics"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("seeded %+v, want %+v", got, want)
	}
}

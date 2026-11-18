// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

type modelProbe struct{ ran *int }

func (modelProbe) Kind() string { return "model_probe" }

func (p modelProbe) Work(context.Context, *river.Job[modelProbe]) error {
	*p.ran++
	return nil
}

// A worker booted with no binding runs its model jobs as clean no-ops, and the
// same jobs work once the routing watcher binds the path they were wired from.
func TestModelJobsAreQuietUntilTheBootPathIsBound(t *testing.T) {
	path, err := compose.NewModelPath(context.Background(), ai.RoutingConfig{}, nil, false, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewModelPath: %v", err)
	}
	bound, err := ai.ParseRouting([]byte(`profile: eu_hosted
tiers:
  premium: {provider: fake, model: saved-large}
embeddings: {provider: fake}
`))
	if err != nil {
		t.Fatalf("ParseRouting: %v", err)
	}
	kinds := []string{"capture_classify", "capture_enrich", "embed_drift_sweep", "agent_scheduler"}
	ran := make([]int, len(kinds))
	workers := make([]jobs.WorkOnly[modelProbe], len(kinds))
	for i, kind := range kinds {
		spec, declared := jobs.SpecFor(kind)
		if !declared {
			t.Fatalf("%s is not a declared kind", kind)
		}
		workers[i] = jobs.QuietWhileUnbound[modelProbe](modelProbe{ran: &ran[i]}, spec, path.Bound)
	}
	work := func(i int) error { return workers[i].Work(context.Background(), &river.Job[modelProbe]{}) }
	for i, kind := range kinds {
		if err := work(i); err != nil || ran[i] != 0 {
			t.Errorf("%s unbound: Work = %v, ran %d times; want a clean no-op", kind, err, ran[i])
		}
	}
	if err := path.Router().Rebind(bound); err != nil {
		t.Fatalf("Rebind: %v", err)
	}
	for i, kind := range kinds {
		if err := work(i); err != nil || ran[i] != 1 {
			t.Errorf("%s after binding: Work = %v, ran %d times; want the job worked", kind, err, ran[i])
		}
	}
}

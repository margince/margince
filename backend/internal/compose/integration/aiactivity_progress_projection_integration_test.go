// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The projection keeps a live occurrence's lease and count monotonic at an
// equal (attempt, state), whatever order the bus delivers its events in.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type progressProbe struct {
	t       *testing.T
	e       *Env
	store   *aiactivity.Store
	key     string
	queued  time.Time
	started time.Time
}

func newProgressProbe(t *testing.T) *progressProbe {
	t.Helper()
	e := Setup(t)
	started := clocktest.Now(t).UTC().Truncate(time.Second)
	return &progressProbe{
		t: t, e: e, store: aiactivity.NewStore(e.DB()),
		key: ids.NewV7().String(), queued: started.Add(-time.Minute), started: started,
	}
}

func (p *progressProbe) apply(attempt int, state string, lease time.Duration, processed, total *int) bool {
	p.t.Helper()
	change := aiactivity.Change{
		Source: "progress_probe", OccurrenceKey: p.key, Kind: "site_read", Attempt: attempt,
		ActorScope: aiactivity.ScopePersonal, ActorUserID: p.e.Rep1,
		State: state, QueuedAt: p.queued, Processed: processed, Total: total,
	}
	switch state {
	case "running":
		change.StartedAt = &p.started
		stale := p.started.Add(lease)
		change.StaleAfter = &stale
	case "done":
		change.StartedAt = &p.started
		finished := p.started.Add(time.Minute)
		change.FinishedAt = &finished
	}
	applied, err := p.store.ApplyStateChange(p.t.Context(), change)
	if err != nil {
		p.t.Fatalf("ApplyStateChange(%s attempt %d): %v", state, attempt, err)
	}
	return applied
}

func (p *progressProbe) row() (processed, total *int, staleAfter *time.Time) {
	p.t.Helper()
	err := p.e.Pool.QueryRow(p.t.Context(), `
		SELECT progress_processed, progress_total, stale_after FROM ai_task_run
		 WHERE source = 'progress_probe' AND occurrence_key = $1`, p.key).Scan(&processed, &total, &staleAfter)
	if err != nil {
		p.t.Fatalf("reading the projected row: %v", err)
	}
	return processed, total, staleAfter
}

func (p *progressProbe) want(processed *int, lease time.Duration) {
	p.t.Helper()
	got, _, stale := p.row()
	if (got == nil) != (processed == nil) || (got != nil && *got != *processed) {
		p.t.Fatalf("processed = %v, want %v", got, processed)
	}
	if stale == nil || !stale.Equal(p.started.Add(lease)) {
		p.t.Fatalf("stale_after = %v, want the claim plus %v", stale, lease)
	}
}

func TestADelayedLeaseRenewalKeepsNewerProgress(t *testing.T) {
	t.Parallel()
	p := newProgressProbe(t)
	p.apply(1, "running", time.Minute, new(3), nil)
	if !p.apply(1, "running", time.Hour, nil, nil) {
		t.Fatal("a longer lease at an equal tuple must still land")
	}
	p.want(new(3), time.Hour)
}

func TestADelayedProgressEventKeepsTheLongerLease(t *testing.T) {
	t.Parallel()
	p := newProgressProbe(t)
	p.apply(1, "running", time.Hour, nil, nil)
	if !p.apply(1, "running", time.Minute, new(3), nil) {
		t.Fatal("a higher count at an equal tuple must still land")
	}
	p.want(new(3), time.Hour)
}

func TestAnExactReplayAndALowerCountChangeNothing(t *testing.T) {
	t.Parallel()
	p := newProgressProbe(t)
	p.apply(1, "running", time.Hour, new(5), nil)
	if p.apply(1, "running", time.Hour, new(5), nil) {
		t.Fatal("an exact replay was applied")
	}
	if p.apply(1, "running", time.Hour, new(2), nil) {
		t.Fatal("a lower count was applied")
	}
	p.want(new(5), time.Hour)
}

func TestSettlingClearsTheCountsAndANewAttemptStartsWithNone(t *testing.T) {
	t.Parallel()
	p := newProgressProbe(t)
	p.apply(1, "running", time.Hour, new(5), new(10))
	p.apply(1, "done", 0, nil, nil)
	processed, total, _ := p.row()
	if processed != nil || total != nil {
		t.Fatalf("a settled row holds %v of %v, want none", processed, total)
	}

	q := newProgressProbe(t)
	q.apply(1, "running", time.Hour, new(5), new(10))
	q.apply(2, "running", time.Hour, nil, nil)
	processed, total, _ = q.row()
	if processed != nil || total != nil {
		t.Fatalf("a new attempt inherited %v of %v from the previous one", processed, total)
	}
}

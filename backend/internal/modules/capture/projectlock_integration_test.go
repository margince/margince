// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// The ladder files a new mail inside the capture transaction, which already
// holds locks. Two captures naming one project must not deadlock on it. A lock
// cycle the ladder meets reruns the whole capture and is not kept as a fault.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// meetingMatcher holds its first two callers until both have arrived, so two
// captures reach the project write together. Later calls pass at once.
type meetingMatcher struct {
	project ids.UUID
	calls   atomic.Int32
	arrived chan struct{}
	both    chan struct{}
	once    sync.Once
	ctx     context.Context
}

func (m *meetingMatcher) MatchProjectKey(ctx context.Context, tx pgx.Tx, _ []string) (ids.UUID, error) {
	// A workspace-readable mail moves the project's clock, which locks the
	// project row; a held one does not. This stands in for a shared mailbox.
	if _, err := tx.Exec(ctx, `UPDATE activity SET audience = 'workspace', audience_reason = NULL
		WHERE source_id LIKE 'lock-%' AND audience <> 'workspace'`); err != nil {
		return ids.Nil, err
	}
	if m.calls.Add(1) > 2 {
		return m.project, nil
	}
	m.arrived <- struct{}{}
	if len(m.arrived) == 2 {
		m.once.Do(func() { close(m.both) })
	}
	select {
	case <-m.both:
		return m.project, nil
	case <-m.ctx.Done():
		return ids.Nil, m.ctx.Err()
	}
}

// cycleOnceMatcher answers a broken deadlock first, as Postgres does, and the
// project after that.
type cycleOnceMatcher struct {
	project ids.UUID
	calls   int
}

func (m *cycleOnceMatcher) MatchProjectKey(_ context.Context, _ pgx.Tx, _ []string) (ids.UUID, error) {
	m.calls++
	if m.calls == 1 {
		return ids.Nil, &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
	}
	return m.project, nil
}

func projectSink(t *testing.T, matcher capture.ProjectKeyMatcher) (*capture.Sink, context.Context, *pgx.Conn) {
	t.Helper()
	owner, pool := setupCaptureDB(t)
	ws := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))).
		WithProjectAttribution(capture.ProjectAttribution{
			Keys:  matcher,
			Stamp: func(context.Context, pgx.Tx, ids.ActivityID, ids.UUID) error { return nil },
		})
	return sink, captureSinkContextFor(context.Background(), ws), owner
}

// recordFrom is the project-subject mail with its own message and sender.
func recordFrom(sourceID, sender string) connector.NormalizedRecord {
	rec := aProjectSubjectRecord()
	rec.NaturalKey.SourceID = sourceID
	rec.Source = "gmail:" + sourceID
	rec.Counterparty.Email = sender
	rec.ThreadKey = "gmail:" + sourceID
	return rec
}

func linksOf(t *testing.T, owner *pgx.Conn, sourceIDs ...string) int {
	t.Helper()
	var n int
	if err := owner.QueryRow(context.Background(), `SELECT count(*) FROM activity_link l
		JOIN activity a ON a.id = l.activity_id
		WHERE l.entity_type = 'project' AND a.source_id = ANY($1)`, sourceIDs).Scan(&n); err != nil {
		t.Fatalf("counting project links: %v", err)
	}
	return n
}

func TestTwoCapturesNamingOneProjectAtOnceBothFileWithoutALockCycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	matcher := &meetingMatcher{arrived: make(chan struct{}, 2), both: make(chan struct{}), ctx: ctx}
	sink, sinkCtx, owner := projectSink(t, matcher)
	matcher.project = seedProjectForCapture(t, owner, "lock")

	sources := []string{"lock-a", "lock-b"}
	errs := make([]error, len(sources))
	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Go(func() {
			_, errs[i] = sink.Upsert(sinkCtx, recordFrom(source, source+"@lock.test"))
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("a capture failed: %v", err)
	}
	if n := linksOf(t, owner, sources...); n != 2 {
		t.Fatalf("%d of the two mails are filed under the project, want both", n)
	}
	if calls := matcher.calls.Load(); calls != 2 {
		t.Errorf("the ladder ran %d times for two mails, want 2: a capture met a lock cycle and ran again", calls)
	}
}

func TestALockCycleInTheLadderRerunsTheCapture(t *testing.T) {
	matcher := &cycleOnceMatcher{}
	sink, sinkCtx, owner := projectSink(t, matcher)
	matcher.project = seedProjectForCapture(t, owner, "cycle")

	if _, err := sink.Upsert(sinkCtx, recordFrom("cycle-1", "cycle@lock.test")); err != nil {
		t.Fatalf("the capture failed: %v", err)
	}
	if n := linksOf(t, owner, "cycle-1"); n != 1 {
		t.Fatalf("the mail is filed under %d projects, want 1: the cycle was kept as a fault, not rerun", n)
	}
	var faults int
	if err := owner.QueryRow(context.Background(), `SELECT count(*) FROM system_log
		WHERE action = 'capture_project_attribution_fault' AND detail->>'source_id' = 'cycle-1'`).Scan(&faults); err != nil {
		t.Fatalf("counting the recorded faults: %v", err)
	}
	if faults != 0 {
		t.Errorf("a lock cycle was recorded as %d fault(s), want none: the capture reruns instead", faults)
	}
}

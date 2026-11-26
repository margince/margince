// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// oneLastPage is a history provider whose first page is its last.
type oneLastPage struct {
	checkpointHistory
	failed int
}

func (p *oneLastPage) BackfillPage(context.Context, connector.Auth, time.Time, string, connector.Sink) (connector.BackfillPageResult, error) {
	return connector.BackfillPageResult{Scanned: 1 + p.failed, Failed: p.failed}, nil
}

// A finished history import settles the unread span it read. What it read goes,
// and what is older than it reached stays. A span that ended after the import
// was asked for is not one it covered. A run that failed a message did not read
// its window.
func TestAFinishedImportSettlesTheUnreadSpanItRead(t *testing.T) {
	day := 24 * time.Hour
	for _, c := range []struct {
		name string
		// from and to are offsets from the import's after_date, in days.
		from, to         int
		endsAfterRequest bool
		failed           int
		wantFrom, wantTo *int
	}{
		{name: "inside the window", from: 10, to: 20},
		{name: "older than the window", from: -10, to: 20, wantFrom: new(-10), wantTo: new(0)},
		{name: "wholly older than the window", from: -20, to: -10, wantFrom: new(-20), wantTo: new(-10)},
		{name: "ending after the import was asked for", from: 10, to: 20, endsAfterRequest: true, wantFrom: new(10), wantTo: new(20)},
		{name: "read by a run that failed a message", from: -10, to: 20, failed: 1, wantFrom: new(-10), wantTo: new(20)},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, reg, run := admissionRunWith(t, &oneLastPage{failed: c.failed})
			owner, _ := setupCaptureDB(t)
			var after time.Time
			var connection ids.UUID
			if err := owner.QueryRow(ctx, `SELECT after_date::timestamp AT TIME ZONE 'UTC', connection_id FROM capture_backfill WHERE id = $1`, run).
				Scan(&after, &connection); err != nil {
				t.Fatal(err)
			}
			at := func(days int) time.Time { return after.Add(time.Duration(days) * day) }
			if _, err := owner.Exec(ctx, `UPDATE capture_connection SET unread_from = $2, unread_to = $3 WHERE id = $1`,
				connection, at(c.from), at(c.to)); err != nil {
				t.Fatal(err)
			}
			if c.endsAfterRequest {
				if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET created_at = $2 WHERE id = $1`, run, at(c.to).Add(-time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
			if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
				t.Fatal(err)
			}
			if _, completed, _, err := reg.RunBackfillExecution(ctx, run, queue.generations[run], queue.scheduler()); err != nil || !completed {
				t.Fatalf("the import did not finish: completed=%v err=%v", completed, err)
			}

			var from, to *time.Time
			if err := owner.QueryRow(ctx, `SELECT unread_from, unread_to FROM capture_connection WHERE id = $1`, connection).
				Scan(&from, &to); err != nil {
				t.Fatal(err)
			}
			assertUnreadEnd(t, "start", from, c.wantFrom, at)
			assertUnreadEnd(t, "end", to, c.wantTo, at)
		})
	}
}

func assertUnreadEnd(t *testing.T, which string, got *time.Time, want *int, at func(int) time.Time) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("the span's %s is still %s, want the span cleared", which, *got)
	case want != nil && (got == nil || !got.Equal(at(*want))):
		t.Errorf("the span's %s is %v, want %s", which, got, at(*want))
	}
}

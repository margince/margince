// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"testing"
)

// The window is folded inside the flush's own transaction, on the database
// clock. A reader gets the samples and the clock they were taken on in one read.
func TestAFlushRecordsASampleOnTheDatabaseClock(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	progress := func(done int) Progress {
		return Progress{Objects: []ObjectProgress{{Object: "contact", RowsDone: done, RowsTotal: 100}}}
	}
	for _, done := range []int{10, 40} {
		if err := s.Flush(ctx, l, Flush{Checkpoint: done, Progress: progress(done), Phase: PhaseImporting, Object: "contact"}); err != nil {
			t.Fatalf("Flush: %v", err)
		}
	}
	run, err := s.GetStaged(ctx, l.Run)
	if err != nil {
		t.Fatalf("GetStaged: %v", err)
	}
	window := run.Progress.Window
	if len(window) != 2 || window[0].Done != 10 || window[1].Done != 40 {
		t.Fatalf("window = %+v, want the two flushes in order", window)
	}
	if run.ReadAt.IsZero() || run.StartedAt == nil || run.ProgressAt == nil || run.HeartbeatAt == nil {
		t.Fatalf("run clock = read %v started %v progress %v heartbeat %v, want every instant set",
			run.ReadAt, run.StartedAt, run.ProgressAt, run.HeartbeatAt)
	}
	if run.ProgressAt.After(run.ReadAt) || run.PhaseObject != "contact" {
		t.Fatalf("progress at %v after the read at %v, or object %q", run.ProgressAt, run.ReadAt, run.PhaseObject)
	}
}

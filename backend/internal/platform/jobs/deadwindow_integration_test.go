// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// The two dead counts, over a real river_job table.
//
// River retains a discarded or cancelled row for seven days, so the unbounded
// count is a week of history: an outage that ended an hour ago and one still
// running are the same number. That number is not wrong — the work did not
// happen — but it is a report, and presenting it as a live call to action is
// what made a finished outage keep a banner red until the rows retired.
//
// The observed case is the fixture: 531 rows from a DNS outage that had been
// over for an hour. The banner must be able to say that is history while still
// showing the total to anybody who wants it.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deadCounts answers one kind's pair, or zeros when the kind is absent.
func deadCounts(t *testing.T, health jobs.Health, kind string) (dead, recent int64) {
	t.Helper()
	for _, k := range health.Kinds {
		if k.Kind == kind {
			return k.Dead, k.DeadRecent
		}
	}
	t.Fatalf("no health row for kind %q — the counts below would both be zero for the wrong reason", kind)
	return 0, 0
}

func TestTheDeadBannerCountsTheWindowAndTheReportCountsTheWeek(t *testing.T) {
	ctx := context.Background()
	_, pool := migratedAppPool(t)
	ws := ids.NewV7()

	// The outage: 531 rows, all discarded, all finished an hour ago. The
	// number is the observed one rather than a round fixture, because what
	// made this a defect was the SIZE of a settled history shouting.
	settled := time.Now().Add(-time.Hour)
	for range 531 {
		seedJob(ctx, t, pool, seed{
			Kind: "capture_sync", State: "discarded", Workspace: ws, CreatedAt: settled,
		})
	}
	// And one that died three days ago, well outside any sensible window —
	// the week's history the report figure is for.
	seedJob(ctx, t, pool, seed{
		Kind: "capture_sync", State: "discarded", Workspace: ws,
		CreatedAt: time.Now().AddDate(0, 0, -3),
	})

	// A window that ends between the two.
	health, err := jobs.WorkspaceHealth(ctx, pool, ws.String(), nil, 24*time.Hour)
	if err != nil {
		t.Fatalf("reading job health: %v", err)
	}
	dead, recent := deadCounts(t, health, "capture_sync")
	if dead != 532 {
		t.Errorf("the report figure counts %d, want 532 — the full history is what it is for, and "+
			"narrowing it would destroy the record an investigator came for", dead)
	}
	if recent != 531 {
		t.Errorf("the banner counts %d, want the 531 inside the window", recent)
	}

	// An hour ago is now outside the window, and the banner goes quiet while
	// the report figure does not move. This is the case the whole change is
	// about: the same rows, read as history.
	health, err = jobs.WorkspaceHealth(ctx, pool, ws.String(), nil, time.Minute)
	if err != nil {
		t.Fatalf("reading job health on a one-minute window: %v", err)
	}
	dead, recent = deadCounts(t, health, "capture_sync")
	if recent != 0 {
		t.Errorf("the banner still counts %d after the outage left the window — a settled history "+
			"goes on asking for a hand, which is the defect", recent)
	}
	if dead != 532 {
		t.Errorf("the report figure moved to %d with the window — the window bounds the alarm, "+
			"never the record", dead)
	}
}

// The count above compares finalized_at directly, with no fallback. This is what
// makes that safe: river_job refuses a terminal row that does not carry the
// moment it died, so there is no undated row for the comparison to drop.
//
// Asserted rather than assumed, because the failure would be silent in the
// worst direction — an undated terminal row would fall out of every window and
// the banner would go quiet about the one row nobody can date.
func TestATerminalRowMustCarryTheMomentItDied(t *testing.T) {
	ctx := context.Background()
	_, pool := migratedAppPool(t)
	ws := ids.NewV7()

	seedJob(ctx, t, pool, seed{Kind: "capture_sync", State: "discarded", Workspace: ws})
	_, err := pool.Exec(ctx, `UPDATE river_job SET finalized_at = NULL WHERE kind = 'capture_sync'`)
	if err == nil {
		t.Fatal("river_job accepted a discarded row with no finalized_at — the banner's count " +
			"compares that column directly, so such a row would silently leave every window")
	}
	if !strings.Contains(err.Error(), "finalized_or_finalized_at_null") {
		t.Errorf("the refusal came from %v, not from finalized_or_finalized_at_null — the count "+
			"relies on that constraint by name", err)
	}
}

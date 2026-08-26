// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The pipeline-risk candidate set at two idle windows, against a real database.
//
// The window is SQL (deals.QuietSQL) and the admission decision is Go, and the
// two can disagree silently: the deal row carries a `stalled` flag computed at
// the product-wide 60-day threshold, so a filter that consults that flag drops
// every row a shorter window was written to find. The lane then fetches exactly
// the right deals and shows none of them, which reads on screen as a quiet
// pipeline rather than as a bug.

import (
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// idleFor backdates a deal's activity clock so the idle window has something to
// measure. The columns are the ones deals.QuietSQL reads; the create path stamps
// them at now, and no writer offers a past value.
func idleFor(t *testing.T, e *integration.Env, deal ids.UUID, days int) {
	t.Helper()
	e.WsExec(t, `UPDATE deal SET created_at = now() - make_interval(days => $2),
		last_activity_at = now() - make_interval(days => $2) WHERE id = $1`, deal, days)
}

func namesOf(rows []agents.SlippingDeal) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	return out
}

// A deal quiet three weeks reaches the short window and not the stalled one.
// This is the whole point of the second threshold, and it is the case the
// stalled flag cannot express.
func TestAShortWindowFindsADealTheStalledThresholdCannotSeeYet(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	quiet := e.SeedDeal(t, "Quiet three weeks", pipeline, stage, &e.AdminUser)
	fresh := e.SeedDeal(t, "Touched yesterday", pipeline, stage, &e.AdminUser)
	idleFor(t, e, quiet, 21)
	idleFor(t, e, fresh, 1)

	short := quietDealLister(e.Pool, deals.QuietThresholdDays)
	got, err := short(e.Admin())
	if err != nil {
		t.Fatalf("reading the short window: %v", err)
	}
	if names := namesOf(got); len(names) != 1 || names[0] != "Quiet three weeks" {
		t.Fatalf("the short window = %v, want only the deal quiet three weeks", names)
	}

	// The same deal, asked with the product-wide patience, is not yet slipping.
	stalledWindow := quietDealLister(e.Pool, deals.StalledThresholdDays)
	got, err = stalledWindow(e.Admin())
	if err != nil {
		t.Fatalf("reading the stalled window: %v", err)
	}
	if names := namesOf(got); len(names) != 0 {
		t.Fatalf("the stalled window = %v, want nothing at 21 days idle", names)
	}
}

// A deal past the stalled threshold reaches BOTH windows. Without this the test
// above passes just as well against a lister that returns nothing at all.
func TestALongIdleDealReachesBothWindows(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	old := e.SeedDeal(t, "Quiet since spring", pipeline, stage, &e.AdminUser)
	idleFor(t, e, old, 90)

	for _, window := range []int{deals.QuietThresholdDays, deals.StalledThresholdDays} {
		got, err := quietDealLister(e.Pool, window)(e.Admin())
		if err != nil {
			t.Fatalf("reading the %d-day window: %v", window, err)
		}
		if names := namesOf(got); len(names) != 1 || names[0] != "Quiet since spring" {
			t.Errorf("the %d-day window = %v, want the deal quiet since spring", window, names)
		}
	}
}

// An overdue close date admits a deal at ANY window, including one it is far too
// fresh for on the idle clock alone. That arm is independent of the threshold
// and must not be lost when the window narrows.
func TestAnOverdueCloseDateAdmitsADealThatIsNotQuietAtAll(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	late := e.SeedDeal(t, "Closing last month", pipeline, stage, &e.AdminUser)
	idleFor(t, e, late, 1)
	e.WsExec(t, `UPDATE deal SET expected_close_date = (now() - interval '30 days')::date
		WHERE id = $1`, late)

	got, err := quietDealLister(e.Pool, deals.QuietThresholdDays)(e.Admin())
	if err != nil {
		t.Fatalf("reading the short window: %v", err)
	}
	if names := namesOf(got); len(names) != 1 || names[0] != "Closing last month" {
		t.Fatalf("the window = %v, want the deal whose close date has passed", names)
	}
	if !got[0].CloseOverdue {
		t.Error("the deal is admitted but not flagged close-overdue, so the card cannot say why")
	}
}

// A won deal never slips, at any window. The suppression lives in the shared
// rule rather than in each caller, and narrowing the window must not lose it.
func TestAClosedDealNeverSlipsAtEitherWindow(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	won := e.SeedDeal(t, "Won in spring", pipeline, stage, &e.AdminUser)
	idleFor(t, e, won, 200)
	// closed_at travels with the status: deal_closed_at refuses a won deal that
	// does not say when it closed, which is the constraint doing its job.
	e.WsExec(t, `UPDATE deal SET status = 'won', closed_at = now() WHERE id = $1`, won)

	for _, window := range []int{deals.QuietThresholdDays, deals.StalledThresholdDays} {
		got, err := quietDealLister(e.Pool, window)(e.Admin())
		if err != nil {
			t.Fatalf("reading the %d-day window: %v", window, err)
		}
		if names := namesOf(got); len(names) != 0 {
			t.Errorf("the %d-day window = %v, want a won deal to slip at neither", window, names)
		}
	}
}

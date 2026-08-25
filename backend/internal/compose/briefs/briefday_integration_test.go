// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// A rep has one brief per local day, and the on-open read serves THAT day's.
//
// Both halves matter and neither implies the other. Without the uniqueness the
// overnight dispatcher racing its own boot pass writes two runs for one
// morning, and a rep opening Home is told two different things about the same
// day. Without the day filter the read hands back whatever ran last, so a rep
// back from holiday reads last Monday's ranking under this morning's date —
// which is worse than an absent brief, because absence is visible and
// staleness is not.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestBriefReadServesTodaysRunAndNotAnOlderOne(t *testing.T) {
	b := setupBrief(t)

	yesterday, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}

	// The same rep opens Home two days later, having pressed nothing. The
	// overnight pass has not run for her, so there is no run for today.
	twoDaysOn := briefClock.AddDate(0, 0, 2)
	if _, err := b.engine.LatestRun(b.repCtx, twoDaysOn); err == nil {
		t.Fatal("the read served a run for a day that has none — a stale ranking under today's date is the defect this filter exists to prevent")
	} else if !apperrorsIsNotFound(err) {
		t.Fatalf("reading a day with no run = %v, want the existence-hiding not-found", err)
	}

	// And the older run is still there, still readable on its own day: the
	// filter narrows the read, it does not delete history.
	stillThere, err := b.engine.LatestRun(b.repCtx, briefClock.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stillThere.ID != yesterday.ID {
		t.Fatalf("the read on the run's own day gave %s, want the run written that day %s", stillThere.ID, yesterday.ID)
	}
	if stillThere.LocalDay.Format(time.DateOnly) != briefClock.Format(time.DateOnly) {
		t.Fatalf("run local_day = %s, want the day it was assembled for %s",
			stillThere.LocalDay.Format(time.DateOnly), briefClock.Format(time.DateOnly))
	}
}

func TestTwoBriefAssembliesOnOneMorningProduceOneRun(t *testing.T) {
	b := setupBrief(t)

	// The boot pass and the hourly pass, reaching the same rep at the same
	// moment. Both must SUCCEED — the loser reads the winner rather than
	// failing, because a duplicate is the constraint doing its job and not an
	// error the job should be retried for.
	var (
		wg      sync.WaitGroup
		runs    [2]BriefRun
		created [2]bool
		errs    [2]error
	)
	for i := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runs[i], created[i], errs[i] = b.engine.SnapshotRunForDay(b.repCtx, briefClock)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("assembly %d failed: %v — the racing loser must read the winner, not error", i, err)
		}
	}
	if runs[0].ID != runs[1].ID {
		t.Fatalf("the two passes produced runs %s and %s, want one run for one morning", runs[0].ID, runs[1].ID)
	}
	if created[0] == created[1] {
		t.Fatalf("both passes reported created=%v, want exactly one of them to have assembled the run", created[0])
	}

	var stored int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM brief_run WHERE user_id = $1 AND local_day = $2`,
		b.Rep1, briefClock.Format(time.DateOnly)).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("brief_run rows for the morning = %d, want 1", stored)
	}
}

func TestAnotherRepsRunIsNeverServedAsMine(t *testing.T) {
	b := setupBrief(t)

	// Rep1's morning exists; Rep2's does not. Rep2 shares Rep1's team, so
	// nothing about the DEALS hides Rep1's run from her — the personal scope is
	// what must, and it must answer not-found rather than forbidden so the run's
	// existence stays hidden.
	if _, err := b.engine.SnapshotRun(b.repCtx, briefClock); err != nil {
		t.Fatal(err)
	}
	other := b.As(b.Rep2, []ids.UUID{b.Team1}, integration.RepPerms)
	if _, err := b.engine.LatestRun(other, briefClock); !apperrorsIsNotFound(err) {
		t.Fatalf("another rep's read of a run they do not own = %v, want not-found", err)
	}
}

// apperrorsIsNotFound keeps the assertions above reading as one sentence
// rather than repeating the wrapping check at each of them.
func apperrorsIsNotFound(err error) bool {
	return errors.Is(err, apperrors.ErrNotFound)
}

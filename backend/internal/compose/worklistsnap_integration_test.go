// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The frozen walk, against the real table.
//
// Every property worth proving here is a property of the ROW — who may resume a
// walk, what expiry does to one, what the sweep leaves behind — and a fake
// would agree with whatever the store believes. The refusals especially: a
// stolen token and an expired one must be indistinguishable, and that is a
// statement about what the SELECT returns rather than about a branch in Go.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestAWalkResumesForTheReaderWhoStartedIt is the ordinary case, and the one
// every refusal below is measured against.
func TestAWalkResumesForTheReaderWhoStartedIt(t *testing.T) {
	e := integration.Setup(t)
	clock := &testClock{at: time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)}
	snaps := worklistsnap.New(e.Pool, clock.Now)
	ctx := repCtx(e, e.Rep1)

	id, err := snaps.Freeze(ctx, "question-one", clock.at,
		worklistsnap.Buckets{Urgent: 2, Review: 1},
		[]worklistsnap.Row{{Source: "customer_waiting", RowID: "a"}, {Source: "task", RowID: "b"}})
	if err != nil {
		t.Fatalf("freezing a walk: %v", err)
	}

	walk, err := snaps.Resume(ctx, id, "question-one")
	if err != nil {
		t.Fatalf("resuming the walk that was just frozen: %v", err)
	}
	if len(walk.Rows) != 2 || walk.Rows[0].RowID != "a" || walk.Rows[1].RowID != "b" {
		t.Errorf("the walk came back as %v, want the order it was frozen in", walk.Rows)
	}
	if walk.Buckets.Urgent != 2 || walk.Buckets.Review != 1 {
		t.Errorf("the frozen figures came back as %+v, want what was written", walk.Buckets)
	}
}

// TestAColleaguesWalkIsNotResumable is the one that matters most.
//
// A snapshot id is an opaque uuid a client holds, and nothing stops one being
// pasted into another person's request. The row is keyed on its reader, and
// this proves the SELECT is too — a walk is one person's position and reading
// somebody else's would hand over the order their day was ranked in.
func TestAColleaguesWalkIsNotResumable(t *testing.T) {
	e := integration.Setup(t)
	clock := &testClock{at: time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)}
	snaps := worklistsnap.New(e.Pool, clock.Now)

	id, err := snaps.Freeze(repCtx(e, e.Rep1), "question-one", clock.at,
		worklistsnap.Buckets{}, []worklistsnap.Row{{Source: "task", RowID: "a"}})
	if err != nil {
		t.Fatalf("freezing rep one's walk: %v", err)
	}

	_, err = snaps.Resume(repCtx(e, e.Rep2), id, "question-one")

	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a colleague resuming another reader's walk got %v, want not-found", err)
	}
}

// TestAnExpiredWalkIsRefusedTheSameWayAStolenOneIs.
//
// Both answer not-found, deliberately: a client that could tell them apart
// would learn whether a walk it does not own exists. The remedy is identical
// either way — start a fresh snapshot — so there is nothing a distinct answer
// would buy the honest caller.
func TestAnExpiredWalkIsRefusedTheSameWayAStolenOneIs(t *testing.T) {
	e := integration.Setup(t)
	clock := &testClock{at: time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)}
	snaps := worklistsnap.New(e.Pool, clock.Now)
	ctx := repCtx(e, e.Rep1)

	id, err := snaps.Freeze(ctx, "question-one", clock.at,
		worklistsnap.Buckets{}, []worklistsnap.Row{{Source: "task", RowID: "a"}})
	if err != nil {
		t.Fatalf("freezing a walk: %v", err)
	}
	// Resumable right up to the boundary, so the refusal below is about the
	// expiry rather than about the walk never having worked.
	if _, err := snaps.Resume(ctx, id, "question-one"); err != nil {
		t.Fatalf("the walk was not resumable before it expired: %v", err)
	}

	clock.at = clock.at.Add(worklistsnap.Life + time.Minute)

	if _, err := snaps.Resume(ctx, id, "question-one"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an expired walk answered %v, want the same not-found a stolen id gets", err)
	}
}

// TestAWalkIsNotResumableUnderADifferentQuestion.
//
// The cursor carries its own fingerprint and the caller compares it, but a
// snapshot id could be lifted onto a token minted for a different scope. The
// walk says which question it answers, so it refuses rather than resuming a
// reader into an answer they did not ask for — page two of "my tasks"
// continuing into "the team's deals".
func TestAWalkIsNotResumableUnderADifferentQuestion(t *testing.T) {
	e := integration.Setup(t)
	clock := &testClock{at: time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)}
	snaps := worklistsnap.New(e.Pool, clock.Now)
	ctx := repCtx(e, e.Rep1)

	id, err := snaps.Freeze(ctx, "my-tasks", clock.at,
		worklistsnap.Buckets{}, []worklistsnap.Row{{Source: "task", RowID: "a"}})
	if err != nil {
		t.Fatalf("freezing a walk: %v", err)
	}

	if _, err := snaps.Resume(ctx, id, "the-teams-deals"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a walk resumed under a different question answered %v, want not-found", err)
	}
}

// TestARepsOldestWalksAreSweptAway holds the cost ceiling.
//
// Nothing but this removes a snapshot, so a rep who refreshes all morning would
// otherwise leave a row behind for every refresh. The NEWEST survive, which is
// the honest cut: the walk somebody started most recently is the one they are
// still on.
func TestARepsOldestWalksAreSweptAway(t *testing.T) {
	e := integration.Setup(t)
	clock := &testClock{at: time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)}
	snaps := worklistsnap.New(e.Pool, clock.Now)
	ctx := repCtx(e, e.Rep1)

	minted := make([]ids.UUID, 0, 5)
	for i := 0; i < 5; i++ {
		clock.at = clock.at.Add(time.Minute)
		id, err := snaps.Freeze(ctx, "question-one", clock.at,
			worklistsnap.Buckets{}, []worklistsnap.Row{{Source: "task", RowID: "a"}})
		if err != nil {
			t.Fatalf("freezing walk %d: %v", i, err)
		}
		minted = append(minted, id)
	}

	// The newest is resumable and the oldest is gone. Asserting both, because a
	// sweep that deleted everything would satisfy the second alone.
	if _, err := snaps.Resume(ctx, minted[len(minted)-1], "question-one"); err != nil {
		t.Errorf("the newest walk was swept away with the old ones: %v", err)
	}
	if _, err := snaps.Resume(ctx, minted[0], "question-one"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a reader's oldest walk survived five refreshes, so nothing bounds what "+
			"their snapshots cost: %v", err)
	}
}

// TestAnAgentHoldsNoWalk.
//
// A snapshot binds one reader. A principal with no human behind it has no walk
// to hold, and writing one against a zero id would be a row every agent shared
// and any of them could resume into.
func TestAnAgentHoldsNoWalk(t *testing.T) {
	e := integration.Setup(t)
	clock := &testClock{at: time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)}
	snaps := worklistsnap.New(e.Pool, clock.Now)

	_, err := snaps.Freeze(e.AgentCtx(), "question-one", clock.at,
		worklistsnap.Buckets{}, []worklistsnap.Row{{Source: "task", RowID: "a"}})

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent froze a walk and got %v, want permission denied", err)
	}
}

// repCtx binds a human rep, which is what a walk is for.
func repCtx(e *integration.Env, rep ids.UUID) context.Context {
	return e.As(rep, []ids.UUID{e.Team1}, integration.AdminPerms)
}

// testClock is the injected clock every expiry assertion moves.
type testClock struct{ at time.Time }

func (c *testClock) Now() time.Time { return c.at }

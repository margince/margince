// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Every contact merge takes the stops, then the contact rows, then the pair
// rows. Two merges of one pair through different doors then queue behind each
// other: one commits and the other is refused, and neither deadlocks.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// raceBehindABarrier holds both contacts' rows, starts first and waits until
// it is parked, then starts second, and releases the rows once both wait.
func (c *caseEnv) raceBehindABarrier(t *testing.T, members []ids.UUID, first, second func() error) [2]error {
	t.Helper()
	owner := integration.OwnerConn(t)
	barrier, err := owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := barrier.Exec(context.Background(), `SELECT id FROM contact WHERE id = ANY($1) FOR UPDATE`, members); err != nil {
		t.Fatal(err)
	}
	var results [2]chan error
	for i, merge := range []func() error{first, second} {
		results[i] = make(chan error, 1)
		go func() { results[i] <- merge() }()
		waitForWaiters(t, owner, i+1)
	}
	if err := barrier.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return [2]error{<-results[0], <-results[1]}
}

// oneMergeWon fails unless one merge committed and the other was refused for
// the merge already made.
func oneMergeWon(t *testing.T, errs [2]error) {
	t.Helper()
	committed := 0
	for _, err := range errs {
		switch code := caseCode(err); {
		case err == nil:
			committed++
		case code != "case_changed" && code != "already_disposed":
			t.Errorf("a racing merge failed for another reason: %v", err)
		}
	}
	if committed != 1 {
		t.Errorf("%d racing merges committed (%v), want exactly one", committed, errs)
	}
}

func TestACaseMergeAndAPairMergeOfOnePairQueue(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep1)
	read := c.read(c.e.Admin(), t, members[0])
	pair, winner := ids.UUID(read.Edges[0].PairId), members[0]
	oneMergeWon(t, c.raceBehindABarrier(t, members,
		func() error {
			_, err := c.store.MergeContactCase(c.e.Admin(), members[0], caseDecision(read, members, members[0], c.e.Rep1))
			return err
		},
		func() error {
			_, err := c.store.DisposeDedupeCandidate(c.e.Admin(), pair, "merge", &winner)
			return err
		}))
}

func TestADirectMergeAndAPairMergeOfOnePairQueue(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep1)
	pair, winner := ids.UUID(c.read(c.e.Admin(), t, members[0]).Edges[0].PairId), members[0]
	oneMergeWon(t, c.raceBehindABarrier(t, members,
		func() error {
			_, err := c.store.MergeContact(c.e.Admin(), ids.From[ids.ContactKind](members[1]), ids.From[ids.ContactKind](members[0]), nil)
			return err
		},
		func() error {
			_, err := c.store.DisposeDedupeCandidate(c.e.Admin(), pair, "merge", &winner)
			return err
		}))
}

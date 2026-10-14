// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The reviewer's queue is bounded, so its tail needs a way to be asked for.
// Oldest first and cut at a limit, a backlog past the ceiling left the reviews
// that had waited longest unreachable by any caller.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedWaitingReview writes one review awaiting a decision, opened at a stated
// instant so the queue's order is the fixture's rather than the clock's.
//
// The approval id is what the state means, not fixture decoration.
// communication_review_routing_shape refuses `awaiting_decision` without one:
// a review waits by having been routed to somebody.
//
// The column carries no foreign key, so the id stands for that routing with no
// approval row behind it.
func seedWaitingReview(ctx context.Context, t *testing.T, e *channelConsentEnv, openedMinutes int) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO communication_review (state, kind, reason_code, refusals, opened_at, approval_id)
		VALUES ('awaiting_decision', 'single', 'marketing_objection',
		        '[{"address":"anna@example.test","reason_code":"marketing_objection"}]'::jsonb,
		        now() - make_interval(mins => $1), $2)
		RETURNING id`, openedMinutes, ids.NewV7()).Scan(&id); err != nil {
		t.Fatalf("seeding a waiting review: %v", err)
	}
	return id
}

func TestTheReviewQueuePagesWithoutSkippingOrRepeating(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := e.ctx
	// Five, oldest first: 50 minutes ago down to 10.
	seeded := map[ids.UUID]bool{}
	for _, age := range []int{50, 40, 30, 20, 10} {
		seeded[seedWaitingReview(ctx, t, e, age)] = true
	}

	reader := exceptionCtx(e.ws, e.user, principal.ObjectGrant{Read: true})
	seen := map[ids.UUID]int{}
	cursor := ""
	for page := 1; page <= 4; page++ {
		rows, total, next, err := e.store.AwaitingDecision(reader, 2, cursor)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		// The count is the whole queue on every page. A window count taken
		// after the keyset filter would fall as the caller paged.
		if total != len(seeded) {
			t.Errorf("page %d reports total %d, want %d on every page", page, total, len(seeded))
		}
		for _, r := range rows {
			seen[r.ID]++
		}
		if next == "" {
			if page != 3 {
				t.Errorf("the queue ended on page %d, want three pages of 2, 2 and 1", page)
			}
			break
		}
		cursor = next
	}

	if len(seen) != len(seeded) {
		t.Errorf("paging reached %d of %d reviews, so the tail is still unreachable", len(seen), len(seeded))
	}
	for id, times := range seen {
		if times != 1 {
			t.Errorf("review %s was answered %d times across the pages, want once", id, times)
		}
		if !seeded[id] {
			t.Errorf("review %s was answered and this fixture did not seed it", id)
		}
	}
}

// A token the caller did not get from this queue is refused rather than read as
// the start of it. Serving page one again is how a client loops on a token it
// keeps re-sending.
func TestTheReviewQueueRefusesATokenItNeverMinted(t *testing.T) {
	e := setupChannelConsent(t)
	seedWaitingReview(e.ctx, t, e, 10)
	reader := exceptionCtx(e.ws, e.user, principal.ObjectGrant{Read: true})

	if _, _, _, err := e.store.AwaitingDecision(reader, 2, "not-a-cursor"); err == nil {
		t.Fatal("an undecodable cursor was accepted, so a client re-sending it reads page one forever")
	}
}

// The last page carries no token. One offered there is a page a caller can ask
// for and never receive.
func TestTheLastReviewPageOffersNoCursor(t *testing.T) {
	e := setupChannelConsent(t)
	seedWaitingReview(e.ctx, t, e, 10)
	reader := exceptionCtx(e.ws, e.user, principal.ObjectGrant{Read: true})

	rows, _, next, err := e.store.AwaitingDecision(reader, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("read %d reviews, want the one seeded", len(rows))
	}
	if next != "" {
		t.Errorf("a short page offered the cursor %q", next)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A held message carries the route back to the review that would unstop it.
//
// A refusal answers 422 with the review it opened, and that answer reaches the
// rep exactly once: at the moment they pressed send. Navigate away, and the
// scheduled list shows a held row that explains why the message stopped with no
// way to reach the work that would let it go.
//
// So the row carries the id. It is a POINTER and never permission — reading the
// review is gated on its own terms, and a caller handed an id for somebody
// else's review is still refused when they open it.

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// scheduledRow is the shape these assertions read off the wire.
type scheduledRow struct {
	ID         string  `json:"id"`
	Status     string  `json:"status"`
	HeldReason *string `json:"held_reason"`
	ReviewID   *string `json:"review_id"`
}

// scheduledList reads the caller's own scheduled messages.
func scheduledList(t *testing.T, c *consentEnv) []scheduledRow {
	t.Helper()
	var out []scheduledRow
	if status := c.Call(t, "GET", "/v1/scheduled-sends", nil, nil, &out); status != http.StatusOK {
		t.Fatalf("listing scheduled messages → %d", status)
	}
	return out
}

// A HELD MESSAGE NAMES ITS REVIEW.
func TestAHeldMessageCarriesTheRouteToItsReview(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	var review string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM communication_review WHERE resolved_at IS NULL`).Scan(&review); err != nil {
		t.Fatalf("reading the review: %v", err)
	}

	rows := scheduledList(t, c)
	if len(rows) != 1 {
		t.Fatalf("%d scheduled row(s), want the one held message", len(rows))
	}
	if rows[0].Status != "held" {
		t.Fatalf("the row reads %q, want held", rows[0].Status)
	}
	if rows[0].ReviewID == nil {
		t.Fatal("the held row names no review, so a rep looking at their scheduled list can see " +
			"that the message stopped and has no way to reach the work that would unstop it")
	}
	if *rows[0].ReviewID != review {
		t.Errorf("the row names review %q, want %q", *rows[0].ReviewID, review)
	}

	// THE DETAIL READ AGREES WITH THE LIST. A row that named its review in one
	// and not the other would leave a screen working from whichever it happened
	// to fetch.
	var detail scheduledRow
	if status := c.Call(t, "GET", "/v1/scheduled-sends/"+rows[0].ID, nil, nil, &detail); status != http.StatusOK {
		t.Fatalf("reading the held message → %d", status)
	}
	if detail.ReviewID == nil || *detail.ReviewID != review {
		t.Errorf("the detail read names %v, want %q", detail.ReviewID, review)
	}
}

// A MESSAGE NOBODY REFUSED NAMES NO REVIEW, EVEN BESIDE ONE THAT DOES.
//
// The two messages sit in one list: a refused one holding a live review, and an
// ordinary one holding nothing. What this pins is that the two rows get
// DIFFERENT answers — a lookup keyed on anything coarser than the message
// itself would put the first one's review on the second, sending a rep to work
// about a message they did not click.
//
// It does NOT pin the held-status filter in stampReviews; the test below that
// does is TestARescheduledMessageNamesNoReview.
func TestAnOrdinaryScheduledMessageNamesNoReview(t *testing.T) {
	c := setupConsent(t)

	// First the refusal, so a live review exists to be mis-stamped.
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}

	// Then an ordinary message the engine allows, scheduled rather than sent.
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to":              []string{"subject@consent.test"},
		"consent_purpose": "business_correspondence",
		"scheduled_at":    time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz":    "UTC",
	}, nil, nil); status >= 400 {
		t.Fatalf("scheduling an ordinary message → %d", status)
	}

	rows := scheduledList(t, c)
	if len(rows) != 2 {
		t.Fatalf("%d scheduled row(s), want the held one and the ordinary one", len(rows))
	}
	var held, ordinary *scheduledRow
	for i := range rows {
		if rows[i].Status == "held" {
			held = &rows[i]
		} else {
			ordinary = &rows[i]
		}
	}
	if held == nil || ordinary == nil {
		t.Fatalf("want one held row and one ordinary row, got %+v", rows)
	}
	// The held one carries its review, so this test is really about two
	// different answers rather than about an installation that stamps nothing.
	if held.ReviewID == nil {
		t.Fatal("the held row names no review, so there is nothing that could be mis-stamped " +
			"and this test proves nothing")
	}
	if ordinary.ReviewID != nil {
		t.Errorf("the ordinary message names review %q — following it would send the rep to work "+
			"about the OTHER message", *ordinary.ReviewID)
	}
}

// AN ANSWERED REVIEW STOPS BEING NAMED WHILE THE MESSAGE IS STILL HELD.
//
// The lookup reads live reviews only, and this is what proves that rather than
// proving the status filter beside it. A rep DECLINES the routed decision: the
// review returns to them, is then cancelled, and the message stays held the
// whole time — so a row still naming the closed review would be offering a
// route to work somebody has already finished.
func TestAMessageWhoseReviewIsOverNamesItNoMore(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	before := scheduledList(t, c)
	if len(before) != 1 || before[0].ReviewID == nil {
		t.Fatalf("no held row naming a review to begin with: %+v", before)
	}

	// The review is closed WITHOUT the message moving: written directly,
	// because every door that closes a review also settles the message, and
	// what this test needs is the two apart.
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE communication_review SET state = 'cancelled', resolved_at = now()
		  WHERE id = $1`, *before[0].ReviewID); err != nil {
		t.Fatalf("closing the review: %v", err)
	}

	after := scheduledList(t, c)
	if len(after) != 1 {
		t.Fatalf("%d scheduled row(s) after, want the same one message", len(after))
	}
	if after[0].Status != "held" {
		t.Fatalf("the message reads %q, want held — this test is about a held row whose review "+
			"is over, and a moved message would pass it for the wrong reason", after[0].Status)
	}
	if after[0].ReviewID != nil {
		t.Errorf("the held message still names review %q after it was closed — a rep following "+
			"it reaches work somebody has already finished", *after[0].ReviewID)
	}
}

// A RESCHEDULED MESSAGE NAMES NO REVIEW, THOUGH ITS REVIEW IS STILL LIVE.
//
// This is the case that makes the held-status filter in stampReviews decide the
// answer rather than merely narrow the read, and it took a review round to
// find: rescheduling a held message moves it back to 'scheduled' and
// deliberately LEAVES ITS REVIEW LIVE, because the message is still going out
// and its decision is still outstanding (RescheduleInTx).
//
// So the lookup would happily answer for that row. Without the filter, a
// message the rep has already dealt with would carry a route to a refusal taken
// at a moment that has passed — and the rep would be sent to work about a
// message they have already moved.
func TestARescheduledMessageNamesNoReview(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	held := scheduledList(t, c)
	if len(held) != 1 || held[0].ReviewID == nil {
		t.Fatalf("no held row naming a review to begin with: %+v", held)
	}
	review := *held[0].ReviewID

	var current struct {
		Version int64 `json:"version"`
	}
	if status := c.Call(t, "GET", "/v1/scheduled-sends/"+held[0].ID, nil, nil, &current); status != http.StatusOK {
		t.Fatalf("reading the held message → %d", status)
	}
	if status := c.Call(t, "PATCH", "/v1/scheduled-sends/"+held[0].ID, AnyMap{
		"scheduled_at": time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "UTC",
	}, map[string]string{"If-Match": strconv.FormatInt(current.Version, 10)}, nil); status >= 400 {
		t.Fatalf("moving the held message → %d", status)
	}

	// The review really is still live, which is what makes this test about the
	// filter rather than about a review that closed itself.
	var resolved *string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT resolved_at::text FROM communication_review WHERE id = $1`, review).Scan(&resolved); err != nil {
		t.Fatalf("reading the review: %v", err)
	}
	if resolved != nil {
		t.Fatalf("the review closed when the message moved, so the filter is not what this test " +
			"is exercising")
	}

	after := scheduledList(t, c)
	if len(after) != 1 {
		t.Fatalf("%d scheduled row(s), want the one moved message", len(after))
	}
	if after[0].Status == "held" {
		t.Fatalf("the message still reads held, so it did not move")
	}
	if after[0].ReviewID != nil {
		t.Errorf("the moved message names review %q — the rep already dealt with this one, and "+
			"following it reaches a refusal taken at a moment that has passed", *after[0].ReviewID)
	}
}

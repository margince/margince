// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
)

// Asking a colleague to decide a refused send is not offered, and neither is
// the deciders' queue: both routes are gone. Calling the old ask raises no
// approval card and leaves the review with the rep.
func TestTheRetiredAskAndQueueAreGone(t *testing.T) {
	c := setupConsent(t)
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	before := reviewState(t, c, review)

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("the retired ask → %d, want the route gone", status)
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews", nil, nil, nil); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("the retired deciders' queue → %d, want the route gone", status)
	}
	var cards int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE kind = 'communication_review'`).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if cards != 0 {
		t.Fatalf("%d approval card(s) raised by a route that no longer exists", cards)
	}
	if after := reviewState(t, c, review); after != before {
		t.Fatalf("calling the retired ask moved the review from %s to %s", before, after)
	}
}

// A card raised under the retired communication_review kind stays readable and
// cannot be approved: nothing would carry out a yes.
func TestARetiredReviewCardIsReadableAndCannotBeApproved(t *testing.T) {
	c := setupConsent(t)
	seat := sessionUserID(t, c.AppEnv)
	ctx := context.Background()
	var pending, decided string
	for status, into := range map[string]*string{"pending": &pending, "approved": &decided} {
		if err := c.Owner.QueryRow(ctx, `
			INSERT INTO approval (kind, status, proposed_by, on_behalf_of, summary, proposed_change,
			                      diff_hash, expires_at, decided_at, requested_decider_id, requested_decider_source)
			VALUES ('communication_review', $1, 'human:'||$2::text, $2::uuid, 'Send it anyway?', '{}', $3,
			        now() + interval '1 day', CASE WHEN $1 = 'pending' THEN NULL ELSE now() END,
			        $2::uuid, 'on_behalf_of')
			RETURNING id::text`, status, seat, "retired-review-"+status).Scan(into); err != nil {
			t.Fatalf("seeding a %s retired card: %v", status, err)
		}
	}
	if status := c.Call(t, "GET", "/v1/approvals/"+decided, nil, nil, nil); status != http.StatusOK {
		t.Errorf("opening a decided retired card → %d, want 200", status)
	}
	if status := c.Call(t, "POST", "/v1/approvals/"+pending+"/approve", AnyMap{}, nil, nil); status == http.StatusOK {
		t.Fatal("approving a retired review card was accepted")
	}
	var after string
	if err := c.Owner.QueryRow(ctx, `SELECT status FROM approval WHERE id = $1`, pending).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != "pending" {
		t.Errorf("the refused approval left the card %s, want pending", after)
	}
}

func reviewState(t *testing.T, c *consentEnv, review string) string {
	t.Helper()
	var state string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT state FROM communication_review WHERE id = $1`, review).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
)

// ASKING A COLLEAGUE TO DECIDE A REFUSED SEND IS NOT OFFERED.
//
// A rep overrules a machine refusal with their own reason instead. The retired
// door refuses, raises no approval card, and leaves the review with the rep.
func TestAskingAColleagueToDecideARefusedSendIsRefused(t *testing.T) {
	c := setupConsent(t)
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	review := liveReviewID(t, c)
	before := reviewState(t, c, review)

	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/request-decision",
		AnyMap{}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("asking a colleague → %d, want 422", status)
	}
	var cards int
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE kind = 'communication_review'`).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if cards != 0 {
		t.Fatalf("%d approval card(s) raised by a door that no longer asks anybody", cards)
	}
	if after := reviewState(t, c, review); after != before {
		t.Fatalf("the refused ask moved the review from %s to %s", before, after)
	}
}

// The deprecated deciders' queue still answers its callers, with an empty page,
// though a review is waiting for it.
func TestTheDecidersQueueAnswersAnEmptyPage(t *testing.T) {
	c := setupConsent(t)
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing send with no grant → %d, want 409", status)
	}
	liveReviewID(t, c)
	var page struct {
		Data  []AnyMap `json:"data"`
		Total *int     `json:"total"`
		Page  struct {
			HasMore bool `json:"has_more"`
		} `json:"page"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews?limit=5", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("reading the deciders' queue → %d, want 200", status)
	}
	if page.Data == nil || len(page.Data) != 0 || page.Total == nil || *page.Total != 0 || page.Page.HasMore {
		t.Fatalf("the queue answered %+v, want an empty page with a total of 0", page)
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

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
	var state string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT state FROM communication_review WHERE id = $1`, review).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state == "awaiting_decision" {
		t.Fatal("the review reads as waiting on a colleague nobody was asked")
	}
}

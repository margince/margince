// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// review reads one review as its sender is served it.
func (c *consentEnv) review(t *testing.T, id string) servedReview {
	t.Helper()
	var served servedReview
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+id, nil, nil, &served); status != http.StatusOK {
		t.Fatalf("reading review %s → %d", id, status)
	}
	return served
}

// confirmationOf is the Send anyway a sender presses on what they were shown.
func confirmationOf(r servedReview) AnyMap {
	body := directedShowing(r.reasons()...)
	body["assessment_revision"] = r.AssessmentRevision
	return body
}

// An objection recorded after the sender read the review voids the decision
// they were about to take. The held message keeps its words. The press gets a
// fresh review naming the objection, and the sender decides again.
func TestAnObjectionAfterTheReviewAsksTheSenderToDecideAgain(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("marketing with no grant → %d, want 409", status)
	}
	read := c.review(t, liveReviewID(t, c))
	if slices.Contains(read.reasons(), commsauthz.ReasonObjection) {
		t.Fatalf("the first review already names an objection %v: the fixture proves nothing", read.reasons())
	}
	before := heldSends(t, c.AppEnv)

	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/suppress", AnyMap{
		"kind": "marketing_objection", "reason": "They objected on the phone after the review.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("recording the objection → %d", status)
	}

	var stale refusedSend
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+read.ID+"/direct-send", confirmationOf(read), nil, &stale); status != http.StatusConflict {
		t.Fatalf("the stale confirmation → %d, want 409 with a fresh review", status)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`) + c.count(t, `SELECT count(*) FROM communication_instruction`); n != 0 {
		t.Fatalf("%d deliveries and decisions after the stale confirmation, want none", n)
	}
	after := heldSends(t, c.AppEnv)
	if len(after) != 1 || len(before) != 1 || after[0].id != before[0].id || after[0].payload != before[0].payload {
		t.Fatalf("the held message changed under the refresh:\nbefore %+v\nafter  %+v", before, after)
	}
	if stale.Details.ReviewID == "" || stale.Details.ReviewID == read.ID {
		t.Fatalf("the 409 names review %q, want a fresh one beside %s", stale.Details.ReviewID, read.ID)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_review WHERE resolved_at IS NULL`); n != 1 {
		t.Fatalf("%d live reviews after the refresh, want only the fresh one", n)
	}

	fresh := c.review(t, stale.Details.ReviewID)
	if !slices.Contains(fresh.reasons(), commsauthz.ReasonObjection) {
		t.Fatalf("the fresh review names %v, want the objection", fresh.reasons())
	}
	var sent struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+fresh.ID+"/direct-send", confirmationOf(fresh), nil, &sent); status != http.StatusCreated {
		t.Fatalf("deciding again on the fresh review → %d, want 201", status)
	}
	var deliveries, stops int
	if err := c.Owner.QueryRow(t.Context(), `
		SELECT (SELECT count(*) FROM comms_outbound WHERE execution_authority = 'instruction'),
		       (SELECT count(*) FROM communication_suppression WHERE contact_id = @contact AND revoked_at IS NULL)`,
		pgx.StrictNamedArgs{"contact": c.contactID}).Scan(&deliveries, &stops); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 || stops != 1 {
		t.Fatalf("deliveries = %d, live stops = %d after the new decision, want 1 and 1", deliveries, stops)
	}
}

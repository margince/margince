// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPublicBookingPendingCancellationAndPausedPage(t *testing.T) {
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	monday := nextMonday()
	body := AnyMap{"start": monday.Add(time.Hour), "end": monday.Add(90 * time.Minute), "booker": AnyMap{"name": "Guest", "email": "guest@visitor.example"}, "consent": AnyMap{"policy_version": "2026-01", "wording": "Contact me about this meeting."}}
	query := "?from=" + monday.Format(time.RFC3339) + "&to=" + monday.Add(8*time.Hour).Format(time.RFC3339) + "&duration_minutes=60"
	if status := publicCall(t, e, "GET", base+"/availability"+query, nil, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported public duration: %d", status)
	}
	if status := publicCall(t, e, "POST", base, body, map[string]string{"Idempotency-Key": "guessable"}, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("weak recovery capability: %d", status)
	}
	var result struct {
		Booking    string `json:"booking"`
		Invitation struct {
			ID      string `json:"id"`
			Token   string `json:"management_token"`
			Version int    `json:"version"`
		} `json:"invitation"`
	}
	if status := publicCall(t, e, "POST", base, body, nil, &result); status != 201 || result.Booking != "pending" || result.Invitation.Token == "" {
		t.Fatalf("booking: %d %+v", status, result)
	}
	var state struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	path := "/v1/public/meeting/" + result.Invitation.Token
	if status := publicCall(t, e, "GET", path, nil, nil, &state); status != 200 || state.Status != "pending" {
		t.Fatalf("guest state: %d %+v", status, state)
	}
	if status := publicCall(t, e, "PATCH", path, AnyMap{"action": "cancel", "version": state.Version}, nil, &state); status != http.StatusAccepted || state.Status != "canceling" {
		t.Fatalf("guest cancellation: %d %+v", status, state)
	}
	var profile AnyMap
	if status := e.Call(t, "GET", "/v1/scheduling/profile", nil, nil, &profile); status != 200 {
		t.Fatal(status)
	}
	profile["enabled"] = false
	if status := e.Call(t, "PUT", "/v1/scheduling/profile", profile, nil, nil); status != 200 {
		t.Fatal(status)
	}
	body["delivery"] = "record_only"
	if status := publicCall(t, e, "POST", base, body, nil, nil); status != http.StatusNotFound {
		t.Fatalf("paused record-only bypass: %d", status)
	}
}

func TestProposalReplayRehydratesTheVaultWithoutStoringTheCapability(t *testing.T) {
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": "Proposal Guest", "emails": []AnyMap{{"email": "guest@visitor.example", "is_primary": true}}}, nil, &contact); status != 201 {
		t.Fatalf("contact: %d", status)
	}
	headers := map[string]string{"Idempotency-Key": "proposal-recovery"}
	body := AnyMap{"contact_id": contact.ID, "attendee_email": "guest@visitor.example", "subject": "Discuss the project", "duration_minutes": 30, "options": []AnyMap{}}
	var first, retry struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	for _, out := range []any{&first, &retry} {
		if status := e.Call(t, "POST", "/v1/scheduling/proposals", body, headers, out); status != 201 {
			t.Fatalf("proposal: %d", status)
		}
	}
	if first != retry || first.URL == "" {
		t.Fatalf("lost response recovery: %+v %+v", first, retry)
	}
	var recorded string
	if err := e.Owner.QueryRow(context.Background(), `SELECT response_body FROM idempotency_key WHERE key=$1`, headers["Idempotency-Key"]).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(recorded, first.URL) || strings.Contains(recorded, "proposal-") {
		t.Fatal("plaintext capability in replay storage")
	}
}

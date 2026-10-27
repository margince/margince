// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestBookingHTTPIntentIsDeliveredAndCanceledByTheComposedWorker(t *testing.T) {
	e, provider := setupBookingProvider(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	monday := nextMonday()
	body := AnyMap{"start": monday.Add(time.Hour), "end": monday.Add(90 * time.Minute), "booker": AnyMap{"name": "Worker Guest", "email": "worker@visitor.example"}, "consent": AnyMap{"policy_version": "2026-01", "wording": "Contact me about this meeting."}}
	var result struct {
		Invitation struct {
			Token string `json:"management_token"`
		} `json:"invitation"`
	}
	if status := publicCall(t, e, "POST", "/v1/public/booking/"+bookingSlug(t, e), body, nil, &result); status != 201 {
		t.Fatal(status)
	}
	runner, completed, failed := startBookingWorker(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := runner.Enqueue(ctx, compose.MeetingDeliveryArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	if !jobtest.AwaitKindOutcome(ctx, t, completed, failed, "meeting_delivery") {
		t.Fatal("calendar delivery worker failed")
	}
	path := "/v1/public/meeting/" + result.Invitation.Token
	var state struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if status := publicCall(t, e, "GET", path, nil, nil, &state); status != 200 || state.Status != "confirmed" {
		t.Fatalf("worker did not confirm provider receipt: %d %+v", status, state)
	}
	provider.mu.Lock()
	provider.calendar = "different-account@example.com"
	provider.mu.Unlock()
	if _, err := e.Owner.Exec(ctx, `UPDATE meeting_invitation SET next_attempt_at='2000-01-01'`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Enqueue(ctx, compose.MeetingDeliveryArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	if jobtest.AwaitKindOutcome(ctx, t, completed, failed, "meeting_delivery") {
		t.Fatal("lost calendar authority was hidden")
	}
	if status := publicCall(t, e, "GET", path, nil, nil, &state); status != 200 || state.Status != "needs_attention" {
		t.Fatalf("account switch misreported as cancellation: %d %+v", status, state)
	}
	provider.mu.Lock()
	provider.calendar = "ada@example.com"
	provider.mu.Unlock()
	if status := publicCall(t, e, "PATCH", path, AnyMap{"action": "cancel", "version": state.Version}, nil, &state); status != 202 {
		t.Fatal(status)
	}
	if err := runner.Enqueue(ctx, compose.MeetingDeliveryArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	if !jobtest.AwaitKindOutcome(ctx, t, completed, failed, "meeting_delivery") {
		t.Fatal("calendar cancellation worker failed")
	}
	if status := publicCall(t, e, "GET", path, nil, nil, &state); status != 200 || state.Status != "canceled" {
		t.Fatalf("worker did not cancel event: %d %+v", status, state)
	}
}

func TestALeadBookingSurvivesReplayPromotionDeliveryAndCancellation(t *testing.T) {
	e, _ := setupBookingProvider(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	const address = "booker@lead.example"
	var lead struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/leads", AnyMap{"full_name": "Lea Booker", "email": address}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("lead: %d", status)
	}

	monday := nextMonday()
	body := AnyMap{
		"start": monday.Add(time.Hour), "end": monday.Add(90 * time.Minute),
		"booker":  AnyMap{"name": "Lea Booker", "email": address},
		"consent": AnyMap{"policy_version": "2026-01", "wording": "Contact me about this meeting."},
	}
	headers := map[string]string{"Idempotency-Key": "9b823423-076d-4b33-9b18-496be84ec0a1"}
	var first, replay struct {
		Invitation struct {
			ID      string `json:"id"`
			Token   string `json:"management_token"`
			Version int    `json:"version"`
		} `json:"invitation"`
	}
	path := "/v1/public/booking/" + bookingSlug(t, e)
	if status := publicCall(t, e, http.MethodPost, path, body, headers, &first); status != http.StatusCreated {
		t.Fatalf("lead booking: %d", status)
	}
	if status := publicCall(t, e, http.MethodPost, path, body, headers, &replay); status != http.StatusCreated {
		t.Fatalf("lead booking replay: %d", status)
	}
	if first != replay || first.Invitation.ID == "" || first.Invitation.Token == "" {
		t.Fatalf("replay did not recover the booking: first=%+v replay=%+v", first, replay)
	}

	var contacts int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM contact_email WHERE lower(email) = lower($1) AND archived_at IS NULL`, address).Scan(&contacts); err != nil {
		t.Fatal(err)
	}
	if contacts != 0 {
		t.Fatalf("lead booking created %d contact addresses", contacts)
	}
	var meetings int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id
		WHERE a.kind = 'meeting' AND al.lead_id = $1`, lead.ID).Scan(&meetings); err != nil {
		t.Fatal(err)
	}
	if meetings != 1 {
		t.Fatalf("lead booking filed %d meetings on the lead, want one", meetings)
	}
	var tasks int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id
		WHERE a.kind = 'task' AND a.source_system = $1 AND al.lead_id = $2 AND NOT a.is_done`, provenance.LeadRepliedSource, lead.ID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 {
		t.Fatalf("lead booking replay left %d open qualify tasks, want one", tasks)
	}
	var grants int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM contact_consent WHERE lead_id = $1 AND contact_id IS NULL AND state = 'granted'`, lead.ID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 1 {
		t.Fatalf("lead booking recorded %d lead consent grants, want one", grants)
	}
	var promoted struct {
		Contact struct {
			ID string `json:"id"`
		} `json:"contact"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/leads/"+lead.ID+"/promote",
		AnyMap{"trigger": "human_qualify"}, nil, &promoted); status != http.StatusOK || promoted.Contact.ID == "" {
		t.Fatalf("promote booked lead before delivery: %d %+v", status, promoted)
	}

	runner, completed, failed := startBookingWorker(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := runner.Enqueue(ctx, compose.MeetingDeliveryArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	if !jobtest.AwaitKindOutcome(ctx, t, completed, failed, "meeting_delivery") {
		t.Fatal("lead booking delivery failed")
	}
	meetingPath := "/v1/public/meeting/" + first.Invitation.Token
	var state struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if status := publicCall(t, e, http.MethodGet, meetingPath, nil, nil, &state); status != http.StatusOK || state.Status != "confirmed" {
		t.Fatalf("lead booking was not confirmed: %d %+v", status, state)
	}
	if status := publicCall(t, e, http.MethodPatch, meetingPath, AnyMap{"action": "cancel", "version": state.Version}, nil, &state); status != http.StatusAccepted {
		t.Fatalf("cancel lead booking: %d", status)
	}
	if err := runner.Enqueue(ctx, compose.MeetingDeliveryArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	if !jobtest.AwaitKindOutcome(ctx, t, completed, failed, "meeting_delivery") {
		t.Fatal("lead booking cancellation failed")
	}
	if status := publicCall(t, e, http.MethodGet, meetingPath, nil, nil, &state); status != http.StatusOK || state.Status != "canceled" {
		t.Fatalf("lead booking was not canceled: %d %+v", status, state)
	}
}

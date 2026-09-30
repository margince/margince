// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/identity"
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
	db := compose.InstallationDB(e.Pool)
	registry := capture.NewRegistry(db, capture.NewSink(db), identity.NewServiceFor(db), e.Vault)
	registry.Register(gcal.New(gcal.NewOAuth(gcal.OAuthConfig{ClientID: "calendar-fixture", ClientSecret: "calendar-fixture"}), gcal.NewAPI(nil, "")))
	runner, completed, failed := jobtest.StartTestJobRunner(t, e.Pool, compose.JobRunnerConfig{GmailRegistry: registry, ControllerVault: e.Vault, SendOrigin: compose.SendOrigin{PublicBaseURL: "https://mail.example.test"}})
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

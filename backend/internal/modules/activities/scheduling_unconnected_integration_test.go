// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A host whose calendar stopped answering after the profile was saved is asked
// to reconnect, from the invite button and from the calendar picker alike.
func TestInvitingWithoutAUsableCalendarIsRefusedNamingTheProvider(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	clock := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	calendar := &invitationCalendar{}
	store := e.store(nil).WithClock(func() time.Time { return clock }).
		WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).
		WithSchedulingCalendar(calendar).WithMeetingVault(keyvault.NewMemory()).WithPublicBaseURL("https://crm.example.com")
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	profile.Enabled = true
	name := "Test Host"
	profile.HostName = &name
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	contact := ids.NewV7()
	args := schedulingArgs{}
	if _, err := e.owner.Exec(ctx, `INSERT INTO contact(id,full_name,source,captured_by)VALUES(`+args.add(contact)+`,'Guest','manual','human:test')`, args...); err != nil {
		t.Fatal(err)
	}
	calendar.checkErr = connector.ErrAuthRejected

	_, err := store.CreateInvitation(ctx, crmcontracts.MeetingInvitationRequest{
		ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Project meeting",
		Start: clock.Add(3 * time.Hour), End: clock.Add(3*time.Hour + 30*time.Minute),
	})

	var refusal *SchedulingArgumentError
	if !errors.As(err, &refusal) || refusal.Field != "provider" || refusal.Code != "required" {
		t.Fatalf("inviting answered %v, want a refusal naming provider", err)
	}
	calendar.listErr = connector.ErrAuthRejected

	rec := httptest.NewRecorder()
	Handlers{store: store}.GetSchedulingCalendars(rec,
		httptest.NewRequest(http.MethodGet, "/v1/scheduling/calendars?provider=gcal", nil).WithContext(ctx),
		crmcontracts.GetSchedulingCalendarsParams{Provider: "gcal"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("listing calendars answered %d, want 422", rec.Code)
	}
}

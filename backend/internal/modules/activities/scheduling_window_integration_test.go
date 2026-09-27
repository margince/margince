// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestReliableAvailabilityDistinguishesBusyWeeksFromBookingHorizon(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	now := time.Date(2026, 9, 27, 6, 0, 0, 0, time.UTC)
	calendar := &invitationCalendar{busy: []connector.CalendarInterval{{Start: now, End: now.AddDate(0, 0, 14)}}}
	store := e.store(nil).WithClock(func() time.Time { return now }).WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).WithSchedulingCalendar(calendar)
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	host := ids.UserID{UUID: e.rep}
	slots, _, err := store.ReliableAvailability(ctx, host, now, now.AddDate(0, 0, 7), 30*time.Minute)
	if err != nil || len(slots) != 0 {
		t.Fatalf("busy week: %v %v", slots, err)
	}
	slots, _, err = store.ReliableAvailability(ctx, host, now.AddDate(0, 0, 15), now.AddDate(0, 0, 22), 30*time.Minute)
	if err != nil || len(slots) == 0 {
		t.Fatalf("free week inside horizon: %v %v", slots, err)
	}
	november := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	_, _, err = store.ReliableAvailability(ctx, host, november, november.AddDate(0, 0, 7), 30*time.Minute)
	var validation *SchedulingArgumentError
	if !errors.As(err, &validation) || validation.Code != "booking_horizon" {
		t.Fatalf("November outside horizon: %v", err)
	}
	profile.HorizonDays = 90
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	slots, _, err = store.ReliableAvailability(ctx, host, november, november.AddDate(0, 0, 7), 30*time.Minute)
	if err != nil || len(slots) == 0 {
		t.Fatalf("November inside extended horizon: %v %v", slots, err)
	}
}

func TestRescheduleAvailabilityExplainsTheBookingHorizon(t *testing.T) {
	f := newInvitationFixture(t)
	meeting := f.book(t)
	from := f.now.AddDate(0, 0, 35)
	_, _, err := f.store.invitationAvailability(f.ctx, ids.UserID{UUID: f.env.rep}, ids.UUID(meeting.Id), from, from.AddDate(0, 0, 7))
	var validation *SchedulingArgumentError
	if !errors.As(err, &validation) || validation.Code != "booking_horizon" {
		t.Fatalf("reschedule window: %v", err)
	}
}

func TestPublicAvailabilityReturnsAWindowErrorInsteadOfAnEmptyCalendar(t *testing.T) {
	f := newInvitationFixture(t)
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile.Enabled = true
	name := "Test Host"
	profile.HostName = &name
	profile, err = f.store.SaveSchedulingProfile(f.ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	from := f.now.AddDate(0, 0, 35)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(f.ctx)
	Handlers{store: f.store}.GetPublicAvailability(response, request, *profile.Slug, crmcontracts.GetPublicAvailabilityParams{From: from, To: from.AddDate(0, 0, 7)})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("public status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "booking_horizon") {
		t.Fatalf("missing window explanation: %s", response.Body.String())
	}
}

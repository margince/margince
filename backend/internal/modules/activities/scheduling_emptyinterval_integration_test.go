// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestReliableAvailabilityIgnoresEmptyEventsBeforeAddingBuffers(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	empty := now.Add(4 * time.Hour)
	calendar := &invitationCalendar{busy: []connector.CalendarInterval{{Start: empty, End: empty}}}
	store := e.store(nil).WithClock(func() time.Time { return now }).WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).WithSchedulingCalendar(calendar)
	profile := defaultSchedulingProfile()
	profile.Provider = crmcontracts.SchedulingProfileProviderGcal
	profile.BufferMinutes = 15
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	host := ids.UserID{UUID: e.rep}
	slots, _, err := store.ReliableAvailability(ctx, host, now, now.Add(24*time.Hour), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, start := range []time.Time{empty.Add(-30 * time.Minute), empty.Add(-15 * time.Minute), empty} {
		found := false
		for _, slot := range slots {
			found = found || slot.Start.Equal(start)
		}
		if !found {
			t.Fatalf("no slot at %s around the placeholder: %+v", start, slots)
		}
	}
	if _, err := store.validateInvitationTime(ctx, host, empty.Add(-15*time.Minute), empty.Add(15*time.Minute)); err != nil {
		t.Fatalf("empty event blocked booking: %v", err)
	}
	calendar.busy[0].End = empty.Add(-time.Minute)
	_, err = store.calendarBusy(ctx, host, profile, now, now.Add(24*time.Hour), "")
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok || unavailable.Reason != "invalid_interval" {
		t.Fatalf("reversed interval did not fail closed: %v", err)
	}
}

func TestAnEmptyProviderEventDoesNotBlockInvitationDelivery(t *testing.T) {
	f := newInvitationFixture(t)
	f.calendar.busy = []connector.CalendarInterval{{Start: f.request.Start, End: f.request.Start}}
	meeting := f.book(t)
	if meeting.Status != "confirmed" {
		t.Fatalf("empty event blocked the invitation: %+v", meeting)
	}
}

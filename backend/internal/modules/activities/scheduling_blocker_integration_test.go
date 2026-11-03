// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A page saved before the profile existed names no calendar. The host reads
// it as blocked for want of one, and a guest reads it as paused. That is the
// answer the availability and booking calls give by refusing.
func TestAPageWithNoCalendarIsBlockedForTheHostAndPausedForTheGuest(t *testing.T) {
	f := setupBooking(t)
	if _, err := f.e.owner.Exec(f.ctx, `UPDATE booking_page SET scheduling_policy = NULL WHERE host_user_id = $1`, f.e.rep); err != nil {
		t.Fatal(err)
	}
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Blocker == nil || *profile.Blocker != crmcontracts.SchedulingProfileBlockerNoCalendar {
		t.Errorf("blocker = %v, want no_calendar", profile.Blocker)
	}
	guest, err := f.store.hostSchedulingProfile(f.ctx, ids.From[ids.UserKind](f.e.rep))
	if err != nil {
		t.Fatal(err)
	}
	if !publicProfile(guest, nil).Paused {
		t.Error("a page with no calendar reads as taking bookings to its guest")
	}
}

func TestAConnectedPageIsUnblockedAndAnUnusableOneSaysSo(t *testing.T) {
	f := setupBooking(t)
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Blocker == nil || *profile.Blocker != crmcontracts.SchedulingProfileBlockerNone {
		t.Errorf("a connected page reads blocker %v, want none", profile.Blocker)
	}
	if publicProfile(profile, nil).Paused {
		t.Error("a live page reads as paused to its guest")
	}
	f.calendar.checkErr = connector.ErrAuthRejected
	profile, err = f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Blocker == nil || *profile.Blocker != crmcontracts.SchedulingProfileBlockerCalendarUnavailable {
		t.Errorf("an unusable calendar reads blocker %v, want calendar_unavailable", profile.Blocker)
	}
}

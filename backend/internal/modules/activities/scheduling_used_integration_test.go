// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// pageFirstUsedAt reads the live page's first use, nil while it is unused.
func pageFirstUsedAt(t *testing.T, f *invitationFixture) *time.Time {
	t.Helper()
	var used *time.Time
	args := schedulingArgs{}
	if err := f.env.owner.QueryRow(f.ctx, `SELECT first_used_at FROM booking_page WHERE revoked_at IS NULL AND host_user_id=`+args.add(f.env.rep), args...).Scan(&used); err != nil {
		t.Fatal(err)
	}
	return used
}

// bookPublicly books the request through the public page, as a guest does.
func bookPublicly(t *testing.T, f *invitationFixture, key string, start time.Time) {
	t.Helper()
	body := crmcontracts.BookPublicMeetingJSONRequestBody{}
	body.Booker.Email = "guest@example.test"
	req := httptest.NewRequest(http.MethodPost, "/public/booking/host", nil)
	req.Header.Set("Idempotency-Key", key)
	intent, err := publicBookingRequestIntent(req, "host", body)
	if err != nil {
		t.Fatal(err)
	}
	host, err := schedulingHost(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	request := f.request
	request.Start, request.End = start, start.Add(30*time.Minute)
	if _, err := f.store.reserveAndQueueInvitation(f.ctx, host, request, invitationIntent{Public: intent}); err != nil {
		t.Fatal(err)
	}
}

func TestAPublicBookingMarksThePageUsedOnce(t *testing.T) {
	f := newInvitationFixture(t)
	f.book(t)
	if used := pageFirstUsedAt(t, f); used != nil {
		t.Fatalf("an invitation the host sent marked the page used at %v", used)
	}
	bookPublicly(t, f, "2943cb96-b3c5-4445-8685-20d6fe8bb026", monday(13))
	first := pageFirstUsedAt(t, f)
	if first == nil || !first.Equal(f.now) {
		t.Fatalf("first use = %v, want the booking time %v", first, f.now)
	}
	f.now = f.now.Add(time.Hour)
	bookPublicly(t, f, "5f8a0c1e-2d7b-4f7e-9a51-0c3b6d2e4f81", monday(14))
	if again := pageFirstUsedAt(t, f); again == nil || !again.Equal(*first) {
		t.Fatalf("a second booking moved the first use to %v, want %v", again, first)
	}
}

func TestCopyingTheLinkMarksThePageUsedAndAuditsOnce(t *testing.T) {
	f := newInvitationFixture(t)
	for range 2 {
		if err := f.store.MarkBookingPageShared(f.ctx); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(time.Minute)
	}
	if used := pageFirstUsedAt(t, f); used == nil || !used.Equal(monday(6)) {
		t.Fatalf("first use = %v, want the first copy at %v", used, monday(6))
	}
	var audits, events int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM audit_log WHERE entity_type='booking_page' AND action='update' AND after->>'first_used_at' IS NOT NULL`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='booking_page.updated'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	// The fixture's own save left one event; the first copy adds one more.
	if audits != 1 || events != 2 {
		t.Fatalf("two copies left %d audits and %d events, want one audit and one more event", audits, events)
	}
}

func TestReplacingTheLinkKeepsItsFirstUse(t *testing.T) {
	f := newInvitationFixture(t)
	if err := f.store.MarkBookingPageShared(f.ctx); err != nil {
		t.Fatal(err)
	}
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	replace := true
	profile.ReplaceLink = &replace
	f.now = f.now.Add(time.Hour)
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	if used := pageFirstUsedAt(t, f); used == nil || !used.Equal(monday(6)) {
		t.Fatalf("the replaced page's first use = %v, want %v carried over", used, monday(6))
	}
}

// Home names a booking page only when it is switched on, was used, and takes
// no bookings. Each case breaks one of the three.
func TestABookingPageConcernNeedsASwitchedOnUsedPausedPage(t *testing.T) {
	cases := []struct {
		name                 string
		enabled, used, block bool
		want                 bool
	}{
		{name: "used, switched on and paused", enabled: true, used: true, block: true, want: true},
		{name: "never used", enabled: true, block: true},
		{name: "switched off", used: true, block: true},
		{name: "taking bookings", enabled: true, used: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInvitationFixture(t)
			savePage(t, f, tc.enabled)
			if tc.used {
				if err := f.store.MarkBookingPageShared(f.ctx); err != nil {
					t.Fatal(err)
				}
			}
			if tc.block {
				f.calendar.checkErr = connector.ErrAuthRejected
			}
			concern, paused, err := f.store.BookingPageConcern(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if paused != tc.want {
				t.Fatalf("paused = %v, want %v", paused, tc.want)
			}
			if tc.want && (concern.Blocker != crmcontracts.SchedulingProfileBlockerCalendarUnavailable || concern.PageID == ids.Nil) {
				t.Errorf("concern = %+v, want the page and calendar_unavailable", concern)
			}
		})
	}
}

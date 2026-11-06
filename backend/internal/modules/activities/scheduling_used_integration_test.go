// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	if err := tryBookingPublicly(f, key, start); err != nil {
		t.Fatal(err)
	}
}

func tryBookingPublicly(f *invitationFixture, key string, start time.Time) error {
	body := crmcontracts.BookPublicMeetingJSONRequestBody{}
	body.Booker.Email = "guest@example.test"
	req := httptest.NewRequest(http.MethodPost, "/public/booking/host", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	intent, err := publicBookingRequestIntent(req, "host", body)
	if err != nil {
		return err
	}
	host, err := schedulingHost(f.ctx)
	if err != nil {
		return err
	}
	request := f.request
	request.Start, request.End = start, start.Add(30*time.Minute)
	_, err = f.store.reserveAndQueueInvitation(f.ctx, host, request, invitationIntent{Public: intent, DescriptionFromBooker: true})
	return err
}

// firstUseWrites counts the audit rows that stamped a first use, and every
// booking_page.updated event, the fixture's own save included.
func firstUseWrites(t *testing.T, f *invitationFixture) (audits, events int) {
	t.Helper()
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM audit_log WHERE entity_type='booking_page' AND action='update' AND after->>'first_used_at' IS NOT NULL`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='booking_page.updated'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	return audits, events
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
	// The fixture's own save left one event; the first booking adds one more.
	if audits, events := firstUseWrites(t, f); audits != 1 || events != 2 {
		t.Fatalf("two bookings left %d first-use audits and %d events, want one audit and one more event", audits, events)
	}
}

// A booking that lands while the host replaces their link waits for the
// replacement, then stamps the new live page.
func TestABookingDuringALinkReplacementStampsTheNewPage(t *testing.T) {
	f := newInvitationFixture(t)
	host, err := schedulingHost(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	replaced, release := make(chan error, 1), make(chan struct{})
	go func() {
		replaced <- f.store.tx(f.ctx, func(tx pgx.Tx) error {
			if err := storekit.LockWriteIdentity(f.ctx, tx, "booking_page", host.String()); err != nil {
				return err
			}
			if _, err := f.store.replaceBookingPage(f.ctx, tx, host, false); err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	waitForLockWaiters(t, f, 0, nil)
	booked := make(chan error, 1)
	go func() { booked <- tryBookingPublicly(f, "7b1e2c3d-4a5f-4b6c-8d7e-9f0a1b2c3d4e", monday(13)) }()
	waitForLockWaiters(t, f, 1, booked)
	close(release)
	if err := <-replaced; err != nil {
		t.Fatal(err)
	}
	if err := <-booked; err != nil {
		t.Fatal(err)
	}
	if used := pageFirstUsedAt(t, f); used == nil {
		t.Fatal("the booking landed during the replacement and the new live page reads as never used")
	}
}

// waitForLockWaiters waits until at least want backends of this database wait
// on a lock. With want 0 it waits for the replacement to hold its own. A
// booking that finishes first did not wait for the replacement at all.
func waitForLockWaiters(t *testing.T, f *invitationFixture, want int, booked <-chan error) {
	t.Helper()
	// By backend, because a row-lock wait names a transaction and no database.
	ours := `pid IN (SELECT pid FROM pg_stat_activity WHERE datname = current_database())`
	query := `SELECT count(*) FROM pg_locks WHERE NOT granted AND ` + ours
	if want == 0 {
		query = `SELECT count(*) FROM pg_locks WHERE granted AND locktype = 'advisory' AND ` + ours
		want = 1
	}
	// Bounded by attempts, so no wall clock decides the outcome.
	for range 20000 {
		select {
		case err := <-booked:
			t.Fatalf("the booking finished while the link replacement was still open (err %v)", err)
		default:
		}
		var waiting int
		if err := f.env.owner.QueryRow(f.ctx, query).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
	}
	t.Fatalf("no lock reached %d holders or waiters", want)
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
	audits, events := firstUseWrites(t, f)
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
	if audits, _ := firstUseWrites(t, f); audits != 2 {
		t.Fatalf("first-use audit rows = %d, want the copy's and the carry's", audits)
	}
}

// A browser that sends no key still books through the public door.
func TestAPublicBookingWithoutAKeyMarksThePageUsed(t *testing.T) {
	f := newInvitationFixture(t)
	bookPublicly(t, f, "", monday(10))
	if used := pageFirstUsedAt(t, f); used == nil {
		t.Fatal("a public booking sent without a key left the page unused")
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

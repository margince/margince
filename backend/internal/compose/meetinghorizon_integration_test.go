// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The reader's own Worklist says whether their calendar feeds the meetings
// count and, on a day with none left, when the next customer meeting is —
// against a real database, through the route's own wiring.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// horizonReader is Rep1 reading their own day, team-scoped, so a colleague's
// private contact is out of reach.
func horizonReader(e *integration.Env) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
}

// readOwnDay reads Rep1's Worklist with the clock pinned at now.
func readOwnDay(t *testing.T, e *integration.Env, now time.Time) crmcontracts.Worklist {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	day, err := feed.Worklist(horizonReader(e), "mine", "", ids.Nil, 25, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	return day
}

// seedCalendar files one gcal connection for Rep1 in a status, and a failing
// sync beside it when failing is set. Hand-inserted because the only writer is
// the OAuth grant flow, which needs a provider.
func seedCalendar(t *testing.T, e *integration.Env, provider, status string, failing bool) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var id ids.UUID
		if err := tx.QueryRow(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, scopes, status, auth)
			VALUES ($1, $2, '{}', $3, $4) RETURNING id`,
			provider, e.Rep1, status, []byte(`{"refresh_token":"r","granted":[]}`)).Scan(&id); err != nil {
			return err
		}
		if !failing {
			return nil
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_sync_state (connection_id, next_sync_at, consecutive_failures,
			                                last_synced_at, last_error_class, failing_since)
			VALUES ($1, now(), 3, now(), 'unreachable', now())`, id)
		return err
	}); err != nil {
		t.Fatalf("seeding the %s connection at %s: %v", provider, status, err)
	}
}

func TestTheReadersCalendarSaysWhetherItFeedsTheMeetingsCount(t *testing.T) {
	type connection struct {
		provider, status string
		failing          bool
	}
	for _, tc := range []struct {
		name string
		seed []connection
		want crmcontracts.WorklistCalendar
	}{
		{"none", nil, crmcontracts.WorklistCalendarNotConnected},
		{"only a mailbox", []connection{{"gmail", "connected", false}}, crmcontracts.WorklistCalendarNotConnected},
		{"parked", []connection{{"gcal", "disconnected", false}}, crmcontracts.WorklistCalendarNotConnected},
		{"connected", []connection{{"gcal", "connected", false}}, crmcontracts.WorklistCalendarConnected},
		{"wants reauthorisation", []connection{{"gcal", "reauth_required", false}}, crmcontracts.WorklistCalendarUnreadable},
		{"in error", []connection{{"gcal", "error", false}}, crmcontracts.WorklistCalendarUnreadable},
		{"sync failing", []connection{{"gcal", "connected", true}}, crmcontracts.WorklistCalendarUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			for _, conn := range tc.seed {
				seedCalendar(t, e, conn.provider, conn.status, conn.failing)
			}

			day := readOwnDay(t, e, todayAt(9*time.Hour))

			if day.Calendar == nil || *day.Calendar != tc.want {
				t.Fatalf("calendar = %v, want %q", day.Calendar, tc.want)
			}
		})
	}
}

// horizonContacts makes the two people a customer meeting is with: one the
// reader may see, and a colleague's private buyer they may not.
func horizonContacts(t *testing.T, e *integration.Env) (visible, hidden ids.UUID) {
	t.Helper()
	visible = partiesContact(t, e, "Blair Shared", ids.UUID{}, nil)
	hidden = partiesContact(t, e, "Alex Private", e.Rep3, nil)
	private := "owner"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](hidden),
		contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the colleague's buyer private: %v", err)
	}
	return visible, hidden
}

// bookOwnMeeting logs a meeting Rep1 hosts at an instant, linked to the given
// contacts — none makes it an internal meeting.
func bookOwnMeeting(t *testing.T, e *integration.Env, subject string, at time.Time, status string, with ...ids.UUID) ids.UUID {
	t.Helper()
	return bookMeetingFor(t, e, horizonReader(e), subject, at, status, with...)
}

// bookMeetingFor logs a meeting Rep1 hosts as author, who must be able to link
// every contact named.
func bookMeetingFor(
	t *testing.T, e *integration.Env, author context.Context, subject string, at time.Time, status string, with ...ids.UUID,
) ids.UUID {
	t.Helper()
	host := ids.From[ids.UserKind](e.Rep1)
	in := activities.LogActivityInput{Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", HostUserID: &host}
	if status != "" {
		in.MeetingStatus = &status
	}
	for _, contact := range with {
		in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "contact", EntityID: contact})
	}
	meeting, _, err := e.Activities.LogActivity(author, in)
	if err != nil {
		t.Fatalf("booking %q: %v", subject, err)
	}
	return ids.UUID(meeting.Id)
}

// An empty day names the soonest booked customer meeting past it — not the
// internal one before it, not the cancelled one — with only the people the
// reader may see in the room.
func TestAnEmptyDayNamesTheNextCustomerMeetingWithOnlyVisibleAttendees(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(9 * time.Hour)
	visible, hidden := horizonContacts(t, e)
	bookOwnMeeting(t, e, "Team sync", now.Add(48*time.Hour), "")
	bookOwnMeeting(t, e, "Called off", now.Add(60*time.Hour), "canceled", visible)
	// Booked by the colleague who owns the private buyer, onto Rep1's calendar.
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AccountRepPerms)
	want := bookMeetingFor(t, e, colleague, "Quarterly review", now.Add(72*time.Hour), "", visible, hidden)
	bookOwnMeeting(t, e, "Renewal", now.Add(96*time.Hour), "booked", visible)

	day := readOwnDay(t, e, now)

	next := day.NextMeeting
	if next == nil || ids.UUID(next.ActivityId) != want {
		t.Fatalf("next_meeting = %+v, want the quarterly review %v", next, want)
	}
	if next.Participants == nil || len(*next.Participants) != 1 || ids.UUID((*next.Participants)[0].ContactId) != visible {
		t.Errorf("participants = %+v, want only the buyer this reader may see %v", next.Participants, visible)
	}
}

// Past the thirty-day window is not "next", and a meeting left today is the
// next conversation already, so neither points anywhere.
func TestNoNextMeetingPastTheWindowOrBesideOneLeftToday(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   time.Duration
	}{
		{"forty days out", 40 * 24 * time.Hour},
		{"later today", 3 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			now := todayAt(9 * time.Hour)
			visible, _ := horizonContacts(t, e)
			bookOwnMeeting(t, e, "The meeting", now.Add(tc.at), "", visible)
			if tc.at < 24*time.Hour {
				bookOwnMeeting(t, e, "Three days out", now.Add(72*time.Hour), "", visible)
			}

			day := readOwnDay(t, e, now)

			if day.NextMeeting != nil {
				t.Errorf("next_meeting = %+v, want absent", day.NextMeeting)
			}
		})
	}
}

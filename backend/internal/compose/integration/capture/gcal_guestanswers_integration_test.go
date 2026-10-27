// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// crowdGuest is the address of the i-th outside guest on a crowded invitation.
func crowdGuest(i int) string { return fmt.Sprintf("guest%03d@conference.example", i) }

// crowdedList is an invitation naming the owner, one colleague and n outside
// guests, the first of whom is a contact already.
func crowdedList(n int) []map[string]any {
	list := []map[string]any{ownerEntry("accepted"), guestEntry(guestColleagueA, "accepted")}
	for i := range n {
		list = append(list, guestEntry(crowdGuest(i), "needsAction"))
	}
	return list
}

type answeredRow struct {
	userID, contactID *ids.UUID
	response          *string
	pastCap           bool
}

// answeredRows answers each address's guest rows on one meeting.
func answeredRows(t *testing.T, e *integration.SearchEnv, meeting ids.UUID) map[string][]answeredRow {
	t.Helper()
	out := map[string][]answeredRow{}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT coalesce(ap.address, u.email), ap.user_id, ap.contact_id, ap.response, ap.past_party_cap
			  FROM activity_participant ap LEFT JOIN app_user u ON u.id = ap.user_id
			 WHERE ap.activity_id = $1 AND ap.role IN ('organizer', 'attendee')`, meeting)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var address string
			var row answeredRow
			if err := rows.Scan(&address, &row.userID, &row.contactID, &row.response, &row.pastCap); err != nil {
				return err
			}
			out[address] = append(out[address], row)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func readMeeting(t *testing.T, e *integration.SearchEnv, meeting ids.UUID) crmcontracts.Activity {
	t.Helper()
	read, err := activities.NewStore(e.DB()).GetActivity(importerCtx(e), ids.From[ids.ActivityKind](meeting), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the meeting: %v", err)
	}
	return read
}

// A meeting past the party cap names every outside guest by address, files the
// meeting under none of them, makes no contact and credits nobody.
func TestACrowdedMeetingNamesItsOutsideGuestsAndCreditsNone(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	known := ensureGuestContact(t, e, crowdGuest(0))
	const outside = 58
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), crowdGuest(1), crowdedList(outside)...))
	connectFeed(t, env, feed)()
	meeting := gcalMeeting(t, e)

	rows := answeredRows(t, e, meeting)
	if len(rows[guestColleagueA]) != 1 || rows[guestColleagueA][0].userID == nil || *rows[guestColleagueA][0].userID != colleague {
		t.Errorf("the colleague on the crowded meeting: rows %+v, want their seat", rows[guestColleagueA])
	}
	for i := range outside {
		got := rows[crowdGuest(i)]
		if len(got) != 1 || got[0].contactID != nil || got[0].userID != nil || !got[0].pastCap {
			t.Errorf("outside guest %s: rows %+v, want one by address past the cap", crowdGuest(i), got)
		}
	}
	if links := countRows(t, e, `SELECT count(*) FROM activity_link WHERE activity_id = '`+meeting.String()+`' AND contact_id IS NOT NULL`); links != 0 {
		t.Errorf("the crowded meeting is filed under %d contacts, want none", links)
	}
	if minted := countRows(t, e, `SELECT count(*) FROM contact_email WHERE email LIKE '%@conference.example'`); minted != 1 {
		t.Errorf("%d contacts hold a guest address, want only the one that existed before", minted)
	}
	read := readMeeting(t, e, meeting)
	if listed := deref(read.Guests); len(listed) != outside {
		t.Errorf("the meeting lists %d guests, want all %d outside guests", len(listed), outside)
	}
	if read.GuestsLeftOut == nil || *read.GuestsLeftOut != 0 {
		t.Errorf("guests_left_out = %v, want 0", read.GuestsLeftOut)
	}

	later := ensureGuestContact(t, e, crowdGuest(2))
	if _, err := contacts.NewStore(e.DB()).RepairContactCohort(contactCreator(e), later); err != nil {
		t.Fatal(err)
	}
	if got := answeredRows(t, e, meeting)[crowdGuest(2)]; len(got) != 1 || got[0].contactID != nil {
		t.Errorf("a guest who became a contact later: rows %+v, want their row left by address", got)
	}
	for _, contact := range []ids.ContactID{known, later} {
		if meetingLinkedTo(t, e, meeting, contact) {
			t.Errorf("the crowded meeting is filed under contact %s", contact)
		}
		if edges := countRows(t, e, `SELECT count(*) FROM activity_participant WHERE contact_id = '`+contact.String()+`'`); edges != 0 {
			t.Errorf("contact %s stands on %d participant rows, which the interaction graph credits", contact, edges)
		}
	}
}

// Past connector.MaxGuestsPastCap the outside guests are counted, not stored.
func TestACrowdBeyondTheBoundIsCountedNotStored(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seatFor(t, e, guestColleagueA)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), crowdGuest(1), crowdedList(250)...))
	sync := connectFeed(t, env, feed)
	sync()
	meeting := gcalMeeting(t, e)

	stored := countRows(t, e, `SELECT count(*) FROM activity_participant WHERE activity_id = '`+meeting.String()+`' AND past_party_cap`)
	if stored != connector.MaxGuestsPastCap {
		t.Errorf("%d outside guests stored, want %d", stored, connector.MaxGuestsPastCap)
	}
	read := readMeeting(t, e, meeting)
	if read.GuestsLeftOut == nil || *read.GuestsLeftOut != 50 {
		t.Errorf("guests_left_out = %v, want 50", read.GuestsLeftOut)
	}
	if listed := deref(read.Guests); len(listed) != connector.MaxGuestsPastCap {
		t.Errorf("the meeting lists %d guests, want %d", len(listed), connector.MaxGuestsPastCap)
	}
	sync()
	if again := countRows(t, e, `SELECT count(*) FROM activity_participant WHERE activity_id = '`+meeting.String()+`' AND past_party_cap`); again != stored {
		t.Errorf("a second sync left %d outside guests, want %d", again, stored)
	}
}

// Each guest's answer is stored and shown: on the guest list, and for the
// colleagues and linked contacts beside it.
func TestEachGuestsAnswerIsStoredAndShown(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleagueA, colleagueB := seatFor(t, e, guestColleagueA), seatFor(t, e, guestColleagueB)
	known := ensureGuestContact(t, e, guestKnown)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...))
	connectFeed(t, env, feed)()
	meeting := gcalMeeting(t, e)

	rows := answeredRows(t, e, meeting)
	for address, want := range map[string]string{
		guestColleagueA: "accepted", guestColleagueB: "tentative", guestKnown: "accepted",
		guestLater: "declined", guestUnread: "needs_action", guestOptional: "needs_action",
	} {
		if len(rows[address]) != 1 || deref1(rows[address][0].response) != want {
			t.Errorf("%s: rows %+v, want one answering %q", address, rows[address], want)
		}
	}
	read := readMeeting(t, e, meeting)
	shown := map[string]crmcontracts.MeetingResponse{}
	for _, guest := range deref(read.Guests) {
		if guest.Response != nil {
			shown[deref1(guest.Address)] = *guest.Response
		}
	}
	if shown[guestLater] != crmcontracts.MeetingResponseDeclined || shown[guestUnread] != crmcontracts.MeetingResponseNeedsAction {
		t.Errorf("the guest list shows the answers %v", shown)
	}
	answers := map[ids.UUID]crmcontracts.MeetingResponse{}
	for _, answer := range deref(read.AttendeeResponses) {
		switch {
		case answer.UserId != nil:
			answers[ids.UUID(*answer.UserId)] = answer.Response
		case answer.ContactId != nil:
			answers[ids.UUID(*answer.ContactId)] = answer.Response
		}
	}
	if answers[colleagueA] != crmcontracts.MeetingResponseAccepted || answers[colleagueB] != crmcontracts.MeetingResponseTentative ||
		answers[known.UUID] != crmcontracts.MeetingResponseAccepted {
		t.Errorf("attendee_responses = %v, want colleague A accepted, B tentative and the contact accepted", answers)
	}
}

// A later sync changes an answer in place and audits it. A colleague who
// declines loses the seat the roster gave them and stays named by address.
func TestALaterSyncChangesAnAnswer(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	start := captureNow.AddDate(0, 0, -3)
	invite := func(colleagueAnswer, guestAnswer string) map[string]any {
		return lateGuestEvent("review", "confirmed", start, guestUnread,
			ownerEntry("accepted"), guestEntry(guestColleagueA, colleagueAnswer), guestEntry(guestUnread, guestAnswer))
	}
	feed := &calendarFeed{}
	feed.set(invite("needsAction", "needsAction"))
	sync := connectFeed(t, env, feed)
	sync()
	meeting := gcalMeeting(t, e)
	if !seatReads(t, e, colleague, meeting) {
		t.Fatal("the invited colleague cannot read the meeting")
	}

	feed.set(invite("declined", "tentative"))
	sync()

	rows := answeredRows(t, e, meeting)
	if got := rows[guestUnread]; len(got) != 1 || deref1(got[0].response) != "tentative" {
		t.Errorf("the outside guest: rows %+v, want one now answering tentative", got)
	}
	if got := rows[guestColleagueA]; len(got) != 1 || got[0].userID != nil || deref1(got[0].response) != "declined" {
		t.Errorf("the colleague who declined: rows %+v, want one by address answering declined", got)
	}
	if seatReads(t, e, colleague, meeting) {
		t.Error("a colleague who declined still reads the meeting")
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+meeting.String()+
		`' AND action = 'update' AND (after->>'guest_answers_changed')::int = 1`); got != 1 {
		t.Errorf("%d audit rows record the changed answer, want 1", got)
	}
}

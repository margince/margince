// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Whose meeting it is (storekit.MeetingResponsibilitySQL) and who is in it
// (activities.MeetingRoomColumns), against rows the real writers produced.

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bookHostedMeeting logs one meeting hosted by a named seat and adds the
// extra participant rows a capture or a copy would have written.
func bookHostedMeeting(t *testing.T, e *integration.Env, subject string, at time.Time, host ids.UUID, extra map[ids.UUID]string) {
	t.Helper()
	hostID := ids.From[ids.UserKind](host)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", HostUserID: &hostID,
	})
	if err != nil {
		t.Fatalf("booking %q: %v", subject, err)
	}
	owner := integration.OwnerConn(t)
	for seat, role := range extra {
		query := `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, $3)`
		args := []any{row.Id, seat, role}
		if role == "importer" {
			query = `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`
			args = args[:2]
		}
		if _, err := owner.Exec(context.Background(), query, args...); err != nil {
			t.Fatalf("stamping %s on %q: %v", role, subject, err)
		}
	}
}

func myMeetingsToday(t *testing.T, e *integration.Env, reader ids.UUID) []string {
	t.Helper()
	lane := attentionMeetings{store: activities.NewStore(InstallationDB(e.Pool))}
	rows, _, err := lane.Today(e.As(reader, nil, integration.AdminPerms),
		attention.MeetingWindow{From: laneDay, Until: laneDay.Truncate(24 * time.Hour).Add(24 * time.Hour)}, 12, attention.TasksMine, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the meeting lane: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Subject)
	}
	return out
}

func TestOnlyTakingPartMakesAMeetingYours(t *testing.T) {
	e := integration.Setup(t)
	bookHostedMeeting(t, e, "Quarterly review", laneDay.Add(2*time.Hour), e.Rep1, map[ids.UUID]string{
		e.Rep2: "importer",
		e.Rep3: "cc",
	})
	bookHostedMeeting(t, e, "Workshop", laneDay.Add(4*time.Hour), e.Rep1, map[ids.UUID]string{
		e.Rep3: "attendee",
	})

	if got := myMeetingsToday(t, e, e.Rep1); fmt.Sprint(got) != "[Quarterly review Workshop]" {
		t.Errorf("the host's meetings = %v, want both", got)
	}
	if got := myMeetingsToday(t, e, e.Rep2); len(got) != 0 {
		t.Errorf("the seat that only imported a copy got %v, want none", got)
	}
	if got := myMeetingsToday(t, e, e.Rep3); fmt.Sprint(got) != "[Workshop]" {
		t.Errorf("the copied-then-attending seat got %v, want only the meeting they attend", got)
	}
}

func TestTheCompanyAndContactPagesNameTheSameRoom(t *testing.T) {
	e := integration.Setup(t)
	clock := func() time.Time { return laneDay }
	company := e.SeedCompany(t, "Halden Werke", nil)
	guest := e.SeedContact(t, "Greta Guest", nil)
	cc := e.SeedContact(t, "Carl Copied", nil)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, guest, company); err != nil {
		t.Fatal(err)
	}
	subject := "Rollout planning"
	at := laneDay.Add(48 * time.Hour)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: guest}},
	})
	if err != nil {
		t.Fatalf("booking: %v", err)
	}
	owner := integration.OwnerConn(t)
	for contact, role := range map[ids.UUID]string{guest: "attendee", cc: "cc"} {
		if _, err := owner.Exec(context.Background(),
			`INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, $3)`, row.Id, contact, role); err != nil {
			t.Fatal(err)
		}
	}

	companyPage, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(e.DB()), clock).Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assembling the company: %v", err)
	}
	contactPage, err := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), clock, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), clock).Assemble(e.Admin(), ids.From[ids.ContactKind](guest))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	if companyPage.NextMeeting == nil || contactPage.NextMeeting == nil || contactPage.NextMeeting.Participants == nil {
		t.Fatalf("next meeting: company %+v, contact %+v", companyPage.NextMeeting, contactPage.NextMeeting)
	}
	var onCompany, onContact []string
	for _, p := range companyPage.NextMeeting.Participants {
		onCompany = append(onCompany, p.DisplayName)
	}
	for _, p := range *contactPage.NextMeeting.Participants {
		onContact = append(onContact, p.FullName)
	}
	if fmt.Sprint(onCompany) != fmt.Sprint(onContact) || len(onCompany) == 0 {
		t.Errorf("the company page names %v and the contact page %v for one meeting", onCompany, onContact)
	}
}

// meetingRowsOf counts the reader's Worklist rows about one meeting, by source.
func meetingRowsOf(t *testing.T, e *integration.Env, reader ids.UUID, now time.Time, meeting ids.UUID) map[string]int {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(e.As(reader, nil, integration.AdminPerms), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the worklist: %v", err)
	}
	out := map[string]int{}
	for _, row := range page.Queue {
		if row.Id == meeting.String() || (row.SourceRecord != nil && ids.UUID(row.SourceRecord.Id) == meeting) {
			out[string(row.Source)]++
		}
	}
	return out
}

// A meeting is held only once somebody says so. Booked and ahead, it is a
// meeting to prepare. Past with no outcome, it is one question about its
// outcome. Saved as held, it is a follow-up.
func TestAMeetingIsHeldOnlyOnceItsOutcomeSaysSo(t *testing.T) {
	e := integration.Setup(t)
	now := laneDay.Add(3 * time.Hour)
	ownerID := ids.From[ids.UserKind](e.Rep1)
	conn := integration.OwnerConn(t)
	newCustomer := func(name, email string) ids.UUID {
		created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: name, OwnerID: &ownerID, Source: "manual",
			Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		account := ids.NewV7()
		if _, err := conn.Exec(context.Background(), `INSERT INTO company (id, display_name, owner_id, lifecycle, source, captured_by)
			VALUES ($1, $2, $3, 'customer', 'seed', 'system')`, account, name+" Account", e.Rep1); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(context.Background(), `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
			VALUES ('employment', $1, $2, 'seed', 'system')`, created.Id, account); err != nil {
			t.Fatal(err)
		}
		return ids.UUID(created.Id)
	}
	book := func(subject string, at time.Time, status string, customer ids.UUID) ids.UUID {
		rep := ids.From[ids.UserKind](e.Rep1)
		row, _, err := e.Activities.LogActivity(e.As(e.Rep1, nil, integration.AdminPerms), activities.LogActivityInput{
			Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", MeetingStatus: &status, HostUserID: &rep,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
		})
		if err != nil {
			t.Fatalf("booking %q: %v", subject, err)
		}
		return ids.UUID(row.Id)
	}
	ahead := book("Booked ahead", now.Add(2*time.Hour), "booked", newCustomer("Ahead Customer", "ahead@customer.test"))
	unreported := book("Past, no outcome", now.AddDate(0, 0, -3), "booked", newCustomer("Unreported Customer", "unreported@customer.test"))
	held := book("Past, held", now.AddDate(0, 0, -3), "held", newCustomer("Held Customer", "held@customer.test"))

	if got := meetingRowsOf(t, e, e.Rep1, now, ahead); got["meeting_follow_up"] != 0 || got["meeting_outcome"] != 0 || got["meeting"] != 1 {
		t.Errorf("a booked meeting ahead reads %v, want only the meeting to prepare", got)
	}
	if got := meetingRowsOf(t, e, e.Rep1, now, unreported); got["meeting_outcome"] != 1 || got["meeting_follow_up"] != 0 {
		t.Errorf("a past meeting with no outcome reads %v, want exactly one outcome question", got)
	}
	if got := meetingRowsOf(t, e, e.Rep1, now, held); got["meeting_follow_up"] != 1 || got["meeting_outcome"] != 0 {
		t.Errorf("a held meeting reads %v, want the follow-up path only", got)
	}
}

func seatEmail(t *testing.T, seat ids.UUID) string {
	t.Helper()
	var email string
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT email FROM app_user WHERE id = $1`, seat).Scan(&email); err != nil {
		t.Fatal(err)
	}
	return email
}

// captureCalendarEvent runs one provider event through the calendar mapper and
// the real sink, under the connector of the seat whose calendar holds it.
func captureCalendarEvent(t *testing.T, e *integration.Env, calendarOf ids.UUID, ev meetingmap.Event) {
	t.Helper()
	raw := []byte(`{"id":"` + ev.ID + `"}`)
	record := meetingmap.Classify(ev, seatEmail(t, calendarOf)).ToRecord(integration.CalendarSystem, raw)
	if _, err := calendarSink(e).Upsert(calendarOwnerCtx(e, calendarOf), record); err != nil {
		t.Fatalf("capturing %s: %v", ev.ID, err)
	}
}

// A calendar that holds a copy of a meeting makes nobody's work. The seat the
// provider lists as organizer or attendee takes part; the seat whose
// connector only imported it does not.
func TestACalendarCopyIsNotTheImportersMeetingButItsAttendeesAre(t *testing.T) {
	e := integration.Setup(t)
	organizer := seatEmail(t, e.Rep2)
	guest := meetingmap.Actor{Email: "buyer@customer.test"}
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-copied", Subject: "Colleague's review", StartsAt: laneDay.Add(2 * time.Hour), EndsAt: laneDay.Add(3 * time.Hour),
		Organizer: meetingmap.Actor{Email: organizer}, Attendees: []meetingmap.Actor{guest},
	})
	captureCalendarEvent(t, e, e.Rep3, meetingmap.Event{
		ID: "evt-invited", Subject: "Invited review", StartsAt: laneDay.Add(4 * time.Hour), EndsAt: laneDay.Add(5 * time.Hour),
		Organizer: guest, Attendees: []meetingmap.Actor{{Email: seatEmail(t, e.Rep3)}},
	})

	if got := myMeetingsToday(t, e, e.Rep1); len(got) != 0 {
		t.Errorf("the importing seat got %v, want none: its calendar only held a copy", got)
	}
	if got := myMeetingsToday(t, e, e.Rep2); fmt.Sprint(got) != "[Colleague's review]" {
		t.Errorf("the organizer got %v, want the meeting they organize", got)
	}
	if got := myMeetingsToday(t, e, e.Rep3); fmt.Sprint(got) != "[Invited review]" {
		t.Errorf("the invited seat got %v, want the meeting it attends", got)
	}
}

// A captured meeting stored before the owner's role was recorded fails closed.
// It is nobody's work until the replay of its stored original restores the
// owner's provider role.
func TestALegacyCalendarRowIsNobodysUntilItsOriginalIsReplayed(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	starts := laneDay.Add(36 * time.Hour)
	seedCapturedMeeting(t, e, "evt-legacy", starts, attendee{email: backfillOwner, response: "accepted", self: true})
	until := starts.Add(24 * time.Hour)
	lane := attentionMeetings{store: activities.NewStore(InstallationDB(e.Pool))}
	read := func() int {
		rows, _, err := lane.Today(e.As(e.AdminUser, nil, integration.AdminPerms), attention.MeetingWindow{From: starts.Add(-time.Hour), Until: until}, 12, attention.TasksMine, ids.UUID{})
		if err != nil {
			t.Fatal(err)
		}
		return len(rows)
	}
	if got := read(); got != 0 {
		t.Fatalf("a legacy row with only an importer host reads %d meetings, want none", got)
	}
	if _, err := repairMeetingAttendeesBatch(e.Admin(), e.Pool, meetingAttendeeRepairPerTick, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != 1 {
		t.Fatalf("after the replay the owner reads %d meetings, want the one they accepted", got)
	}
}

// Only a provider-attested mailbox binds a seat to a calendar event beyond its
// login address. A declared alias and a delivered-to sighting do not, and the
// set matches what capture.SeatProvedAddressTx accepts for the same seats.
func TestOnlyAProvenIdentityBindsASeatToACalendarEvent(t *testing.T) {
	e := integration.Setup(t)
	declared, delivered, proved := "deals-desk@ourco.test", "rep3-forward@ourco.test", "rep3-calendar@ourco.test"
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(e.As(e.Rep2, nil, integration.AdminPerms), "address", declared); err != nil {
		t.Fatal(err)
	}
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
		VALUES ($1, 'address', $2, 'delivered_to', 'system:alias-discovery')`, e.Rep3, delivered); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO capture_connection (provider, user_id, status, account_label)
		VALUES ('gcal', $1, 'connected', $2)`, e.Rep3, proved); err != nil {
		t.Fatal(err)
	}
	guest := meetingmap.Actor{Email: "buyer@customer.test"}
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-declared", Subject: "Desk review", StartsAt: laneDay.Add(2 * time.Hour), EndsAt: laneDay.Add(3 * time.Hour),
		Organizer: guest, Attendees: []meetingmap.Actor{{Email: declared}},
	})
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-delivered", Subject: "Forwarded invite", StartsAt: laneDay.Add(4 * time.Hour), EndsAt: laneDay.Add(5 * time.Hour),
		Organizer: guest, Attendees: []meetingmap.Actor{{Email: delivered}},
	})
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-proved", Subject: "Proved invite", StartsAt: laneDay.Add(6 * time.Hour), EndsAt: laneDay.Add(7 * time.Hour),
		Organizer: guest, Attendees: []meetingmap.Actor{{Email: proved}},
	})

	if got := myMeetingsToday(t, e, e.Rep2); len(got) != 0 {
		t.Errorf("a seat that only declared the alias got %v, want none", got)
	}
	if got := myMeetingsToday(t, e, e.Rep3); fmt.Sprint(got) != "[Proved invite]" {
		t.Errorf("the seat got %v, want only the event naming its provider-attested mailbox", got)
	}
	for _, tc := range []struct {
		seat    ids.UUID
		address string
		want    bool
	}{{e.Rep2, declared, false}, {e.Rep3, delivered, false}, {e.Rep3, proved, true}} {
		var got bool
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			var err error
			got, err = capture.SeatProvedAddressTx(e.Admin(), tx, tc.seat, tc.address)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("SeatProvedAddressTx(%s) = %v, but calendar attendance treats it as %v", tc.address, got, tc.want)
		}
	}
}

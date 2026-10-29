// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// databaseNow is the database's clock: the contact's last touch and the stored
// clock read now(), so the fixture dates itself from the same instant.
func databaseNow(t *testing.T, e *integration.Env) time.Time {
	t.Helper()
	var now time.Time
	if err := e.Pool.QueryRow(context.Background(), `SELECT now()`).Scan(&now); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	return now
}

// A call logged for today's date has happened from the start of that date. In
// Kiritimati, fourteen hours east of UTC, its stored noon is still ahead all
// day. The last touch, the account's last contact and the stored clock count it.
func TestADayEntryDatedTodayHasHappenedBeforeItsStoredNoon(t *testing.T) {
	e := integration.Setup(t)
	kiritimati := setInstallationZone(t, e, "Pacific/Kiritimati")
	now := databaseNow(t, e)
	today := occurredday.AnchorIn(now, kiritimati)
	if !today.After(now) {
		t.Fatalf("today's anchor %v is not ahead of %v, so this test proves nothing", today, now)
	}
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Atoll Freight", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the account: %v", err)
	}
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Tarawa Buyer"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	employee, employer := ids.From[ids.ContactKind](ids.UUID(contact.Id)), ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &employee, CompanyID: &employer,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	subject, outbound := "Called about the renewal", "outbound"
	call, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, Direction: &outbound, OccurredAt: &today, Source: "manual",
		OccurredPrecision: activities.PrecisionDay,
		Links:             []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}},
	})
	if err != nil {
		t.Fatalf("logging the call: %v", err)
	}

	companyID, contactID := ids.From[ids.CompanyKind](ids.UUID(company.Id)), ids.From[ids.ContactKind](ids.UUID(contact.Id))
	var account company360.LastTouch
	var touched contact360.LastTouch
	var clock *time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		accounts, err := company360.LastTouchFor(e.Admin(), tx, []ids.CompanyID{companyID}, now, company360.AssembleOptions{})
		if err != nil {
			return err
		}
		byContact, err := contact360.LastTouchFor(e.Admin(), tx, []ids.ContactID{contactID}, contact360.AssembleOptions{})
		if err != nil {
			return err
		}
		account, touched = accounts[companyID], byContact[contactID]
		return tx.QueryRow(context.Background(), `SELECT last_activity_at FROM contact WHERE id = $1`, contact.Id).Scan(&clock)
	}); err != nil {
		t.Fatalf("reading the last touch: %v", err)
	}
	if account.Contact == nil || account.Contact.ActivityID.UUID != ids.UUID(call.Id) || account.OutboundAt == nil {
		t.Errorf("the account's last touch = %+v, want today's call", account)
	}
	if touched.ContactAt == nil || !touched.ContactAt.Equal(today) || touched.ContactPrecision != activities.PrecisionDay {
		t.Errorf("the contact's last touch = %v (%q), want today's call at %v as a day", touched.ContactAt, touched.ContactPrecision, today)
	}
	if clock == nil || !clock.Equal(today) {
		t.Errorf("the contact's stored clock = %v, want today's call at %v", clock, today)
	}
}

// A day meeting's date ends at midnight in the zone, which is before its
// stored noon in Auckland. The morning after, it is no longer the next
// meeting, and a day meeting comes before an hour on the following date.
func TestTheNextMeetingFollowsTheDatesInTheZone(t *testing.T) {
	e := integration.Setup(t)
	auckland := setInstallationZone(t, e, "Pacific/Auckland")
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Harbour Buyer"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	book := func(subject string, at time.Time, precision string) {
		t.Helper()
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", OccurredPrecision: precision,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}},
		}); err != nil {
			t.Fatalf("booking %q: %v", subject, err)
		}
	}
	book("Tenth, all day", time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), activities.PrecisionDay)
	book("Eleventh, all day", time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC), activities.PrecisionDay)
	book("Twelfth, half past midnight", time.Date(2026, 3, 12, 0, 30, 0, 0, auckland), activities.PrecisionInstant)

	next, found, err := activities.NewStore(InstallationDB(e.Pool)).NextBookedMeeting(e.Admin(), activities.NextBookedMeetingInput{
		EntityType: "contact", EntityID: ids.UUID(contact.Id), After: time.Date(2026, 3, 11, 0, 30, 0, 0, auckland),
	})
	if err != nil || !found {
		t.Fatalf("reading the next meeting: found=%v err=%v", found, err)
	}
	if next.Subject != "Eleventh, all day" {
		t.Errorf("on the eleventh the next meeting is %q, want the eleventh's day meeting", next.Subject)
	}
}

// dayMeetingOwingAFollowUp is a held day meeting four dates back in zone that
// owes a follow-up, its date, its attendee and whether it still owes one.
func dayMeetingOwingAFollowUp(t *testing.T, e *integration.Env, zone *time.Location) (time.Time, ids.UUID, func() bool) {
	t.Helper()
	sent, met := seedFollowUps(t, e)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](sent), nil); err != nil {
		t.Fatalf("archiving the sent mail: %v", err)
	}
	now := databaseNow(t, e)
	dated, dayPrecision := occurredday.AnchorIn(now.AddDate(0, 0, -4), zone), activities.PrecisionDay
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](met), activities.UpdateActivityInput{
		OccurredAt: &dated, OccurredPrecision: &dayPrecision,
	}); err != nil {
		t.Fatalf("making the meeting a day meeting: %v", err)
	}
	seam := attentionAwaiting{store: activities.NewStore(e.DB())}
	owed := func() bool {
		t.Helper()
		rows, _, err := seam.MeetingFollowUps(e.Admin(), now)
		if err != nil {
			t.Fatalf("reading meeting follow-ups: %v", err)
		}
		return awaited(rows, met) != nil
	}
	if !owed() {
		t.Fatal("the day meeting owes no follow-up before anything was sent, so this test proves nothing")
	}
	var attendee ids.UUID
	if err := e.Pool.QueryRow(context.Background(), `SELECT contact_id FROM activity_link WHERE activity_id = $1`, met).Scan(&attendee); err != nil {
		t.Fatalf("reading the meeting's contact: %v", err)
	}
	return dated, attendee, owed
}

// A follow-up sent on the morning of a day meeting's date answers it. The
// date ends at midnight, so measuring from the end missed that morning.
func TestAFollowUpOnADayMeetingsDateAnswersIt(t *testing.T) {
	e := integration.Setup(t)
	auckland := setInstallationZone(t, e, "Pacific/Auckland")
	dated, attendee, owed := dayMeetingOwingAFollowUp(t, e, auckland)

	year, month, day := dated.Date()
	morning := time.Date(year, month, day, 8, 0, 0, 0, auckland)
	logMail(t, e, attendee, "outbound", morning)
	if owed() {
		t.Errorf("a follow-up at %v on the meeting's date did not answer it", morning)
	}
}

// A day call is placed by its own date. Dated the day before a day meeting, it
// is not a follow-up, though in Auckland its stored noon falls after the
// meeting's date begins. Dated the meeting's own date, it is one.
func TestADayCallDatedBeforeADayMeetingIsNotItsFollowUp(t *testing.T) {
	e := integration.Setup(t)
	auckland := setInstallationZone(t, e, "Pacific/Auckland")
	dated, attendee, owed := dayMeetingOwingAFollowUp(t, e, auckland)
	call := func(date time.Time) {
		t.Helper()
		subject, outbound := "Rang them", "outbound"
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "call", Subject: &subject, Direction: &outbound, OccurredAt: &date, Source: "manual",
			OccurredPrecision: activities.PrecisionDay,
			Links:             []activities.ActivityLinkInput{{EntityType: "contact", EntityID: attendee}},
		}); err != nil {
			t.Fatalf("logging the day call: %v", err)
		}
	}

	call(dated.AddDate(0, 0, -1))
	if !owed() {
		t.Error("a day call dated the day before the meeting answered it")
	}
	call(dated)
	if owed() {
		t.Error("a day call dated the meeting's own date did not answer it")
	}
}

// On a busy day the meetings lane still opens with the day meeting, and the
// day meeting takes no timed meeting's place: each is read to its own limit.
func TestABusyDayKeepsTheDayMeetingOnTheLane(t *testing.T) {
	e := integration.Setup(t)
	for i := range 13 {
		bookMeeting(t, e, fmt.Sprintf("Slot %02d", i), laneDay.Add(time.Duration(10+i*20)*time.Minute), "")
	}
	subject, date := "Trade fair", time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &date, Source: "manual", OccurredPrecision: activities.PrecisionDay,
	}); err != nil {
		t.Fatalf("logging the day meeting: %v", err)
	}
	lane := attentionMeetings{store: activities.NewStore(InstallationDB(e.Pool))}
	window := attention.MeetingWindow{
		From: laneDay, Until: laneDay.Truncate(24 * time.Hour).Add(24 * time.Hour),
		DaysFrom: occurredday.Anchor(laneDay), DaysUntil: occurredday.Anchor(laneDay).AddDate(0, 0, 1),
	}
	rows, _, err := lane.Today(e.Admin(), window, 12, attention.TasksVisible, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the meeting lane: %v", err)
	}
	if len(rows) != 13 || rows[0].Subject != subject || rows[1].Subject != "Slot 00" || rows[12].Subject != "Slot 11" {
		subjects := make([]string, 0, len(rows))
		for _, row := range rows {
			subjects = append(subjects, row.Subject)
		}
		t.Errorf("the lane holds %v, want the trade fair and then the twelve soonest slots", subjects)
	}
}

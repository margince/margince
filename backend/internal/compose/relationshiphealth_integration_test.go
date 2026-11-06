// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A company's Relationship rating counts a past meeting only once someone
// recorded it held. The rows come from the real writers. A calendar meeting
// comes in through the Sink as a live pull lands it, with no status.

import (
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// quietAccount is a company with one employed contact whose last word to us
// was 77 days ago: at risk by the message alone.
type quietAccount struct {
	company ids.CompanyID
	contact ids.UUID
	now     time.Time
}

func seedQuietAccount(t *testing.T, e *integration.Env) quietAccount {
	t.Helper()
	acct := quietAccount{
		company: ids.From[ids.CompanyKind](e.SeedCompany(t, "Quiet GmbH", nil)),
		contact: e.SeedContact(t, "Quinn Quiet", nil),
		now:     time.Now().UTC(),
	}
	contactID, primary := ids.From[ids.ContactKind](acct.contact), true
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &acct.company, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	subject, inbound, at := "Re: renewal", "inbound", acct.now.AddDate(0, 0, -77)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
	}); err != nil {
		t.Fatalf("logging their last reply: %v", err)
	}
	return acct
}

// captureMeetingWith lands one calendar meeting with the contact through the
// real Sink, under its own event id.
func captureMeetingWith(t *testing.T, e *integration.Env, contact ids.UUID, eventID string, at time.Time) {
	t.Helper()
	if _, err := calendarSink(e).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: eventID},
		Fields:     capture.ActivityFields{Kind: "meeting", Subject: "Account review", OccurredAt: at},
		Links:      []datasource.EntityRef{{Type: "contact", ID: contact}},
		Source:     calendarSystem + ":" + eventID,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        []byte(`{"id":"` + eventID + `"}`),
	}); err != nil {
		t.Fatalf("capturing the meeting: %v", err)
	}
}

// logAccountMeeting logs a meeting with the contact through the activities writer.
func logAccountMeeting(t *testing.T, e *integration.Env, acct quietAccount, subject, status string, at time.Time) {
	t.Helper()
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &status, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
	}); err != nil {
		t.Fatalf("logging the %s meeting: %v", status, err)
	}
}

func pageOf(t *testing.T, e *integration.Env, acct quietAccount) crmcontracts.Company360 {
	t.Helper()
	page, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), func() time.Time { return acct.now }).
		Assemble(e.Admin(), acct.company)
	if err != nil {
		t.Fatalf("assembling the account page: %v", err)
	}
	return page
}

func relationshipOf(t *testing.T, e *integration.Env, acct quietAccount) *crmcontracts.HealthDimension {
	t.Helper()
	page := pageOf(t, e, acct)
	if page.Health == nil || page.Health.Relationship == nil {
		t.Fatalf("the account page carries no relationship rating: %+v", page.Health)
	}
	return page.Health.Relationship
}

func TestAMeetingKeepsTheRelationshipOffAtRisk(t *testing.T) {
	cases := []struct {
		name      string
		meetingAt func(now time.Time) time.Time
		want      crmcontracts.HealthDimensionReasonCode
	}{
		{
			"booked for the day after tomorrow", func(now time.Time) time.Time { return now.AddDate(0, 0, 2) },
			crmcontracts.HealthDimensionReasonCodeMeetingBooked,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			acct := seedQuietAccount(t, e)
			captureMeetingWith(t, e, acct.contact, "evt-account-review", tc.meetingAt(acct.now))

			got := relationshipOf(t, e, acct)
			if got.Rating == crmcontracts.HealthDimensionRatingAtRisk {
				t.Fatalf("relationship = at risk (%q) beside a meeting with them", got.Reason)
			}
			if got.ReasonCode == nil || *got.ReasonCode != tc.want {
				t.Fatalf("reason code = %v (%q), want %q", got.ReasonCode, got.Reason, tc.want)
			}
		})
	}
}

// A calendar meeting whose start passed and that nobody reported is awaiting
// its outcome. It is not the last time we met them, so the account stays quiet
// from their last message.
func TestAPastMeetingNobodyReportedIsNotLastMet(t *testing.T) {
	e := integration.Setup(t)
	acct := seedQuietAccount(t, e)
	captureMeetingWith(t, e, acct.contact, "evt-account-review", acct.now.AddDate(0, 0, -21))

	page := pageOf(t, e, acct)
	got := page.Health.Relationship
	if got == nil || got.ReasonCode == nil || *got.ReasonCode != crmcontracts.HealthDimensionReasonCodeQuiet {
		t.Fatalf("relationship = %+v, want quiet from their message 77 days ago", got)
	}
	if got.ReasonParams == nil || got.ReasonParams.Days == nil || *got.ReasonParams.Days != 77 {
		t.Errorf("quiet reason params = %+v, want 77 days: the unreported meeting is not the last contact", got.ReasonParams)
	}
	if page.Health.LastMeetingAt != nil {
		t.Errorf("last meeting held = %v, want none: nobody reported the meeting held", page.Health.LastMeetingAt)
	}
	if page.LastContact == nil || page.LastContact.Kind == crmcontracts.Company360LastContactKindMeeting {
		t.Errorf("last contact = %+v, want their message: the unreported meeting is no contact", page.LastContact)
	}
	if page.Contacts == nil || len(page.Contacts.Data) != 1 {
		t.Fatalf("contacts = %+v, want the one employed contact", page.Contacts)
	}
	if last := page.Contacts.Data[0].Strength.LastInteraction; last == nil || acct.now.Sub(*last) < 76*24*time.Hour {
		t.Errorf("the contact's last interaction = %v, want their message 77 days ago", last)
	}
}

// The other direction: a meeting held before the threshold is not current
// contact, and a canceled one is not contact at all.
func TestAnOldOrCanceledMeetingLeavesTheRelationshipAtRisk(t *testing.T) {
	e := integration.Setup(t)
	acct := seedQuietAccount(t, e)
	logAccountMeeting(t, e, acct, "Old review", "held", acct.now.AddDate(0, 0, -40))
	subject, canceled, at := "Called-off check-in", "canceled", acct.now.AddDate(0, 0, -3)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &canceled, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
	}); err != nil {
		t.Fatalf("logging the canceled meeting: %v", err)
	}

	got := relationshipOf(t, e, acct)
	if got.Rating != crmcontracts.HealthDimensionRatingAtRisk {
		t.Fatalf("relationship = %q (%q), want at risk — the only recent meeting was called off", got.Rating, got.Reason)
	}
	if got.ReasonCode == nil || *got.ReasonCode != crmcontracts.HealthDimensionReasonCodeQuiet {
		t.Fatalf("reason code = %v (%q), want quiet, measured from the meeting 40 days ago", got.ReasonCode, got.Reason)
	}

	strength, err := e.Contacts.ContactStrength(e.Admin(), ids.From[ids.ContactKind](acct.contact), acct.now)
	if err != nil {
		t.Fatalf("reading the contact's strength: %v", err)
	}
	age := time.Duration(0)
	if strength.LastInteraction != nil {
		age = acct.now.Sub(*strength.LastInteraction)
	}
	if age < 39*24*time.Hour || age > 41*24*time.Hour {
		t.Fatalf("the contact's last interaction = %v, want the meeting 40 days ago — a canceled meeting is not contact",
			strength.LastInteraction)
	}
}

// Status and clock decide together. Only a meeting recorded held whose start
// passed is contact we had. A past booking is awaiting its outcome. A canceled
// one is not contact. A row marked held but dated ahead is no booking.
func TestAMeetingCountsOnlyAsWhatItsStatusAndTimeSay(t *testing.T) {
	cases := []struct {
		name   string
		status string
		days   int // negative is ahead
		want   crmcontracts.HealthDimensionReasonCode
	}{
		{"booked, start passed, never marked held", "booked", 5, crmcontracts.HealthDimensionReasonCodeQuiet},
		{"canceled, start passed", "canceled", 5, crmcontracts.HealthDimensionReasonCodeQuiet},
		{"marked held but dated ahead", "held", -2, crmcontracts.HealthDimensionReasonCodeQuiet},
		{"marked held last week", "held", 7, crmcontracts.HealthDimensionReasonCodeLastMet},
		{"booked ahead", "booked", -2, crmcontracts.HealthDimensionReasonCodeMeetingBooked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			acct := seedQuietAccount(t, e)
			subject, at := "Status check", acct.now.AddDate(0, 0, -tc.days)
			logged := at
			if tc.status == "held" && tc.days < 0 {
				logged = acct.now.AddDate(0, 0, -1)
			}
			meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "meeting", Subject: &subject, MeetingStatus: &tc.status, OccurredAt: &logged, Source: "manual",
				Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
			})
			if err != nil {
				t.Fatalf("logging the meeting: %v", err)
			}
			if !logged.Equal(at) {
				// Writers refuse held before the start; older rows can carry it.
				e.WsExec(t, `UPDATE activity SET occurred_at = $1 WHERE id = $2`, at, ids.UUID(meeting.Id))
			}

			got := relationshipOf(t, e, acct)
			if got.ReasonCode == nil || *got.ReasonCode != tc.want {
				t.Fatalf("reason code = %v (%q), want %q", got.ReasonCode, got.Reason, tc.want)
			}
		})
	}
}

// A reader who may see the account's contacts but not its activity gets no
// meeting dates, and a rating that cannot reveal a meeting through its reason.
func TestHealthWithoutTheActivityGrantNamesNoMeeting(t *testing.T) {
	e := integration.Setup(t)
	acct := seedQuietAccount(t, e)
	captureMeetingWith(t, e, acct.contact, "evt-hidden-review", acct.now.AddDate(0, 0, -21))
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, "activity")

	page, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), func() time.Time { return acct.now }).
		Assemble(e.As(e.AdminUser, nil, perms), acct.company)
	if err != nil {
		t.Fatalf("assembling the account page without the activity grant: %v", err)
	}
	if page.Health == nil || page.Health.Relationship == nil {
		t.Fatalf("health = %+v, want a relationship rating from what the reader may see", page.Health)
	}
	if page.Health.LastMeetingAt != nil {
		t.Errorf("last meeting = %v, want none for a reader who may not read activity", page.Health.LastMeetingAt)
	}
	code := page.Health.Relationship.ReasonCode
	if code != nil && (*code == crmcontracts.HealthDimensionReasonCodeLastMet ||
		*code == crmcontracts.HealthDimensionReasonCodeMeetingBooked) {
		t.Errorf("reason = %q (%q), which tells a reader without the activity grant that a meeting exists",
			*code, page.Health.Relationship.Reason)
	}
}

// A message dated ahead of the read, scheduled or mis-stamped, is not their
// last word: the rating and the strip both read their newest real message.
func TestAFutureDatedMessageHidesNotTheirLastRealOne(t *testing.T) {
	e := integration.Setup(t)
	acct := seedQuietAccount(t, e)
	inbound := "inbound"
	logInbound := func(subject string, at time.Time) {
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
		}); err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
	}
	lastReal := acct.now.AddDate(0, 0, -10)
	logInbound("Re: next steps", lastReal)
	logInbound("Scheduled: quarterly note", acct.now.AddDate(0, 0, 3))

	page := pageOf(t, e, acct)
	if page.LastInboundAt == nil || page.LastInboundAt.Sub(lastReal).Abs() > time.Second {
		t.Fatalf("last inbound = %v, want the real message at %v", page.LastInboundAt, lastReal)
	}
	if strip := page.StateStrip; strip == nil || strip.Engagement == nil || strip.Engagement.LastInboundAt == nil ||
		strip.Engagement.LastInboundAt.Sub(lastReal).Abs() > time.Second {
		t.Fatalf("the strip's last inbound = %+v, want the real message at %v", page.StateStrip, lastReal)
	}
	if page.Health == nil || page.Health.DaysSinceLastInbound == nil || *page.Health.DaysSinceLastInbound != 10 {
		t.Fatalf("days since last inbound = %+v, want 10", page.Health)
	}
	got := page.Health.Relationship
	if got == nil || got.Rating == crmcontracts.HealthDimensionRatingAtRisk {
		t.Fatalf("relationship = %+v, want in touch through the message 10 days ago", got)
	}
}

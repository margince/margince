// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A company's Relationship rating counts meetings, on the same rule the
// contact strength fold counts them, against rows the real writers produced:
// the meeting comes in through the calendar Sink exactly as a live pull lands
// it, with no direction and no status.

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

func relationshipOf(t *testing.T, e *integration.Env, acct quietAccount) *crmcontracts.HealthDimension {
	t.Helper()
	page, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), func() time.Time { return acct.now }).
		Assemble(e.Admin(), acct.company)
	if err != nil {
		t.Fatalf("assembling the account page: %v", err)
	}
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
			"held three weeks ago", func(now time.Time) time.Time { return now.AddDate(0, 0, -21) },
			crmcontracts.HealthDimensionReasonCodeLastMet,
		},
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

// The other direction: a meeting held before the threshold is not current
// contact, and a canceled one is not contact at all — for the company rating
// and for the contact strength fold alike.
func TestAnOldOrCanceledMeetingLeavesTheRelationshipAtRisk(t *testing.T) {
	e := integration.Setup(t)
	acct := seedQuietAccount(t, e)
	captureMeetingWith(t, e, acct.contact, "evt-old-review", acct.now.AddDate(0, 0, -40))
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

// Status and clock decide together. A booking whose start passed is contact
// we had, marked held or not; a canceled one is not; and a row marked held
// but dated ahead is not a booking to keep the account off at risk.
func TestAMeetingCountsOnlyAsWhatItsStatusAndTimeSay(t *testing.T) {
	cases := []struct {
		name   string
		status string
		days   int // negative is ahead
		want   crmcontracts.HealthDimensionReasonCode
	}{
		{"booked, start passed, never marked held", "booked", 5, crmcontracts.HealthDimensionReasonCodeLastMet},
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
			if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "meeting", Subject: &subject, MeetingStatus: &tc.status, OccurredAt: &at, Source: "manual",
				Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
			}); err != nil {
				t.Fatalf("logging the meeting: %v", err)
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

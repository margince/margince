// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A canceled or no-show meeting does not keep last_activity_at on a deal, a
// contact or a company, or the recency evidence deal health cites, on itself.
// A booked meeting still moves the clock, wherever it is dated. A calendar
// meeting lands and is called off through the real Sink; a logged one through
// LogActivity and UpdateActivity.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// clockFixture is one workspace under test and the moment it treats as now.
type clockFixture struct {
	e   *integration.Env
	now time.Time
}

func newClockFixture(t *testing.T) clockFixture {
	t.Helper()
	return clockFixture{e: integration.Setup(t), now: time.Now().UTC().Truncate(time.Second)}
}

func (f clockFixture) newDeal(t *testing.T, name string, company *ids.CompanyID) ids.UUID {
	t.Helper()
	pipeline, open, _ := integration.DealFixture(t, f.e)
	deal, err := f.e.Deals.CreateDeal(f.e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: open, CompanyID: company, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.UUID(deal.Id)
}

// logMeeting logs a meeting by hand with the given status, filed on one record.
func (f clockFixture) logMeeting(t *testing.T, on activities.ActivityLinkInput, status string, at time.Time) ids.ActivityID {
	t.Helper()
	subject := "Clock meeting"
	logged, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &status, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{on},
	})
	if err != nil {
		t.Fatalf("logging the %s meeting: %v", status, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

func (f clockFixture) setStatus(t *testing.T, meeting ids.ActivityID, status string) {
	t.Helper()
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), meeting, activities.UpdateActivityInput{MeetingStatus: &status}); err != nil {
		t.Fatalf("marking the meeting %s: %v", status, err)
	}
}

func assertClock(t *testing.T, got *time.Time, want time.Time, why string) {
	t.Helper()
	if got == nil || !got.Equal(want) {
		t.Fatalf("last_activity_at = %v, want %v: %s", got, want, why)
	}
}

func (f clockFixture) dealClock(t *testing.T, deal ids.UUID) *time.Time {
	t.Helper()
	d, err := f.e.Deals.GetDeal(f.e.Admin(), ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	return d.LastActivityAt
}

func onRecord(entityType string, id ids.UUID) activities.ActivityLinkInput {
	return activities.ActivityLinkInput{EntityType: entityType, EntityID: id}
}

func TestACanceledCalendarMeetingLeavesTheDealClock(t *testing.T) {
	f := newClockFixture(t)
	deal := f.newDeal(t, "Clock calendar deal", nil)
	heldAt := f.now.AddDate(0, 0, -10)
	held := f.logMeeting(t, onRecord("deal", deal), "held", heldAt)
	review := integration.CalendarMeeting{
		Event: "evt-clock-review", At: f.now.AddDate(0, 0, -2),
		Links: []datasource.EntityRef{{Type: "deal", ID: deal}},
	}
	review.Capture(t, f.e)
	assertClock(t, f.dealClock(t, deal), review.At, "a captured meeting nobody called off is the newest activity")

	review.Cancel(t, f.e)
	assertClock(t, f.dealClock(t, deal), heldAt, "a canceled meeting must not keep the deal looking recently active")
	h, err := f.e.Deals.DealHealth(f.e.Admin(), ids.From[ids.DealKind](deal), f.now)
	if err != nil {
		t.Fatal(err)
	}
	if latest := h.Evidence.MostRecentActivityID; latest == nil || *latest != held.UUID {
		t.Errorf("deal health cites activity %v as the latest, want the held meeting %v", latest, held.UUID)
	}
}

func TestANoShowLoggedMeetingLeavesTheDealClock(t *testing.T) {
	f := newClockFixture(t)
	deal := f.newDeal(t, "Clock no-show deal", nil)
	heldAt, missedAt := f.now.AddDate(0, 0, -10), f.now.AddDate(0, 0, -2)
	f.logMeeting(t, onRecord("deal", deal), "held", heldAt)
	missed := f.logMeeting(t, onRecord("deal", deal), "booked", missedAt)
	assertClock(t, f.dealClock(t, deal), missedAt, "a booked meeting is the newest activity")

	f.setStatus(t, missed, "no_show")
	assertClock(t, f.dealClock(t, deal), heldAt, "a no-show must not keep the deal looking recently active")

	f.setStatus(t, missed, "held")
	assertClock(t, f.dealClock(t, deal), missedAt, "a meeting marked held after all counts again")
}

func TestABookedFutureMeetingStillMovesTheDealClock(t *testing.T) {
	f := newClockFixture(t)
	deal := f.newDeal(t, "Clock future deal", nil)
	f.logMeeting(t, onRecord("deal", deal), "held", f.now.AddDate(0, 0, -10))
	ahead := f.now.AddDate(0, 0, 14)
	f.logMeeting(t, onRecord("deal", deal), "booked", ahead)
	assertClock(t, f.dealClock(t, deal), ahead, "a booked meeting counts as contact wherever it is dated")
}

func TestACanceledCalendarMeetingLeavesTheContactClock(t *testing.T) {
	f := newClockFixture(t)
	contact := f.e.SeedContact(t, "Clock Contact", nil)
	clock := func() *time.Time {
		c, err := f.e.Contacts.GetContact(f.e.Admin(), ids.From[ids.ContactKind](contact), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		return c.LastActivityAt
	}
	heldAt := f.now.AddDate(0, 0, -10)
	f.logMeeting(t, onRecord("contact", contact), "held", heldAt)
	intro := integration.CalendarMeeting{Event: "evt-clock-intro", At: f.now.AddDate(0, 0, -2), Links: contactLink(contact)}
	intro.Capture(t, f.e)
	assertClock(t, clock(), intro.At, "a captured meeting nobody called off is the newest activity")

	intro.Cancel(t, f.e)
	assertClock(t, clock(), heldAt, "a canceled meeting must not keep the contact looking recently contacted")
}

// The company reads its deals' meetings too, so the called-off meeting is
// filed on the deal and only the company's deal branch can see it.
func TestANoShowOnItsDealLeavesTheCompanyClock(t *testing.T) {
	f := newClockFixture(t)
	company := f.e.SeedCompany(t, "Clock Co", nil)
	companyID := ids.From[ids.CompanyKind](company)
	deal := f.newDeal(t, "Clock company deal", &companyID)
	clock := func() *time.Time {
		c, err := f.e.Contacts.GetCompany(f.e.Admin(), companyID, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		return c.LastActivityAt
	}
	notedAt, missedAt := f.now.AddDate(0, 0, -10), f.now.AddDate(0, 0, -2)
	subject := "Account note"
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, OccurredAt: &notedAt, Source: "manual",
		Links: []activities.ActivityLinkInput{onRecord("company", company)},
	}); err != nil {
		t.Fatalf("logging the note: %v", err)
	}
	missed := f.logMeeting(t, onRecord("deal", deal), "booked", missedAt)
	assertClock(t, clock(), missedAt, "a booked meeting on its deal is the company's newest activity")

	f.setStatus(t, missed, "no_show")
	assertClock(t, clock(), notedAt, "a no-show on its deal must not keep the company looking recently active")
}

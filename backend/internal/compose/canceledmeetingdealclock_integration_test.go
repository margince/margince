// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A canceled or no-show meeting does not keep deal.last_activity_at, or the
// recency evidence deal health cites, on itself. A booked meeting still moves
// the clock, wherever it is dated. The calendar meeting lands and is called off
// through the real Sink; the logged one through LogActivity and UpdateActivity.

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

// clockDeal is one deal under test and the moment the test treats as now.
type clockDeal struct {
	e   *integration.Env
	id  ids.UUID
	now time.Time
}

func newClockDeal(t *testing.T, name string) clockDeal {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	return clockDeal{e: e, id: e.SeedDeal(t, name, pipeline, open, nil), now: time.Now().UTC().Truncate(time.Second)}
}

// logMeeting logs a meeting on the deal by hand with the given status.
func (d clockDeal) logMeeting(t *testing.T, status string, at time.Time) ids.ActivityID {
	t.Helper()
	subject := "Deal clock meeting"
	logged, _, err := d.e.Activities.LogActivity(d.e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &status, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: d.id}},
	})
	if err != nil {
		t.Fatalf("logging the %s meeting: %v", status, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

func (d clockDeal) setStatus(t *testing.T, meeting ids.ActivityID, status string) {
	t.Helper()
	if _, err := d.e.Activities.UpdateActivity(d.e.Admin(), meeting, activities.UpdateActivityInput{MeetingStatus: &status}); err != nil {
		t.Fatalf("marking the meeting %s: %v", status, err)
	}
}

func (d clockDeal) assertClock(t *testing.T, want time.Time, why string) {
	t.Helper()
	deal, err := d.e.Deals.GetDeal(d.e.Admin(), ids.From[ids.DealKind](d.id), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if deal.LastActivityAt == nil || !deal.LastActivityAt.Equal(want) {
		t.Fatalf("deal.last_activity_at = %v, want %v: %s", deal.LastActivityAt, want, why)
	}
}

func (d clockDeal) health(t *testing.T) deals.DealHealthEvidence {
	t.Helper()
	h, err := d.e.Deals.DealHealth(d.e.Admin(), ids.From[ids.DealKind](d.id), d.now)
	if err != nil {
		t.Fatal(err)
	}
	return h.Evidence
}

func TestACanceledCalendarMeetingLeavesTheDealClock(t *testing.T) {
	d := newClockDeal(t, "Clock calendar deal")
	heldAt := d.now.AddDate(0, 0, -10)
	held := d.logMeeting(t, "held", heldAt)
	review := integration.CalendarMeeting{
		Event: "evt-clock-review", At: d.now.AddDate(0, 0, -2),
		Links: []datasource.EntityRef{{Type: "deal", ID: d.id}},
	}
	review.Capture(t, d.e)
	d.assertClock(t, review.At, "a captured meeting nobody called off is the newest activity")

	review.Cancel(t, d.e)
	d.assertClock(t, heldAt, "a canceled meeting must not keep the deal looking recently active")
	ev := d.health(t)
	if ev.MostRecentActivityID == nil || *ev.MostRecentActivityID != held.UUID {
		t.Errorf("deal health cites activity %v as the latest, want the held meeting %v", ev.MostRecentActivityID, held.UUID)
	}
}

func TestANoShowLoggedMeetingLeavesTheDealClock(t *testing.T) {
	d := newClockDeal(t, "Clock no-show deal")
	heldAt, missedAt := d.now.AddDate(0, 0, -10), d.now.AddDate(0, 0, -2)
	d.logMeeting(t, "held", heldAt)
	missed := d.logMeeting(t, "booked", missedAt)
	d.assertClock(t, missedAt, "a booked meeting is the newest activity")

	d.setStatus(t, missed, "no_show")
	d.assertClock(t, heldAt, "a no-show must not keep the deal looking recently active")

	d.setStatus(t, missed, "held")
	d.assertClock(t, missedAt, "a meeting marked held after all counts again")
}

func TestABookedFutureMeetingStillMovesTheDealClock(t *testing.T) {
	d := newClockDeal(t, "Clock future deal")
	d.logMeeting(t, "held", d.now.AddDate(0, 0, -10))
	ahead := d.now.AddDate(0, 0, 14)
	d.logMeeting(t, "booked", ahead)
	d.assertClock(t, ahead, "a booked meeting counts as contact wherever it is dated")
}

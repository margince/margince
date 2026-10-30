// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A booked meeting ahead keeps a deal off stalled and gives deal health full
// recency, though the deal's clock no longer counts it. A meeting called off,
// filed elsewhere or held to one colleague does neither, and nor does a
// future-dated email. The readers that pick a last touch on their own ignore
// what has not happened yet. Activities land through LogActivity.

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// idleDeal is an open deal opened 90 days ago whose last activity is a mail
// mailedDaysAgo days old.
func (f clockFixture) idleDeal(t *testing.T, name string, mailedDaysAgo int) ids.UUID {
	t.Helper()
	deal := f.newDeal(t, name, nil)
	f.e.WsExec(t, `UPDATE deal SET created_at = now() - interval '90 days' WHERE id = $1`, deal)
	f.logOn(t, "email", onRecord("deal", deal), f.now.AddDate(0, 0, -mailedDaysAgo))
	return deal
}

// logOn logs one activity of kind on one record; a meeting is booked.
func (f clockFixture) logOn(t *testing.T, kind string, on activities.ActivityLinkInput, at time.Time) ids.ActivityID {
	t.Helper()
	if kind == "meeting" {
		return f.logMeeting(t, on, "booked", at)
	}
	subject, direction := "Ahead "+kind, "outbound"
	logged, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: kind, Subject: &subject, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{on},
	})
	if err != nil {
		t.Fatalf("logging the %s: %v", kind, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

// stalled answers the flag the single read stamps and whether the
// stalled=true list filter returns the deal.
func (f clockFixture) stalled(t *testing.T, deal ids.UUID) (single, listed bool) {
	t.Helper()
	d, err := f.e.Deals.GetDeal(f.e.Admin(), ids.From[ids.DealKind](deal), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := f.e.Deals.ListDeals(f.e.Admin(), deals.ListDealsInput{Stalled: new(true), Limit: new(100)})
	if err != nil {
		t.Fatal(err)
	}
	listed = slices.ContainsFunc(page, func(row crmcontracts.Deal) bool { return ids.UUID(row.Id) == deal })
	return d.Stalled != nil && *d.Stalled, listed
}

func (f clockFixture) health(t *testing.T, deal ids.UUID, now time.Time) deals.DealHealth {
	t.Helper()
	h, err := f.e.Deals.DealHealth(f.e.Admin(), ids.From[ids.DealKind](deal), now)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestABookedMeetingAheadKeepsADealOffStalled(t *testing.T) {
	f := newClockFixture(t)
	booked := f.idleDeal(t, "Booked ahead", 70)
	meeting := f.logOn(t, "meeting", onRecord("deal", booked), f.now.AddDate(0, 0, 7))
	if single, listed := f.stalled(t, booked); single || listed {
		t.Errorf("an idle deal with a booked meeting ahead: single read stalled %v, stalled list holds it %v; want neither", single, listed)
	}
	f.setStatus(t, meeting, "canceled")
	if single, listed := f.stalled(t, booked); !single || !listed {
		t.Errorf("the meeting was canceled: single read stalled %v, stalled list holds it %v; want both", single, listed)
	}

	mailed := f.idleDeal(t, "Future mail", 70)
	f.logOn(t, "email", onRecord("deal", mailed), f.now.AddDate(0, 0, 7))
	if single, listed := f.stalled(t, mailed); !single || !listed {
		t.Errorf("a future-dated email is no booked meeting: single read stalled %v, stalled list holds it %v; want both", single, listed)
	}

	private := f.idleDeal(t, "Private meeting", 70)
	held := f.logOn(t, "meeting", onRecord("deal", private), f.now.AddDate(0, 0, 7))
	if _, err := f.e.Activities.SetAudience(f.e.Admin(), held, activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: f.e.AdminUser}}}); err != nil {
		t.Fatalf("holding the meeting to one colleague: %v", err)
	}
	if single, listed := f.stalled(t, private); !single || !listed {
		t.Errorf("a meeting only one colleague may see changes no shared flag: single read stalled %v, "+
			"stalled list holds it %v; want both", single, listed)
	}
}

func TestABookedMeetingAheadGivesDealHealthFullRecency(t *testing.T) {
	f := newClockFixture(t)
	cases := []struct {
		name    string
		ahead   func(deal ids.UUID)
		recency float64
	}{
		{"booked a week ahead", func(deal ids.UUID) { f.logOn(t, "meeting", onRecord("deal", deal), f.now.AddDate(0, 0, 7)) }, 1.0},
		{"booked a year ahead", func(deal ids.UUID) { f.logOn(t, "meeting", onRecord("deal", deal), f.now.AddDate(0, 0, 366)) }, 1.0},
		{"canceled", func(deal ids.UUID) {
			f.setStatus(t, f.logOn(t, "meeting", onRecord("deal", deal), f.now.AddDate(0, 0, 7)), "canceled")
		}, 0.4},
		{"a future-dated email", func(deal ids.UUID) { f.logOn(t, "email", onRecord("deal", deal), f.now.AddDate(0, 0, 7)) }, 0.4},
		{"booked on another deal", func(ids.UUID) {
			f.logOn(t, "meeting", onRecord("deal", f.newDeal(t, "Other deal", nil)), f.now.AddDate(0, 0, 7))
		}, 0.4},
		{"held to one colleague", func(deal ids.UUID) {
			held := f.logOn(t, "meeting", onRecord("deal", deal), f.now.AddDate(0, 0, 7))
			if _, err := f.e.Activities.SetAudience(f.e.Admin(), held, activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: f.e.AdminUser}}}); err != nil {
				t.Fatalf("holding the meeting to one colleague: %v", err)
			}
		}, 0.4},
	}
	for _, c := range cases {
		deal := f.newDeal(t, "Health "+c.name, nil)
		mail := f.logOn(t, "email", onRecord("deal", deal), f.now.AddDate(0, 0, -20))
		c.ahead(deal)
		h := f.health(t, deal, f.now)
		if h.Factors.ActivityRecency != c.recency {
			t.Errorf("%s: recency %v, want %v", c.name, h.Factors.ActivityRecency, c.recency)
		}
		if got := h.Evidence.MostRecentActivityID; got == nil || *got != mail.UUID {
			t.Errorf("%s: the recency evidence names %v, want the past email %v", c.name, got, mail.UUID)
		}
		if h.Evidence.MeetingAhead != (c.recency == 1.0) {
			t.Errorf("%s: evidence says meeting ahead %v", c.name, h.Evidence.MeetingAhead)
		}
	}
}

// Around the meeting's start, before the refold has moved the clock and after,
// the deal neither drops recency nor reads as stalled. Each read runs on a pool
// whose now() is the moment it asks about.
func TestDealHealthHoldsAcrossTheMeetingsStart(t *testing.T) {
	f := newClockFixture(t)
	deal := f.idleDeal(t, "Meeting starts", 70)
	meetsAt := f.now.AddDate(0, 0, 2)
	f.logOn(t, "meeting", onRecord("deal", deal), meetsAt)
	ws := ids.From[ids.WorkspaceKind](f.e.WS)
	for _, read := range []struct {
		when  string
		shift time.Duration
	}{
		{"just before it starts", 2*24*time.Hour - 2*time.Minute},
		{"just after it starts, before the refold", 2*24*time.Hour + 2*time.Minute},
		{"three days later, no refold run", 5 * 24 * time.Hour},
	} {
		at := clocktest.Now(t).UTC().Add(read.shift)
		store := deals.NewStore(database.BindTo(shiftedPool(t, read.shift), ws), DealsInstallation())
		h, err := store.DealHealth(f.e.Admin(), ids.From[ids.DealKind](deal), at)
		if err != nil {
			t.Fatal(err)
		}
		if h.Factors.ActivityRecency != 1.0 || h.Evidence.Stalled {
			t.Errorf("%s: recency %v, stalled %v; want 1.0 and not stalled", read.when, h.Factors.ActivityRecency, h.Evidence.Stalled)
		}
	}
	runRefold(t, shiftedPool(t, 3*24*time.Hour))
	assertClock(t, f.dealClock(t, deal), meetsAt, "the refold moves the clock to the meeting")
	if h := f.health(t, deal, meetsAt.Add(time.Minute)); h.Factors.ActivityRecency != 1.0 || h.Evidence.Stalled || h.Evidence.MeetingAhead {
		t.Errorf("after the refold: recency %v, stalled %v, meeting ahead %v; want 1.0, not stalled, the clock carrying it",
			h.Factors.ActivityRecency, h.Evidence.Stalled, h.Evidence.MeetingAhead)
	}
}

func TestABookedMeetingAheadKeepsTheCloseDateSweepFromDowngrading(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Quiet but booked", e.late, new("commit"), new(30), 90)
	subject, booked, at := "Next step", "booked", clocktest.Now(t).UTC().AddDate(0, 0, 5)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &booked, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{onRecord("deal", id)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.readSwept(t, id).forecastCat; got == nil || *got != "commit" {
		t.Errorf("forecast_category = %v, want commit: a booked meeting ahead is not a deal gone quiet", got)
	}
}

// Each reader that picks a last touch itself names the past email and not the
// future-dated one.
func TestTheLastTouchReadersIgnoreAFutureDatedEmail(t *testing.T) {
	f := newClockFixture(t)
	past, future := f.now.AddDate(0, 0, -30), f.now.AddDate(0, 0, 5)

	deal := f.idleDeal(t, "Reader deal", 30)
	f.logOn(t, "email", onRecord("deal", deal), future)
	page, _, err := f.e.Deals.ListDeals(f.e.Admin(), deals.ListDealsInput{Limit: new(100)})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page {
		if ids.UUID(row.Id) == deal && (row.LastEmail == nil || !row.LastEmail.OccurredAt.Equal(past)) {
			t.Errorf("the board card's last email = %v, want the past one at %v", row.LastEmail, past)
		}
	}
	var facts deals.QuietFacts
	inWorkspaceTx(t, f.e, func(tx pgx.Tx) (err error) {
		facts, err = deals.ReadQuietFacts(f.e.Admin(), tx, ids.From[ids.DealKind](deal))
		return err
	})
	if facts.LastOutbound == nil || !facts.LastOutbound.At.Equal(past) {
		t.Errorf("the close-date review's last outbound = %v, want the past email at %v", facts.LastOutbound, past)
	}
	quiet, err := f.e.Activities.LastTouchBefore(f.e.Admin(), f.now.AddDate(0, 0, -7), 100, "reader_test")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(quiet, func(c activities.LastTouchCandidate) bool {
		return c.EntityID == deal && c.LastTouch.Equal(past)
	}) {
		t.Errorf("the no-activity reminder does not see the deal quiet since %v: %+v", past, quiet)
	}

	for _, at := range []time.Time{past, future} {
		subject, inbound := "Their reply", "inbound"
		if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
			Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
			Links: []activities.ActivityLinkInput{onRecord("deal", deal)},
		}); err != nil {
			t.Fatal(err)
		}
	}

	contact := f.e.SeedContact(t, "Reader Contact", nil)
	f.logOn(t, "email", onRecord("contact", contact), past)
	f.logOn(t, "email", onRecord("contact", contact), future)
	var touches map[ids.ContactID]contact360.LastTouch
	inWorkspaceTx(t, f.e, func(tx pgx.Tx) (err error) {
		touches, err = contact360.LastTouchFor(f.e.Admin(), tx, []ids.ContactID{ids.From[ids.ContactKind](contact)}, contact360.AssembleOptions{})
		return err
	})
	if got := touches[ids.From[ids.ContactKind](contact)].OutboundAt; got == nil || !got.Equal(past) {
		t.Errorf("the contact's last outbound touch = %v, want the past email at %v", got, past)
	}

	name := "Reader Lead"
	created, _, err := f.e.Contacts.CreateLead(f.e.Admin(), contacts.CreateLeadInput{FullName: &name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	lead := ids.UUID(created.Id)
	f.logOn(t, "email", onRecord("lead", lead), past)
	f.logOn(t, "email", onRecord("lead", lead), future)
	l, err := f.e.Contacts.GetLead(f.e.Admin(), ids.From[ids.LeadKind](lead), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if l.LastActivityAt == nil || !l.LastActivityAt.Equal(past) {
		t.Errorf("the lead's last activity = %v, want the past email at %v", l.LastActivityAt, past)
	}
}

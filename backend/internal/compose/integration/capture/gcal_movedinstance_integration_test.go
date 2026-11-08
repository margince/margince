// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A recurring meeting on two colleagues' Google Calendars, with instances
// moved. An instance is known by its slot in the series, so a move neither
// splits one meeting in two nor joins it to another instance.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// instance is one occurrence of the series: the slot the series gave it and
// where it is now.
type instance struct{ slot, start time.Time }

// bookSeries puts the series' instances on both calendars. Each calendar
// numbers an instance by its own event id and its slot, as Google does.
func (p *pairEnv) bookSeries(instances ...instance) {
	for label, self := range map[string]string{"a": captureOwner, "b": guestColleagueA} {
		other := guestColleagueA
		if label == "b" {
			other = captureOwner
		}
		items := make([]map[string]any, 0, len(instances))
		for _, in := range instances {
			ev := lateGuestEvent("review-"+label+"_"+in.slot.UTC().Format("20060102T150405Z"), "confirmed", in.start, captureOwner,
				selfEntry(self, "accepted"), guestEntry(other, "accepted"), guestEntry(guestKnown, "accepted"))
			ev["iCalUID"] = crossSeatUID
			ev["originalStartTime"] = map[string]string{"dateTime": in.slot.Format(time.RFC3339)}
			items = append(items, ev)
		}
		p.cals.set(label, items...)
	}
}

// wantEachInstanceOnce asserts one live meeting per instance, each imported by
// both seats, holding its slot's identity and stored at its current start.
func (p *pairEnv) wantEachInstanceOnce(t *testing.T, instances ...instance) {
	t.Helper()
	for _, in := range instances {
		row := p.wantSharedInstance(t, len(instances), in)
		if got := countRows(t, p.env.e, `SELECT count(*) FROM activity WHERE id = '`+row.String()+`'
			AND occurred_at = '`+in.start.UTC().Format(time.RFC3339)+`'`); got != 1 {
			t.Errorf("the instance of %s is not stored at its current start %s", in.slot, in.start)
		}
	}
}

// wantSharedInstance asserts the series holds live meetings in all, and that the
// instance's identity is on a row both seats import. It answers that row.
func (p *pairEnv) wantSharedInstance(t *testing.T, live int, in instance) ids.UUID {
	t.Helper()
	e := p.env.e
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'meeting' AND archived_at IS NULL`); got != live {
		t.Fatalf("%d live meetings for the series, want one per instance (%d)", got, live)
	}
	key := activities.MeetingIdentityKey(crossSeatUID, in.slot.UTC().Format(time.RFC3339))
	row := scalarID(t, e, `SELECT activity_id FROM activity_identity WHERE identity_key = '`+key+`'`)
	if got := countRows(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = '`+row.String()+`'
		AND user_id IN ('`+e.Rep1.String()+`', '`+p.guestSeat.String()+`')`); got != 2 {
		t.Errorf("the instance of %s is imported by %d of the two seats, want both", in.slot, got)
	}
	return row
}

func TestAnInstanceMovedBetweenTwoSeatsSyncsStaysOneMeeting(t *testing.T) {
	p := newPairEnv(t)
	slot := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	before := instance{slot.AddDate(0, 0, -7), slot.AddDate(0, 0, -7)}
	after := instance{slot.AddDate(0, 0, 7), slot.AddDate(0, 0, 7)}
	p.bookSeries(before, instance{slot, slot}, after)
	p.sync(t, "a")
	moved := instance{slot, slot.Add(2 * time.Hour)}
	p.bookSeries(before, moved, after)

	p.sync(t, "b")
	p.wantSharedInstance(t, 3, moved)
	p.sync(t, "a")

	p.wantEachInstanceOnce(t, before, moved, after)
}

func TestInstancesMovedOntoOneSlotStayTheirOwnMeetings(t *testing.T) {
	day := captureNow.AddDate(0, 0, -3).Truncate(24 * time.Hour).Add(10 * time.Hour)
	monday, tuesday, wednesday := day.AddDate(0, 0, -1), day, day.AddDate(0, 0, 1)
	for name, series := range map[string][]instance{
		"onto the next instance's slot": {{monday, tuesday}, {tuesday, tuesday}},
		"two instances onto one day":    {{monday, tuesday.Add(5 * time.Hour)}, {tuesday, tuesday}, {wednesday, tuesday.Add(5 * time.Hour)}},
	} {
		t.Run(name, func(t *testing.T) {
			p := newPairEnv(t)
			p.bookSeries(series...)

			p.sync(t, "a")
			p.sync(t, "b")

			p.wantEachInstanceOnce(t, series...)
		})
	}
}

// A replay of a moved instance still filed under its start key re-keys it by
// its slot, and the start key no longer names it.
func TestAReplayRetiresAMovedInstancesStartKey(t *testing.T) {
	p := newPairEnv(t)
	slot := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	moved := instance{slot, slot.Add(26 * time.Hour)}
	p.bookSeries(moved)
	p.sync(t, "a")
	startKey := activities.MeetingIdentityKey(crossSeatUID, moved.start.UTC().Format(time.RFC3339))
	slotKey := activities.MeetingIdentityKey(crossSeatUID, slot.UTC().Format(time.RFC3339))
	p.exec(t, `UPDATE activity_identity SET identity_key = $1 WHERE identity_key = $2`, startKey, slotKey)

	p.sync(t, "a")

	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity_identity WHERE identity_key = '`+startKey+`'`); got != 0 {
		t.Error("the moved instance still holds the key of its start")
	}
	if got := countRows(t, p.env.e, `SELECT count(*) FROM activity_identity WHERE identity_key = '`+slotKey+`'`); got != 1 {
		t.Errorf("%d rows hold the instance's slot key, want 1", got)
	}
}

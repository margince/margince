// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An organizer who edits a weekly meeting with "this and following events"
// splits it. Google gives the rest of the series a new iCalUID. It sends
// nothing for the old series' instances, and Home still shows one meeting per
// slot. The test lives here because Home's meeting lane is this package's seam.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	splitHost  = "host@customer.test"
	splitBuyer = "buyer@customer.test"
	splitPeer  = "peer@customer.test"
)

// seriesSplit is the admin's own Google calendar and one slot of a weekly
// meeting on it.
type seriesSplit struct {
	e     *integration.Env
	sink  *capture.Sink
	owner string
	slot  time.Time
}

func newSeriesSplit(t *testing.T) seriesSplit {
	t.Helper()
	e := integration.Setup(t)
	s := seriesSplit{
		e: e, sink: NewCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil),
		slot: time.Now().UTC().AddDate(0, 0, 2).Truncate(time.Hour),
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id = $1`, e.AdminUser).Scan(&s.owner)
	}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES('gcal',$1,'connected',$2)`,
		e.AdminUser, s.owner)
	return s
}

// sync captures the occurrence of series uid at the slot, organized by
// organizer and listing the calendar owner and guests.
func (s seriesSplit) sync(t *testing.T, uid, organizer string, guests ...string) ids.UUID {
	t.Helper()
	attendees := []map[string]any{{"email": s.owner, "responseStatus": "accepted", "self": true}}
	for _, address := range append([]string{organizer}, guests...) {
		if address != s.owner {
			attendees = append(attendees, map[string]any{"email": address, "responseStatus": "accepted"})
		}
	}
	slot := map[string]string{"dateTime": s.slot.Format(time.RFC3339)}
	raw, err := json.Marshal(map[string]any{
		"id": uid + "_" + s.slot.Format("20060102T150405Z"), "iCalUID": uid, "recurringEventId": uid,
		"status": "confirmed", "summary": "Meeting " + uid, "start": slot, "originalStartTime": slot,
		"organizer": map[string]string{"email": organizer}, "attendees": attendees,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := gcal.RecordOf(raw, s.owner)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.sink.Upsert(s.e.ConnectorOwnerCtx(s.e.AdminUser, "gcal"), rec)
	if err != nil {
		t.Fatalf("syncing series %s: %v", uid, err)
	}
	return ref.ID
}

// onHome answers the meetings Home's lane lists around the slot.
func (s seriesSplit) onHome(t *testing.T) []attention.Meeting {
	t.Helper()
	lane := attentionMeetings{store: activities.NewStore(InstallationDB(s.e.Pool))}
	window := attention.MeetingWindow{From: s.slot.Add(-time.Hour), Until: s.slot.Add(time.Hour)}
	shown, _, err := lane.Today(s.e.Admin(), window, 12, attention.TasksVisible, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the meeting lane: %v", err)
	}
	return shown
}

// The series Google split off weeklyUID at the slot, named as Google names it.
func (s seriesSplit) splitUID() string {
	return "weekly_R" + s.slot.Format("20060102T150405") + "@google.com"
}

const weeklyUID = "weekly@google.com"

func TestASeriesSplitByTheOrganizerShowsOneMeetingPerSlot(t *testing.T) {
	s := newSeriesSplit(t)
	old := s.sync(t, weeklyUID, splitHost, splitBuyer)
	current := s.sync(t, s.splitUID(), splitHost, splitPeer, splitBuyer)

	shown := s.onHome(t)
	if len(shown) != 1 {
		t.Fatalf("Home lists %d meetings at the slot, want one", len(shown))
	}
	if shown[0].ID != current {
		t.Errorf("Home lists %s, want the new series' meeting %s", shown[0].ID, current)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, old); got != 1 {
		t.Error("the old series' meeting is still live")
	}
	if again := s.sync(t, weeklyUID, splitHost, splitBuyer); again != current {
		t.Errorf("the old series synced again reached %s, want the new series' meeting", again)
	}
}

func TestASeriesSplitFoldsWhicheverSeriesArrivesFirst(t *testing.T) {
	s := newSeriesSplit(t)
	current := s.sync(t, s.splitUID(), splitHost, splitPeer, splitBuyer)
	s.sync(t, weeklyUID, splitHost, splitBuyer)

	shown := s.onHome(t)
	if len(shown) != 1 || shown[0].ID != current {
		t.Fatalf("Home lists %v at the slot, want the new series' meeting alone", shown)
	}
}

func TestTwoSeriesAtOneSlotStayTwoMeetingsWhenTheyAreNotOne(t *testing.T) {
	cases := map[string]struct {
		organizers [2]string
		split      bool
	}{
		"a split series of another organizer":       {organizers: [2]string{splitHost, splitPeer}, split: true},
		"an unrelated series of the same organizer": {organizers: [2]string{splitHost, splitHost}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSeriesSplit(t)
			second := "pricing@google.com"
			if c.split {
				second = s.splitUID()
			}
			for i, uid := range []string{weeklyUID, second} {
				s.sync(t, uid, c.organizers[i], splitBuyer)
			}
			if shown := s.onHome(t); len(shown) != 2 {
				t.Errorf("Home lists %d meetings at the slot, want both", len(shown))
			}
		})
	}
}

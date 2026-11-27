// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An organizer who edits a weekly meeting with "this and following events"
// splits it. Google gives the rest of the series a new iCalUID. It sends
// nothing for the old series' instances, and Home still shows one meeting per
// slot. The tests live here because Home's meeting lane, the repair task and
// the fold are this package's unexported code.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
		slot: clocktest.Now(t).UTC().AddDate(0, 0, 2).Truncate(time.Hour),
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
	t.Parallel()
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
	t.Parallel()
	s := newSeriesSplit(t)
	current := s.sync(t, s.splitUID(), splitHost, splitPeer, splitBuyer)
	s.sync(t, weeklyUID, splitHost, splitBuyer)

	shown := s.onHome(t)
	if len(shown) != 1 || shown[0].ID != current {
		t.Fatalf("Home lists %v at the slot, want the new series' meeting alone", shown)
	}
}

func TestTwoSeriesAtOneSlotStayTwoMeetingsWhenTheyAreNotOne(t *testing.T) {
	t.Parallel()
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

// storedSplitPair captures one slot of a weekly series and then of the series
// Google split off it on the admin's calendar. The sink does not fold split series, as capture stored
// them before it did. It answers the old and the new meeting.
func storedSplitPair(t *testing.T, e *integration.Env, organizers [2]string) (ids.UUID, ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES('gcal',$1,'connected','admin@calendar.test')`,
		e.AdminUser)
	// Wall time: the writer refuses "held" for a meeting its own clock puts ahead.
	slot := time.Now().UTC().AddDate(0, 0, -3).Truncate(time.Hour)
	var stored [2]ids.UUID
	for i, series := range []string{weeklyUID, "weekly_R" + slot.Format("20060102T150405") + "@google.com"} {
		stored[i] = integration.CalendarMeeting{
			Event: fmt.Sprintf("event-%d", i), At: slot,
			Identity: connector.CrossDoorIdentity{Series: series, Occurrence: slot},
			Parties: []connector.MessageParticipant{
				{Email: organizers[i], Role: connector.ParticipantRoleOrganizer},
				{Email: splitBuyer, Role: "attendee"},
			},
		}.Capture(t, e)
	}
	return stored[0], stored[1]
}

func TestTheSplitSeriesTaskFoldsOnlyAPairThatIsOneMeeting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		organizers [2]string
		worked     bool
		folded     bool
	}{
		{name: "one meeting in two series", organizers: [2]string{splitHost, splitHost}, folded: true},
		{name: "another organizer", organizers: [2]string{splitHost, splitPeer}},
		{name: "a member recorded the old meeting's outcome", organizers: [2]string{splitHost, splitHost}, worked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			old, current := storedSplitPair(t, e, tc.organizers)
			if tc.worked {
				e.RecordHeld(t, old)
			}

			runCalendarDescriptionTask(ctx, t, e, splitSeriesDataTask(e.Pool))
			runCalendarDescriptionTask(ctx, t, e, splitSeriesDataTask(e.Pool))

			oldLive := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, old)
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, current); got != 1 {
				t.Fatal("the new series' meeting was retired")
			}
			if tc.folded != (oldLive == 0) {
				t.Fatalf("the old series' meeting live = %d, want folded = %v", oldLive, tc.folded)
			}
			if !tc.folded {
				return
			}
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
				AND actor_type = 'system' AND after ? 'archived_at'`, old); got != 1 {
				t.Errorf("the fold wrote %d archive audit rows, want 1", got)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity_identity WHERE activity_id = $1`, current); got != 2 {
				t.Errorf("the new meeting holds %d identities, want both series' keys", got)
			}
		})
	}
}

// A replay moves the split series' meeting after the pair was found. The fold
// asks the rule again under the lock, so two meetings at two slots stay.
func TestASeriesSplitFoldAsksTheRuleAgainUnderTheLock(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	old, current := storedSplitPair(t, e, [2]string{splitHost, splitHost})
	meetings := activities.NewStore(InstallationDB(e.Pool))
	var splits []activities.SeriesSplit
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		splits, err = activities.SeriesSplitsTx(ctx, tx, ids.From[ids.ActivityKind](current), e.AdminUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(splits) != 1 || splits[0].Replaced != ids.From[ids.ActivityKind](old) {
		t.Fatalf("found splits %v, want the stored pair", splits)
	}
	e.WsExec(t, `UPDATE activity SET occurred_at = occurred_at + interval '1 hour' WHERE id = $1`, current)

	var folded bool
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		folded, err = foldSeriesSplitTx(ctx, tx, meetings, splits[0], e.AdminUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if folded || e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, old) != 1 {
		t.Error("a pair the rule no longer holds was folded")
	}
}

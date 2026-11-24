// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// storedSplitPair captures one slot of a weekly series and then of the series
// Google split off it on the admin's calendar. The sink does not fold split series, as capture stored
// them before it did. It answers the old and the new meeting.
func storedSplitPair(t *testing.T, e *integration.Env, organizers [2]string) (ids.UUID, ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES('gcal',$1,'connected','admin@calendar.test')`,
		e.AdminUser)
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

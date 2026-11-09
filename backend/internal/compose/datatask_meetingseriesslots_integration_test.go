// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const weeklySeries = "weekly-sync@acme.test"

var (
	mondaySlot  = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	tuesdaySlot = mondaySlot.AddDate(0, 0, 1)
)

func slotKey(at time.Time) string {
	return activities.MeetingIdentityKey(weeklySeries, at.Format(time.RFC3339))
}

// seedStoredOccurrence leaves what a Google Calendar sync of one occurrence of
// the weekly series stored: the row under the event's own key and the event.
// A zero slot stores a one-off.
func seedStoredOccurrence(t *testing.T, e *integration.Env, event string, slot, start time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source_system, source_id, source, captured_by)
		VALUES ($1, 'meeting', 'Weekly sync', $2, 'gcal', $3, 'gcal:' || $3, $4)`,
		id, start, event, "connector:gcal:"+e.Rep2.String())
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'client@acme.test', 'organizer')`, id)
	stored := map[string]any{
		"id": event, "iCalUID": weeklySeries, "status": "confirmed", "summary": "Weekly sync",
		"start":     map[string]string{"dateTime": start.Format(time.RFC3339)},
		"end":       map[string]string{"dateTime": start.Add(time.Hour).Format(time.RFC3339)},
		"organizer": map[string]string{"email": "client@acme.test"},
		"attendees": []map[string]any{{"email": rep2Address, "self": true, "responseStatus": "accepted"}},
	}
	if !slot.IsZero() {
		stored["originalStartTime"] = map[string]string{"dateTime": slot.Format(time.RFC3339)}
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('gcal', $1, $2)`, event, payload)
	return id
}

func holdIdentity(t *testing.T, e *integration.Env, meeting ids.UUID, key string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'connector:gcal')`, key, meeting)
}

func identitiesOf(t *testing.T, e *integration.Env, meeting ids.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT coalesce(string_agg(identity_key, ' ' ORDER BY identity_key), '')
		FROM activity_identity WHERE activity_id = $1`, meeting)
}

// A moved occurrence keyed by its start is re-keyed by its slot, and the start
// key no longer names it. A one-off keeps its key: its stored original may
// predate a reschedule.
func TestTheSeriesSlotTaskRekeysAMovedOccurrenceBySlot(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	moved := tuesdaySlot.Add(5 * time.Hour)
	monday := seedStoredOccurrence(t, e, "evt-mon", mondaySlot, moved)
	holdIdentity(t, e, monday, slotKey(moved))
	oneOff := seedStoredOccurrence(t, e, "evt-once", time.Time{}, mondaySlot)
	holdIdentity(t, e, oneOff, slotKey(moved.Add(time.Hour)))

	for range 2 {
		runCalendarDescriptionTask(ctx, t, e, meetingSeriesSlotsDataTask(e.Pool))
	}

	if got := identitiesOf(t, e, monday); got != slotKey(mondaySlot) {
		t.Errorf("the moved occurrence holds %q, want its slot key %q alone", got, slotKey(mondaySlot))
	}
	if got := identitiesOf(t, e, oneOff); got != slotKey(moved.Add(time.Hour)) {
		t.Errorf("the one-off holds %q, want the key it had", got)
	}
}

// foldTuesdayOntoMonday leaves what a sync did before the slot key. Monday's
// occurrence, moved onto Tuesday's slot, held Tuesday's start key. So
// Tuesday's own row was folded onto it by the real writer. before runs on the
// two rows ahead of the fold.
func foldTuesdayOntoMonday(t *testing.T, e *integration.Env, before func(monday, tuesday ids.UUID)) (monday, tuesday ids.UUID) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	monday = seedStoredOccurrence(t, e, "evt-mon", mondaySlot, tuesdaySlot)
	holdIdentity(t, e, monday, slotKey(tuesdaySlot))
	tuesday = seedStoredOccurrence(t, e, "evt-tue", tuesdaySlot, tuesdaySlot)
	if before != nil {
		before(monday, tuesday)
	}
	meetings := activities.NewStore(e.DB())
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return meetings.RetireMeetingCopyTx(ctx, tx, ids.From[ids.ActivityKind](tuesday), ids.From[ids.ActivityKind](monday))
	}); err != nil {
		t.Fatal(err)
	}
	return monday, tuesday
}

// ownerExec runs what the runtime role may not: editing an audit row's
// evidence, or a trigger on the activity table.
func ownerExec(t *testing.T, sql string) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), sql); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// restrictMeeting places a meeting under a hold, with the retention evidence
// the restriction check asks for.
func restrictMeeting(t *testing.T, e *integration.Env, meeting ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, meeting)
	e.WsExec(t, `UPDATE activity SET restricted_at = now(), archived_at = coalesce(archived_at, now()), restricted_reason = 'litigation hold',
		restricted_until = now() + interval '1 year',
		retention_class = 'commercial_correspondence', retention_class_at = now() WHERE id = $1`, meeting)
}

func outboxRows(t *testing.T, e *integration.Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM event_outbox`)
}

// tuesdayGuest is the guest only Tuesday's occurrence lists.
const tuesdayGuest = "tuesday-only@acme.test"

func tuesdayGuestOn(t *testing.T, e *integration.Env, meeting ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = $2`, meeting, tuesdayGuest) == 1
}

// rerunSeriesSlotSteps runs every step once more, one row per page, and
// answers how many rows the pages changed.
func rerunSeriesSlotSteps(ctx context.Context, t *testing.T, e *integration.Env) int {
	t.Helper()
	changed := 0
	for _, step := range meetingSeriesSlotsDataTask(e.Pool).Steps {
		in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
		for range 50 {
			var out datatask.BatchOut
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				var err error
				out, err = step.Batch(ctx, tx, in)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			changed += out.Changed
			if out.Done {
				break
			}
			in.Cursor = out.Cursor
		}
	}
	return changed
}

// Two slots of one series folded together are split. The folded row is live
// again with its own slot's key. The guest the fold carried leaves the
// survivor, and the split is audited and announced. Running it again, page
// by page, changes nothing.
func TestTheSeriesSlotTaskSplitsTwoSlotsFoldedTogether(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	monday, tuesday := foldTuesdayOntoMonday(t, e, func(_, tuesday ids.UUID) {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, tuesday, tuesdayGuest)
	})
	if !tuesdayGuestOn(t, e, monday) {
		t.Fatal("the fold did not carry the folded occurrence's guest; the case proves nothing")
	}
	outboxBefore := outboxRows(t, e)

	runCalendarDescriptionTask(ctx, t, e, meetingSeriesSlotsDataTask(e.Pool))

	if !liveMeeting(t, e, tuesday) {
		t.Fatal("the folded occurrence is still archived")
	}
	if got := identitiesOf(t, e, monday); got != slotKey(mondaySlot) {
		t.Errorf("the survivor holds %q, want its own slot key %q alone", got, slotKey(mondaySlot))
	}
	if got := identitiesOf(t, e, tuesday); got != slotKey(tuesdaySlot) {
		t.Errorf("the split occurrence holds %q, want its slot key %q", got, slotKey(tuesdaySlot))
	}
	if tuesdayGuestOn(t, e, monday) || !tuesdayGuestOn(t, e, tuesday) {
		t.Error("the guest the fold carried is still on the survivor, or gone from its own meeting")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
		AND action = 'restore'`, tuesday); got != 1 {
		t.Errorf("the split wrote %d restore audit rows, want 1", got)
	}
	outboxAfter := outboxRows(t, e)
	if outboxAfter <= outboxBefore {
		t.Error("the split announced nothing on the outbox")
	}

	if changed := rerunSeriesSlotSteps(ctx, t, e); changed != 0 {
		t.Errorf("a second run changed %d rows, want none", changed)
	}
	if got := outboxRows(t, e); got != outboxAfter {
		t.Errorf("a second run wrote %d outbox rows, want none", got-outboxAfter)
	}
}

// foldTheOlderWay folds tuesday onto monday as a fold did before it listed
// the guests it carried, which no writer does any more. stranger adds a guest
// row to monday from inside that transaction.
func foldTheOlderWay(t *testing.T, e *integration.Env, monday, tuesday ids.UUID, stranger bool) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	statements := []string{
		`INSERT INTO activity_participant (activity_id, address, role)
		 SELECT $1, d.address, d.role FROM activity_participant d WHERE d.activity_id = $2
		    AND NOT EXISTS (SELECT 1 FROM activity_participant s WHERE s.activity_id = $1 AND lower(s.address) = lower(d.address))`,
		`UPDATE activity SET archived_at = now() WHERE id = $2 AND $1::uuid IS NOT NULL`,
		`INSERT INTO audit_log (action, entity_type, entity_id, actor_type, actor_id, before, after, evidence)
		 VALUES ('archive', 'activity', $2, 'system', 'system', '{"archived_at": null}', jsonb_build_object('archived_at', now()),
		         jsonb_build_object('reason', 'meeting_duplicate', 'survivor', $1::uuid::text))`,
	}
	if stranger {
		statements = append(statements, `INSERT INTO activity_participant (activity_id, address, role)
			SELECT $1, 'stranger@acme.test', 'attendee' WHERE $2::uuid IS NOT NULL`)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		for _, statement := range statements {
			if _, err := tx.Exec(ctx, statement, monday, tuesday); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A fold recorded before folds listed their guests is split on the guest rows
// its own transaction inserted. Each must repeat a guest of the copy. A row
// that transaction inserted from anywhere else leaves it unknown.
func TestTheSeriesSlotTaskReadsAnOlderFoldsGuestsFromItsTransaction(t *testing.T) {
	for name, tc := range map[string]struct{ stranger, split bool }{
		"every row repeats a guest of the copy": {split: true},
		"a row came from elsewhere":             {stranger: true},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			monday := seedStoredOccurrence(t, e, "evt-mon", mondaySlot, tuesdaySlot)
			holdIdentity(t, e, monday, slotKey(tuesdaySlot))
			tuesday := seedStoredOccurrence(t, e, "evt-tue", tuesdaySlot, tuesdaySlot)
			e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, tuesday, tuesdayGuest)
			foldTheOlderWay(t, e, monday, tuesday, tc.stranger)

			runCalendarDescriptionTask(ctx, t, e, meetingSeriesSlotsDataTask(e.Pool))

			if live := liveMeeting(t, e, tuesday); live != tc.split {
				t.Fatalf("the folded occurrence is live=%v, want %v", live, tc.split)
			}
			if tc.split && tuesdayGuestOn(t, e, monday) {
				t.Error("the guest the older fold carried is still on the survivor")
			}
		})
	}
}

// A fold whose undo would not give back what it took, or whose rows a privacy
// act reached, is left for review. The survivor is still re-keyed, so the
// stale key resolves nothing.
func TestTheSeriesSlotTaskLeavesAnUnsafeSplitForReview(t *testing.T) {
	for name, tc := range map[string]struct {
		before func(t *testing.T, e *integration.Env, monday, tuesday ids.UUID)
		after  func(t *testing.T, e *integration.Env, monday, tuesday ids.UUID)
	}{
		"a colleague changed the survivor after the fold": {after: func(t *testing.T, e *integration.Env, monday, _ ids.UUID) {
			e.WsExec(t, `INSERT INTO audit_log (action, entity_type, entity_id, actor_type, actor_id, after, occurred_at)
				VALUES ('update', 'activity', $1, 'human', $2, '{"meeting_status":"held"}', now() + interval '1 minute')`,
				monday, "human:"+e.Rep2.String())
		}},
		"another meeting holds the folded slot": {after: func(t *testing.T, e *integration.Env, _, _ ids.UUID) {
			other := importMeeting(t, e, "human:"+e.Rep1.String())
			e.WsExec(t, `DELETE FROM activity_identity WHERE identity_key = $1`, slotKey(tuesdaySlot))
			holdIdentity(t, e, other, slotKey(tuesdaySlot))
		}},
		"the rows came from two seats' calendars": {before: func(t *testing.T, e *integration.Env, _, tuesday ids.UUID) {
			e.WsExec(t, `UPDATE activity SET captured_by = $2 WHERE id = $1`, tuesday, "connector:gcal:"+e.Rep1.String())
		}},
		"the fold moved document links": {before: func(t *testing.T, e *integration.Env, _, tuesday ids.UUID) {
			e.WsExec(t, `INSERT INTO attachment (entity_type, entity_id, activity_id, filename, storage_key, bytes_withheld, url,
				source, captured_by, category, external_source_id, external_part_id)
				VALUES ('activity', $1::uuid, $1::uuid, 'Notes', '', true, 'https://docs.example.test/notes', 'gcal', 'connector:gcal',
				        'link', 'meeting:' || $1::uuid::text, 'notes-1')`, tuesday)
		}},
		"the survivor is restricted": {after: func(t *testing.T, e *integration.Env, monday, _ ids.UUID) {
			restrictMeeting(t, e, monday)
		}},
		"the folded row is restricted": {after: func(t *testing.T, e *integration.Env, _, tuesday ids.UUID) {
			restrictMeeting(t, e, tuesday)
		}},
		"the folded row was erased": {after: func(t *testing.T, e *integration.Env, _, tuesday ids.UUID) {
			e.SeedScrubTombstone(t, "activity", tuesday, clocktest.Now(t))
		}},
		"the survivor was erased in place": {after: func(t *testing.T, e *integration.Env, monday, _ ids.UUID) {
			e.SeedScrubTombstone(t, "activity", monday, clocktest.Now(t))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			var before func(monday, tuesday ids.UUID)
			if tc.before != nil {
				before = func(monday, tuesday ids.UUID) { tc.before(t, e, monday, tuesday) }
			}
			monday, tuesday := foldTuesdayOntoMonday(t, e, before)
			if tc.after != nil {
				tc.after(t, e, monday, tuesday)
			}

			runCalendarDescriptionTask(ctx, t, e, meetingSeriesSlotsDataTask(e.Pool))

			if e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, tuesday) != 0 {
				t.Error("the folded occurrence was split though the split was unsafe")
			}
			restricted := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND restricted_at IS NOT NULL`, monday) == 1
			if got := identitiesOf(t, e, monday); !restricted && got != slotKey(mondaySlot) {
				t.Errorf("the survivor holds %q, want its own slot key %q alone", got, slotKey(mondaySlot))
			}
		})
	}
}

// A refusal after the restore rolls the whole split back: the folded row
// stays archived and the restore leaves no audit row.
func TestARefusalAfterTheRestoreRollsTheSplitBack(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	_, tuesday := foldTuesdayOntoMonday(t, e, nil)
	other := importMeeting(t, e, "human:"+e.Rep1.String())
	// Takes the folded slot's key the moment the folded row is unarchived,
	// so the split's last check finds it held by a third meeting.
	ownerExec(t, `CREATE FUNCTION take_folded_slot() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
			VALUES ('meeting', '`+slotKey(tuesdaySlot)+`', '`+other.String()+`', 'test')
			ON CONFLICT (identity_kind, identity_key) DO UPDATE SET activity_id = EXCLUDED.activity_id;
			RETURN NEW;
		END $$`)
	ownerExec(t, `CREATE TRIGGER take_folded_slot AFTER UPDATE OF archived_at ON activity
		FOR EACH ROW WHEN (NEW.id = '`+tuesday.String()+`' AND OLD.archived_at IS NOT NULL AND NEW.archived_at IS NULL)
		EXECUTE FUNCTION take_folded_slot()`)
	t.Cleanup(func() {
		ownerExec(t, `DROP TRIGGER take_folded_slot ON activity`)
		ownerExec(t, `DROP FUNCTION take_folded_slot()`)
	})

	runCalendarDescriptionTask(ctx, t, e, meetingSeriesSlotsDataTask(e.Pool))

	if liveMeeting(t, e, tuesday) {
		t.Error("the folded occurrence is live though the split refused after the restore")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
		AND action = 'restore'`, tuesday); got != 0 {
		t.Errorf("the rolled-back split left %d restore audit rows, want none", got)
	}
}

// A claim retires only the keys its own row holds in the series. A colleague's
// row in the same series keeps its key, and a claim that loses still retires
// the claiming row's stale key.
func TestAMeetingClaimRetiresOnlyItsOwnRowsKeysInTheSeries(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	mine := seedStoredOccurrence(t, e, "evt-mine", mondaySlot, mondaySlot)
	holdIdentity(t, e, mine, slotKey(mondaySlot.Add(time.Hour)))
	colleagues := seedStoredOccurrence(t, e, "evt-colleague", tuesdaySlot, tuesdaySlot)
	e.WsExec(t, `UPDATE activity SET captured_by = $2 WHERE id = $1`, colleagues, "connector:gcal:"+e.Rep1.String())
	holdIdentity(t, e, colleagues, slotKey(tuesdaySlot))

	var won bool
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		won, err = activities.ClaimIdentity(ctx, tx, ids.From[ids.ActivityKind](mine),
			activities.IdentityKindMeeting, slotKey(tuesdaySlot), "connector:gcal")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if won {
		t.Error("the claim won a key a colleague's row holds")
	}
	if got := identitiesOf(t, e, mine); got != "" {
		t.Errorf("the losing row still holds %q, want its stale key retired", got)
	}
	if got := identitiesOf(t, e, colleagues); got != slotKey(tuesdaySlot) {
		t.Errorf("the colleague's row holds %q, want its own key untouched", got)
	}
}

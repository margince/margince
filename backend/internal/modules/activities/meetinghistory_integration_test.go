// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A meeting's status is a history, and these are the cases that distinguish a
// history from a current value.
//
// The defect they exist against: `activity.meeting_status` says what a meeting
// IS. A meeting booked on Monday and held on Friday reads as `held` today, so
// counting the column reports no bookings for the week it was booked in, and
// every rate built on it inherits the error.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingHistoryRow is one transition as the tests read it back.
type meetingHistoryRow struct {
	Status            string
	EffectiveAt       time.Time
	ScheduledStart    *time.Time
	Actor             string
	PartialPreHistory bool
}

// historyOf reads every transition of one meeting, oldest first.
func historyOf(t *testing.T, e *sendEnv, activityID ids.ActivityID) []meetingHistoryRow {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `
		SELECT status, effective_at, scheduled_start, actor, partial_pre_history
		FROM activity_meeting_history WHERE activity_id = $1
		ORDER BY effective_at, id`, activityID.UUID)
	if err != nil {
		t.Fatalf("reading meeting history: %v", err)
	}
	defer rows.Close()
	var out []meetingHistoryRow
	for rows.Next() {
		var r meetingHistoryRow
		if err := rows.Scan(&r.Status, &r.EffectiveAt, &r.ScheduledStart, &r.Actor, &r.PartialPreHistory); err != nil {
			t.Fatalf("scanning a transition: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading meeting history: %v", err)
	}
	return out
}

// meetingCtx binds a rep who may write activities.
func meetingCtx(e *sendEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Create: true, Update: true},
				"person":   {Read: true}, "deal": {Read: true},
				"organization": {Read: true}, "project": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func meetingStore(e *sendEnv) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
}

// bookMeeting logs one booked meeting scheduled for start.
func bookMeeting(t *testing.T, e *sendEnv, subject string, start time.Time) ids.ActivityID {
	t.Helper()
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind:          string(crmcontracts.ActivityKindMeeting),
		Subject:       &subject,
		OccurredAt:    &start,
		MeetingStatus: &booked,
		Source:        "manual",
	})
	if err != nil {
		t.Fatalf("booking a meeting: %v", err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(out.Id))
}

// moveTo patches one meeting to a new status.
func moveTo(t *testing.T, e *sendEnv, id ids.ActivityID, status crmcontracts.ActivityMeetingStatus) {
	t.Helper()
	s := string(status)
	if _, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{MeetingStatus: &s}); err != nil {
		t.Fatalf("moving the meeting to %s: %v", status, err)
	}
}

// A booking and a holding are two events, not one status that overwrote the
// other. This is the case the whole table exists for.
func TestABookingAndAHoldingAreTwoEvents(t *testing.T) {
	e := setupSend(t)
	start := time.Now().Add(72 * time.Hour)
	id := bookMeeting(t, e, "Northgate kickoff", start)
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusHeld)

	history := historyOf(t, e, id)
	if len(history) != 2 {
		t.Fatalf("recorded %d transitions, want 2 (booked, then held)", len(history))
	}
	if history[0].Status != "booked" {
		t.Errorf("first transition = %q, want booked", history[0].Status)
	}
	if history[1].Status != "held" {
		t.Errorf("second transition = %q, want held", history[1].Status)
	}
	// The point of the table: the booking survives the holding. Reading the
	// current column alone would answer "held" and nothing else.
	if !history[0].EffectiveAt.Before(history[1].EffectiveAt) &&
		!history[0].EffectiveAt.Equal(history[1].EffectiveAt) {
		t.Errorf("the booking (%s) is not on or before the holding (%s)",
			history[0].EffectiveAt, history[1].EffectiveAt)
	}
}

// The actor is the authenticated principal, never something a caller asserts —
// the same rule the audit row's captured_by follows.
func TestATransitionRecordsTheAuthenticatedActor(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Kestrel intro", time.Now().Add(24*time.Hour))

	history := historyOf(t, e, id)
	if len(history) != 1 {
		t.Fatalf("recorded %d transitions, want 1", len(history))
	}
	want := "human:" + e.rep.String()
	if history[0].Actor != want {
		t.Errorf("actor = %q, want %q — the history must name the seat that acted", history[0].Actor, want)
	}
}

// A PATCH resending the status a meeting already holds is somebody saving a
// form. Recording it would make one meeting held twice, and every show rate
// built on the count would drift upward with every re-save.
func TestResendingTheSameStatusIsNotATransition(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Repeat save", time.Now().Add(48*time.Hour))
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusHeld)
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusHeld)
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusHeld)

	history := historyOf(t, e, id)
	if len(history) != 2 {
		t.Fatalf("recorded %d transitions, want 2 — three saves of `held` are one holding", len(history))
	}
}

// A no-show later corrected to held keeps BOTH events and has one terminal
// outcome. The correction is part of the record, not a replacement of it.
func TestACorrectedOutcomeKeepsBothEventsAndOneTerminalStatus(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "They did turn up", time.Now().Add(24*time.Hour))
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusNoShow)
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusHeld)

	history := historyOf(t, e, id)
	if len(history) != 3 {
		t.Fatalf("recorded %d transitions, want 3 (booked, no_show, held)", len(history))
	}
	if history[1].Status != "no_show" || history[2].Status != "held" {
		t.Errorf("transitions = %s then %s, want no_show then held", history[1].Status, history[2].Status)
	}
	// The terminal outcome is the LAST transition, and the correction did not
	// erase the no-show that a coverage answer still has to account for.
	var sawNoShow bool
	for _, h := range history {
		if h.Status == "no_show" {
			sawNoShow = true
		}
	}
	if !sawNoShow {
		t.Error("the corrected no_show vanished; a correction is a further event, not an edit of the past")
	}
}

// A cancelled meeting is its own event. It must be countable separately,
// because it belongs in neither half of a show rate.
func TestACancellationIsItsOwnEvent(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Called off", time.Now().Add(96*time.Hour))
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusCanceled)

	history := historyOf(t, e, id)
	if len(history) != 2 {
		t.Fatalf("recorded %d transitions, want 2 (booked, canceled)", len(history))
	}
	if history[1].Status != "canceled" {
		t.Errorf("second transition = %q, want canceled (one L, the spelling the activity CHECK has always used)",
			history[1].Status)
	}
}

// Every transition carries the start the meeting was scheduled for AS OF that
// moment, so a reschedule cannot rewrite which period an earlier event fell in.
func TestATransitionCarriesTheScheduledStartItWasMadeAgainst(t *testing.T) {
	e := setupSend(t)
	start := time.Now().Add(120 * time.Hour).UTC().Truncate(time.Second)
	id := bookMeeting(t, e, "Scheduled far out", start)

	history := historyOf(t, e, id)
	if len(history) != 1 {
		t.Fatalf("recorded %d transitions, want 1", len(history))
	}
	if history[0].ScheduledStart == nil {
		t.Fatal("the booking recorded no scheduled start; a due-date question cannot be answered from it")
	}
	if got := history[0].ScheduledStart.UTC().Truncate(time.Second); !got.Equal(start) {
		t.Errorf("scheduled_start = %s, want %s", got, start)
	}
}

// A connector replaying the same event writes no second transition. Without
// this a resync doubles every booking count in the period.
func TestAReplayedConnectorEventRecordsOneTransition(t *testing.T) {
	e := setupSend(t)
	ctx := meetingCtx(e)
	start := time.Now().Add(24 * time.Hour)
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	system, external := "google_calendar", "evt-replay-001"

	in := LogActivityInput{
		Kind:          string(crmcontracts.ActivityKindMeeting),
		Subject:       ptr("Synced meeting"),
		OccurredAt:    &start,
		MeetingStatus: &booked,
		Source:        "connector",
		SourceSystem:  &system,
		SourceID:      &external,
	}
	first, created, err := meetingStore(e).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if !created {
		t.Fatal("the first sync did not create the meeting; the fixture proves nothing")
	}
	second, createdAgain, err := meetingStore(e).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("replayed sync: %v", err)
	}
	if createdAgain {
		t.Error("the replay created a second activity; the capture key is what makes a resync safe")
	}
	if second.Id != first.Id {
		t.Fatalf("the replay returned activity %s, want the original %s", second.Id, first.Id)
	}

	history := historyOf(t, e, ids.From[ids.ActivityKind](ids.UUID(first.Id)))
	if len(history) != 1 {
		t.Fatalf("recorded %d transitions after a replay, want 1 — a resync must not double a booking", len(history))
	}
}

// The WRITER forwards the connector's identity onto the transition.
//
// The index test below proves the constraint exists; this proves the production
// path fills the columns it constrains. Without it, dropping SourceSystem and
// SourceID from the insert breaks idempotency for any future connector path
// that records a transition without re-logging the activity, and no test
// notices — the capture key hides it.
func TestTheWriterCarriesTheConnectorIdentityOntoTheTransition(t *testing.T) {
	e := setupSend(t)
	system, external := "google_calendar", "evt-identity-001"
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind:          string(crmcontracts.ActivityKindMeeting),
		Subject:       ptr("Synced with identity"),
		OccurredAt:    ptr(time.Now().Add(24 * time.Hour)),
		MeetingStatus: &booked,
		Source:        "connector",
		SourceSystem:  &system,
		SourceID:      &external,
	})
	if err != nil {
		t.Fatalf("capturing a synced meeting: %v", err)
	}

	var gotSystem, gotID *string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT source_system, source_id FROM activity_meeting_history WHERE activity_id = $1`,
		ids.UUID(out.Id)).Scan(&gotSystem, &gotID); err != nil {
		t.Fatalf("reading the transition: %v", err)
	}
	if gotSystem == nil || *gotSystem != system {
		t.Errorf("source_system = %v, want %q — the transition cannot be matched on replay without it", gotSystem, system)
	}
	if gotID == nil || *gotID != external {
		t.Errorf("source_id = %v, want %q", gotID, external)
	}
}

// The idempotency INDEX holds on its own, without the capture key in front of
// it.
//
// The test above passes even with the source key dropped from the insert,
// because replayedActivity short-circuits a duplicate capture before the
// transition writer runs. Two guards where one is never exercised is one
// guard: this reaches past the capture key and asks the index directly, so a
// connector path that ever writes a transition without re-logging the activity
// still cannot double a booking.
func TestTheSourceKeyRefusesASecondTransitionOnItsOwn(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Directly replayed", time.Now().Add(24*time.Hour))
	const system, external = "google_calendar", "evt-direct-001"

	insert := func() error {
		_, err := e.owner.Exec(context.Background(), `
			INSERT INTO activity_meeting_history
			    (activity_id, status, effective_at, scheduled_start, actor, source_system, source_id)
			VALUES ($1, 'held', now(), now(), 'system:connector', $2, $3)`,
			id.UUID, system, external)
		return err
	}
	if err := insert(); err != nil {
		t.Fatalf("the first connector transition was refused: %v", err)
	}
	if err := insert(); err == nil {
		t.Error("the same connector event was recorded twice; the unique index is what makes a resync safe")
	}
}

// One calendar event keeps ONE external id for its whole life, so the booking
// and the cancellation of a meeting arrive under the same key.
//
// A key of (source_system, source_id) alone would admit the first and refuse
// the second — and because the writer treats a duplicate as already-recorded,
// the cancellation would vanish with no error while the activity column moved
// to canceled. The history and the column would then disagree, permanently.
func TestOneCalendarEventCanRecordTwoDifferentTransitions(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Booked then called off", time.Now().Add(24*time.Hour))
	const system, external = "google_calendar", "evt-lifecycle-001"

	record := func(status string) error {
		_, err := e.owner.Exec(context.Background(), `
			INSERT INTO activity_meeting_history
			    (activity_id, status, effective_at, scheduled_start, actor, source_system, source_id)
			VALUES ($1, $2, now(), now(), 'system:connector', $3, $4)`,
			id.UUID, status, system, external)
		return err
	}
	if err := record("booked"); err != nil {
		t.Fatalf("the connector booking was refused: %v", err)
	}
	if err := record("canceled"); err != nil {
		t.Fatalf("the connector cancellation was refused: %v — one event's booking and cancellation "+
			"share an external id, and both are real transitions", err)
	}
	// And the replay of either is still refused.
	if err := record("canceled"); err == nil {
		t.Error("the same cancellation was recorded twice; a resync must stay idempotent")
	}
}

// Rows the migration backfilled say what a meeting IS and refuse to say when it
// became that. A period count that treated them as history would report
// bookings on the day of the deploy.
func TestABackfilledRowClaimsNoBookingMoment(t *testing.T) {
	e := setupSend(t)
	// Written the way the migration writes one: directly, marked partial. A
	// meeting that existed before the table did has no recoverable booking
	// instant, and inventing one is the failure this flag exists to prevent.
	id := ids.New[ids.ActivityKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'Pre-history meeting', now() - interval '30 days', 'held', 'manual', 'human:x')`,
		id.UUID); err != nil {
		t.Fatalf("seeding a pre-history meeting: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_meeting_history
		    (activity_id, status, effective_at, scheduled_start, actor, partial_pre_history)
		SELECT a.id, a.meeting_status, a.created_at, a.occurred_at, 'system:migration', true
		FROM activity a WHERE a.id = $1`, id.UUID); err != nil {
		t.Fatalf("seeding the baseline row: %v", err)
	}

	history := historyOf(t, e, id)
	if len(history) != 1 {
		t.Fatalf("recorded %d rows, want 1 baseline", len(history))
	}
	if !history[0].PartialPreHistory {
		t.Error("the baseline row is not marked partial; a period count would read it as a real transition")
	}
	if history[0].Status != "held" {
		t.Errorf("baseline status = %q, want the meeting's current held", history[0].Status)
	}
}

// Both doors that can set a status record one, and they record the same shape.
// A door that wrote the column without the history would answer every period
// question short, and no existing test would notice.
func TestBothMeetingStatusDoorsRecordHistory(t *testing.T) {
	e := setupSend(t)

	// Door one: the insert, which sets a status at capture.
	created := bookMeeting(t, e, "Via create", time.Now().Add(24*time.Hour))
	if got := historyOf(t, e, created); len(got) != 1 {
		t.Fatalf("the create door recorded %d transitions, want 1", len(got))
	}

	// Door two: the update, on a meeting that arrived with no status at all.
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind:       string(crmcontracts.ActivityKindMeeting),
		Subject:    ptr("Statusless at capture"),
		OccurredAt: ptr(time.Now().Add(48 * time.Hour)),
		Source:     "manual",
	})
	if err != nil {
		t.Fatalf("capturing a statusless meeting: %v", err)
	}
	statusless := ids.From[ids.ActivityKind](ids.UUID(out.Id))
	if got := historyOf(t, e, statusless); len(got) != 0 {
		t.Fatalf("a capture naming no status recorded %d transitions, want 0", len(got))
	}
	moveTo(t, e, statusless, crmcontracts.ActivityMeetingStatusBooked)
	if got := historyOf(t, e, statusless); len(got) != 1 {
		t.Fatalf("the update door recorded %d transitions, want 1", len(got))
	}
}

// A transition never outlives the meeting it belongs to. Erasure and hard
// delete both reach it through the FK rather than leaving orphan history that
// still says a person met somebody.
func TestHistoryDiesWithItsMeeting(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "To be deleted", time.Now().Add(24*time.Hour))
	if got := historyOf(t, e, id); len(got) != 1 {
		t.Fatalf("recorded %d transitions before the delete, want 1", len(got))
	}
	if _, err := e.owner.Exec(context.Background(), `DELETE FROM activity WHERE id = $1`, id.UUID); err != nil {
		t.Fatalf("deleting the meeting: %v", err)
	}
	if got := historyOf(t, e, id); len(got) != 0 {
		t.Errorf("%d transitions outlived their meeting; the history would still say it happened", len(got))
	}
}

// A transaction that fails after the transition rolls it back with everything
// else: the history is part of the write, not a note beside it.
func TestATransitionRollsBackWithItsWrite(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Rolled back", time.Now().Add(24*time.Hour))
	before := len(historyOf(t, e, id))

	// A patch that the row refuses: a version that does not match.
	stale := int64(-1)
	held := string(crmcontracts.ActivityMeetingStatusHeld)
	if _, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{
		MeetingStatus: &held, IfVersion: &stale,
	}); err == nil {
		t.Fatal("the stale-version patch was accepted; this fixture cannot test a rollback")
	}
	if got := len(historyOf(t, e, id)); got != before {
		t.Errorf("history grew from %d to %d across a refused write", before, got)
	}
}

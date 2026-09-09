// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The one-off pass that closes meetings captured before the calendar's RSVP was
// read.
//
// The fixtures are the shape found on a real installation: of 467 captured
// Google meetings, twelve stored originals carried a decline nobody had read —
// seven where the calendar OWNER declined (genuinely stale rows, all reading as
// booked) and five where only a guest did (correctly still on). Both arms are
// modelled here, because a pass that closed the second kind would take meetings
// a rep is actually attending off their schedule.
//
// Every meeting is seeded THROUGH THE REAL SINK, which is what writes the
// raw_capture row this pass re-reads. Hand-inserting the payload would prove the
// pass works against bytes nothing in production produces.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// backfillOwner is the connected calendar's own address — the seat whose RSVP
// decides whether a meeting is off.
const backfillOwner = "rep@myco.test"

// attendee is one RSVP line as Google states it.
type attendee struct {
	email    string
	response string
	self     bool
}

// calendarPayload builds a Calendar v3 event resource: confirmed, external, and
// carrying whatever RSVPs the case needs. Confirmed on purpose — a DECLINED
// event is still `confirmed` to Google, which is exactly why the decline was
// missed for so long.
func calendarPayload(t *testing.T, eventID string, starts time.Time, attendees ...attendee) []byte {
	t.Helper()
	rows := make([]map[string]any, 0, len(attendees))
	for _, a := range attendees {
		row := map[string]any{"email": a.email, "responseStatus": a.response}
		if a.self {
			row["self"] = true
		}
		rows = append(rows, row)
	}
	raw, err := json.Marshal(map[string]any{
		"id":        eventID,
		"status":    "confirmed",
		"summary":   "Consulting Monthly",
		"start":     map[string]string{"dateTime": starts.Format(time.RFC3339)},
		"organizer": map[string]string{"email": "client@acme.test"},
		"attendees": rows,
	})
	if err != nil {
		t.Fatalf("marshalling the calendar payload: %v", err)
	}
	return raw
}

// seedCapturedMeeting lands one meeting through the real Sink WITHOUT the cancel
// seam — the Sink as it stood before this change, which is what left the stale
// rows this pass exists to clear.
func seedCapturedMeeting(t *testing.T, e *integration.Env, eventID string, starts time.Time, attendees ...attendee) ids.UUID {
	t.Helper()
	ref, err := capture.NewSink(e.DB()).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: eventID},
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Consulting Monthly",
			Body: "Organizer: client@acme.test", OccurredAt: starts,
		},
		Source:     calendarSystem + ":" + eventID,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        calendarPayload(t, eventID, starts, attendees...),
	})
	if err != nil {
		t.Fatalf("seeding the captured meeting %s: %v", eventID, err)
	}
	return ref.ID
}

// seedCalendarConnection records the connection whose account_label the pass
// reads to learn whose calendar a meeting came off. Without it the pass has no
// owner and settles every meeting as unreadable — which is a real behaviour, and
// the reason this is seeded explicitly rather than assumed.
func seedCalendarConnection(t *testing.T, e *integration.Env) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, status, account_label)
			VALUES ($1, $2, 'connected', $3)`, calendarSystem, e.AdminUser, backfillOwner)
		return err
	}); err != nil {
		t.Fatalf("seeding the calendar connection: %v", err)
	}
}

// backfillOutcome reads what the pass concluded about one meeting.
func backfillOutcome(t *testing.T, e *integration.Env, id ids.UUID) (string, bool) {
	t.Helper()
	var outcome *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT outcome FROM activity_meeting_rsvp_backfill WHERE activity_id = $1`, id).Scan(&outcome)
	}); err != nil {
		if err.Error() == "no rows in result set" {
			return "", false
		}
		t.Fatalf("reading the backfill outcome: %v", err)
	}
	if outcome == nil {
		return "", false
	}
	return *outcome, true
}

// runBackfill drains the pass and answers how many meetings it settled.
func runRSVPBackfill(t *testing.T, e *integration.Env) int {
	t.Helper()
	n, err := backfillMeetingRSVPBatch(e.Admin(), e.Pool, meetingRSVPBackfillPerTick, slog.Default())
	if err != nil {
		t.Fatalf("running the rsvp backfill: %v", err)
	}
	return n
}

// The case from the field, and the one the report was about: the owner declined,
// nothing read it, and the meeting has been reading as booked ever since.
func TestTheBackfillClosesAMeetingTheOwnerHadDeclined(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	starts := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	id := seedCapturedMeeting(t, e, "evt-declined", starts,
		attendee{email: backfillOwner, response: "declined", self: true},
		attendee{email: "client@acme.test", response: "accepted"})

	// The state every one of these rows is in: captured, and reading as booked.
	if status, set := readMeetingStatus(t, e, id); set {
		t.Fatalf("the seeded meeting carries %q; the stale rows carry no status at all", status)
	}

	if n := runRSVPBackfill(t, e); n == 0 {
		t.Fatal("the backfill settled nothing, so the stale meeting was never offered")
	}
	if status, set := readMeetingStatus(t, e, id); !set || status != "canceled" {
		t.Errorf("meeting_status = %q (set=%v), want canceled", status, set)
	}
	if outcome, ok := backfillOutcome(t, e, id); !ok || outcome != rsvpBackfillClosed {
		t.Errorf("outcome = %q (recorded=%v), want %q", outcome, ok, rsvpBackfillClosed)
	}
}

// A GUEST declining is not the owner declining. Five of the twelve declines on
// the real installation were this, and closing them would take meetings the rep
// is attending off their schedule.
func TestTheBackfillLeavesAMeetingOnlyAGuestDeclined(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	starts := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	id := seedCapturedMeeting(t, e, "evt-guest-declined", starts,
		attendee{email: backfillOwner, response: "accepted", self: true},
		attendee{email: "client@acme.test", response: "declined"})

	runRSVPBackfill(t, e)

	if status, set := readMeetingStatus(t, e, id); set {
		t.Errorf("meeting_status = %q, want the meeting left on — only a guest declined", status)
	}
	if outcome, ok := backfillOutcome(t, e, id); !ok || outcome != rsvpBackfillStillOn {
		t.Errorf("outcome = %q (recorded=%v), want %q", outcome, ok, rsvpBackfillStillOn)
	}
}

// A meeting somebody ANSWERED for keeps their answer, and the pass says so
// rather than reporting it closed. The writer refuses it; this proves the pass
// records that refusal honestly instead of counting it as a cancellation.
func TestTheBackfillKeepsAnAnsweredOutcome(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	starts := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	id := seedCapturedMeeting(t, e, "evt-already-held", starts,
		attendee{email: backfillOwner, response: "declined", self: true},
		attendee{email: "client@acme.test", response: "accepted"})
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET meeting_status = 'held' WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("recording the outcome: %v", err)
	}

	runRSVPBackfill(t, e)

	if status, _ := readMeetingStatus(t, e, id); status != "held" {
		t.Errorf("meeting_status = %q, want the recorded held kept", status)
	}
	if outcome, ok := backfillOutcome(t, e, id); !ok || outcome != rsvpBackfillAnswered {
		t.Errorf("outcome = %q (recorded=%v), want %q", outcome, ok, rsvpBackfillAnswered)
	}
}

// The pass DRAINS. Every meeting it judges is marked, including the ones it
// leaves alone, so a second run offers nothing — without that a workspace full
// of still-on meetings would be re-read on every tick forever.
func TestTheBackfillDrainsAndDoesNotRepeatItself(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	starts := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	for i := range 5 {
		seedCapturedMeeting(t, e, fmt.Sprintf("evt-still-on-%d", i), starts,
			attendee{email: backfillOwner, response: "accepted", self: true},
			attendee{email: "client@acme.test", response: "accepted"})
	}
	seedCapturedMeeting(t, e, "evt-off", starts,
		attendee{email: backfillOwner, response: "declined", self: true},
		attendee{email: "client@acme.test", response: "accepted"})

	if first := runRSVPBackfill(t, e); first != 6 {
		t.Fatalf("the first pass settled %d meetings, want all 6", first)
	}
	if second := runRSVPBackfill(t, e); second != 0 {
		t.Errorf("the second pass settled %d meetings, want 0 — a settled meeting must not be offered again", second)
	}
}

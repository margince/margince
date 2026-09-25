// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the attendee repair says about a meeting it could not bind anybody on.
//
// Two different facts reach that point and the pass used to record both as
// `none`: an invitation that named no further party, which is finished work,
// and one the party cap refused, where real colleagues are still locked out of
// a meeting they attended. A pass that called both `none` reported itself
// complete over the second, and nothing in the data said otherwise.

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

// The Graph side of the fixtures. Its own owner address because the connection
// row is keyed per provider, and its own seeder because a Graph meeting is a
// different stored shape — which is the point of exercising it at all.
const graphCalOwner = "rep@myco.test"

func seedGraphCalendarConnection(t *testing.T, e *integration.Env) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, status, account_label)
			VALUES ('graphcal', $1, 'connected', $2)`, e.AdminUser, graphCalOwner)
		return err
	}); err != nil {
		t.Fatalf("seeding the Graph calendar connection: %v", err)
	}
}

// seedGraphCalMeeting lands one meeting through the real Sink, the way the
// Google fixture beside it does.
func seedGraphCalMeeting(t *testing.T, e *integration.Env, eventID string, raw []byte) ids.UUID {
	t.Helper()
	ref, err := capture.NewSink(e.DB()).Upsert(
		connectorOwnerCtx(e, e.AdminUser, "graphcal"), connector.NormalizedRecord{
			EntityType: "activity",
			NaturalKey: connector.NaturalKey{SourceSystem: "graphcal", SourceID: eventID},
			Fields: capture.ActivityFields{
				Kind: "meeting", Subject: "All hands", OccurredAt: time.Now().Add(-time.Hour),
			},
			Source:     "graphcal:" + eventID,
			CapturedBy: "connector:graphcal",
			Raw:        raw,
		})
	if err != nil {
		t.Fatalf("seeding the Graph meeting: %v", err)
	}
	return ref.ID
}

// repairOutcome reads what the repair concluded about one meeting.
//
// "the pass recorded nothing" is an answer these cases assert on; a failed read
// is not, and folding the two together would let a broken query look exactly
// like a pass that correctly skipped the meeting.
func repairOutcome(t *testing.T, e *integration.Env, id ids.UUID) (string, bool) {
	t.Helper()
	var outcome string
	var found bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT outcome, true FROM activity_meeting_attendee_repair WHERE activity_id = $1`,
			id).Scan(&outcome, &found)
	}); err != nil {
		return "", false
	}
	return outcome, found
}

// A meeting past the cap is recorded as capped, not as one that named nobody.
func TestTheRepairSaysCappedRatherThanFoundNone(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)

	crowd := make([]attendee, 0, connector.MaxParticipants+2)
	crowd = append(crowd, attendee{email: backfillOwner, response: "accepted", self: true})
	for i := range connector.MaxParticipants + 1 {
		crowd = append(crowd, attendee{
			email:    fmt.Sprintf("guest%d@list.example", i),
			response: "accepted",
		})
	}
	big := seedCapturedMeeting(t, e, "evt-crowd", time.Now().Add(-time.Hour), crowd...)

	if _, err := repairMeetingAttendeesBatch(
		e.Admin(), e.Pool, meetingAttendeeRepairPerTick, slog.Default()); err != nil {
		t.Fatalf("repairMeetingAttendeesBatch: %v", err)
	}
	outcome, found := repairOutcome(t, e, big)
	if !found {
		t.Fatal("the crowded meeting was never settled, so the pass will re-read it forever")
	}
	if outcome != repairCapped {
		t.Errorf("outcome = %q, want %q — recorded as %q it reads as a meeting that named "+
			"nobody, and the colleagues still locked out of it are invisible",
			outcome, repairCapped, outcome)
	}
}

// A meeting UNDER the cap still binds its attendees. The capped check runs
// before the empty one, so a Capped() that answered yes for a small list would
// record every meeting as capped — satisfying the case above while resolving
// nobody, which is the same defect wearing the new outcome's name.
func TestAMeetingUnderTheCapStillBindsItsAttendees(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)

	small := seedCapturedMeeting(t, e, "evt-small", time.Now().Add(-time.Hour),
		attendee{email: backfillOwner, response: "accepted", self: true},
		attendee{email: "buyer@acme.test", response: "accepted"})

	if _, err := repairMeetingAttendeesBatch(
		e.Admin(), e.Pool, meetingAttendeeRepairPerTick, slog.Default()); err != nil {
		t.Fatalf("repairMeetingAttendeesBatch: %v", err)
	}
	outcome, found := repairOutcome(t, e, small)
	if !found {
		t.Fatal("the meeting was never settled, so the pass will re-read it forever")
	}
	if outcome != repairBoundAttendees {
		t.Errorf("outcome = %q, want %q — two names is not a distribution list, and recording "+
			"it as anything else leaves everyone on it unbound", outcome, repairBoundAttendees)
	}
}

// The OTHER calendar provider. The repair reads two payload shapes and only
// Google's was ever exercised, so a Graph meeting past the cap went through the
// arm nothing had run — and `capped` is decided after that parse, so a shape it
// could not read would record `unreadable` and settle the meeting just the same.
func TestTheRepairReadsAGraphCalendarTooAndSaysCapped(t *testing.T) {
	e := integration.Setup(t)
	seedGraphCalendarConnection(t, e)

	crowd := make([]map[string]any, 0, connector.MaxParticipants+1)
	for i := range connector.MaxParticipants + 1 {
		crowd = append(crowd, map[string]any{
			"type": "required",
			"emailAddress": map[string]string{
				"address": fmt.Sprintf("guest%d@list.example", i),
			},
		})
	}
	payload, err := json.Marshal(map[string]any{
		"id":      "graph-crowd",
		"subject": "All hands",
		"start": map[string]string{
			"dateTime": time.Now().Add(-time.Hour).Format("2006-01-02T15:04:05"),
			"timeZone": "UTC",
		},
		"organizer": map[string]any{
			"emailAddress": map[string]string{"address": graphCalOwner},
		},
		"attendees": crowd,
	})
	if err != nil {
		t.Fatalf("marshalling the Graph event: %v", err)
	}
	big := seedGraphCalMeeting(t, e, "graph-crowd", payload)

	if _, err := repairMeetingAttendeesBatch(
		e.Admin(), e.Pool, meetingAttendeeRepairPerTick, slog.Default()); err != nil {
		t.Fatalf("repairMeetingAttendeesBatch: %v", err)
	}
	outcome, found := repairOutcome(t, e, big)
	if !found {
		t.Fatal("the Graph meeting was never settled, so the pass will re-read it forever")
	}
	if outcome != repairCapped {
		t.Errorf("outcome = %q, want %q — recorded as %q a Graph payload the parser could not "+
			"read is indistinguishable from one it read and refused",
			outcome, repairCapped, outcome)
	}
}

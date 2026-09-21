// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

// What CaptureOne DOES with each settlement — the wiring between the rules and
// the sink. Classify's own answers are proved in meetingmap_test.go; these prove
// the verb each answer reaches, which is what a stale booked row comes down to.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// recordingSink counts what reached each verb. It implements
// connector.MeetingCanceller as well as connector.Sink, which is what a
// composed Sink does.
type recordingSink struct {
	upserted  []connector.NormalizedRecord
	cancelled []connector.NaturalKey
	cancelAt  []time.Time
	cancelErr error
}

func (s *recordingSink) Upsert(_ context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	s.upserted = append(s.upserted, rec)
	return datasource.EntityRef{Type: datasource.EntityActivity}, nil
}

func (s *recordingSink) CancelMeeting(_ context.Context, key connector.NaturalKey, at time.Time) error {
	s.cancelled = append(s.cancelled, key)
	s.cancelAt = append(s.cancelAt, at)
	return s.cancelErr
}

// upsertOnlySink is a sink that cannot cancel — a fixture, or a deployment
// composed without the seam. It must still capture.
type upsertOnlySink struct{ upserted int }

func (s *upsertOnlySink) Upsert(context.Context, connector.NormalizedRecord) (datasource.EntityRef, error) {
	s.upserted++
	return datasource.EntityRef{Type: datasource.EntityActivity}, nil
}

// testEvent is the fixture vendor's wire shape: the neutral Event as JSON, so
// these tests exercise CaptureOne rather than any real provider's decode.
func testDecode(raw []byte, _ string) (Event, error) {
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return Event{}, err
	}
	return ev, nil
}

func rawEvent(t *testing.T, ev Event) []byte {
	t.Helper()
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshalling the fixture event: %v", err)
	}
	return raw
}

const captureConnector = "testcal"

var startsAt = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

// The defect this whole path exists for: a meeting called off, or declined,
// reaches the sink as a CANCELLATION. Before this, both returned nil and the row
// stayed booked forever.
func TestACancelledOrDeclinedEventCancelsTheCapturedMeeting(t *testing.T) {
	t.Parallel()
	external := []Actor{{Email: "client@acme.test"}}
	for name, ev := range map[string]Event{
		"cancelled": {
			ID: "evt-1", Cancelled: true, StartsAt: startsAt,
			Organizer: Actor{Email: owner}, Attendees: external,
		},
		"declined": {
			ID: "evt-1", OwnerDeclined: true, StartsAt: startsAt,
			Organizer: Actor{Email: owner}, Attendees: external,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sink := &recordingSink{}
			if err := CaptureOne(context.Background(), rawEvent(t, ev), sink, owner, captureConnector, testDecode); err != nil {
				t.Fatalf("CaptureOne: %v", err)
			}
			if len(sink.upserted) != 0 {
				t.Errorf("wrote %d records; a meeting that is off must not be written as booked", len(sink.upserted))
			}
			if len(sink.cancelled) != 1 {
				t.Fatalf("cancelled %d meetings, want exactly 1 — the row stays booked otherwise", len(sink.cancelled))
			}
			want := connector.NaturalKey{SourceSystem: captureConnector, SourceID: "evt-1"}
			if sink.cancelled[0] != want {
				t.Errorf("cancelled %+v, want %+v — the key must be the one the capture landed under", sink.cancelled[0], want)
			}
			// The meeting's own start, not the moment the sync noticed. A pull
			// running days later would otherwise date every cancellation to the
			// sync schedule.
			if !sink.cancelAt[0].Equal(startsAt) {
				t.Errorf("cancelled at %v, want the meeting's start %v", sink.cancelAt[0], startsAt)
			}
		})
	}
}

// A meeting still on is written, and nothing is cancelled.
func TestALiveEventIsCapturedAndCancelsNothing(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	ev := Event{
		ID: "evt-live", Subject: "Intro", StartsAt: startsAt,
		Organizer: Actor{Email: owner}, Attendees: []Actor{{Email: "client@acme.test"}},
	}
	if err := CaptureOne(context.Background(), rawEvent(t, ev), sink, owner, captureConnector, testDecode); err != nil {
		t.Fatalf("CaptureOne: %v", err)
	}
	if len(sink.upserted) != 1 {
		t.Fatalf("wrote %d records, want the meeting captured once", len(sink.upserted))
	}
	if len(sink.cancelled) != 0 {
		t.Errorf("cancelled %d meetings; a live meeting must cancel nothing", len(sink.cancelled))
	}
}

// An event that was never worth capturing is neither written nor cancelled.
// Cancelling it would be a lookup per internal meeting per sync, forever.
func TestADroppedEventReachesNeitherVerb(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	internal := Event{
		ID: "evt-standup", Subject: "Standup", StartsAt: startsAt,
		Organizer: Actor{Email: owner},
		Attendees: []Actor{{Email: owner}, {Email: "peer@myco.com"}},
	}
	if err := CaptureOne(context.Background(), rawEvent(t, internal), sink, owner, captureConnector, testDecode); err != nil {
		t.Fatalf("CaptureOne: %v", err)
	}
	if len(sink.upserted) != 0 || len(sink.cancelled) != 0 {
		t.Errorf("wrote %d and cancelled %d, want neither", len(sink.upserted), len(sink.cancelled))
	}
}

// A sink that cannot cancel behaves exactly as this package did before the verb
// existed: the event is dropped, and the pull carries on. Failing here would
// take out a whole calendar sync over a verb the deployment never wired.
func TestASinkThatCannotCancelStillSyncs(t *testing.T) {
	t.Parallel()
	sink := &upsertOnlySink{}
	off := Event{
		ID: "evt-1", Cancelled: true, StartsAt: startsAt,
		Organizer: Actor{Email: owner}, Attendees: []Actor{{Email: "client@acme.test"}},
	}
	if err := CaptureOne(context.Background(), rawEvent(t, off), sink, owner, captureConnector, testDecode); err != nil {
		t.Fatalf("CaptureOne: %v", err)
	}
	if sink.upserted != 0 {
		t.Errorf("wrote %d records, want none", sink.upserted)
	}
}

// A cancellation that FAILS stops the pull, exactly as a failed upsert does. The
// alternative is a watermark advancing past a cancellation nothing recorded,
// which no later pull would ever revisit.
func TestACancellationFaultStopsThePull(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("the database said no")
	sink := &recordingSink{cancelErr: wantErr}
	off := Event{
		ID: "evt-1", Cancelled: true, StartsAt: startsAt,
		Organizer: Actor{Email: owner}, Attendees: []Actor{{Email: "client@acme.test"}},
	}
	err := CaptureOne(context.Background(), rawEvent(t, off), sink, owner, captureConnector, testDecode)
	if !errors.Is(err, wantErr) {
		t.Fatalf("CaptureOne err = %v, want the fault to reach the caller", err)
	}
}

// A deliberate skip from the sink is not a fault. It is how the sink says the
// record is not this workspace's to write, and a pull must survive it.
func TestACancellationSkipIsNotAFault(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{cancelErr: connector.ErrSkip}
	off := Event{
		ID: "evt-1", Cancelled: true, StartsAt: startsAt,
		Organizer: Actor{Email: owner}, Attendees: []Actor{{Email: "client@acme.test"}},
	}
	if err := CaptureOne(context.Background(), rawEvent(t, off), sink, owner, captureConnector, testDecode); err != nil {
		t.Fatalf("CaptureOne err = %v, want a skip to be absorbed", err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The Sink's third calendar verb: a meeting this workspace already captured has
// been moved.
//
// A meeting is written once, under its calendar event's natural key, and a
// later pull of the same event was a replay that wrote nothing — so a meeting
// rescheduled in the calendar kept its old date. The replay path now moves it
// (moveOnReplay). An event moved past the calendar connector's capture horizon
// is not captured at all any more, so that pull reaches the row through
// MoveMeeting instead: it moves a meeting that exists and creates nothing.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MeetingMover sets a captured meeting's start and length to what the calendar
// now says, reporting whether anything changed. activities owns the write;
// compose injects it. Nil is a Sink that never moves a meeting — what it did
// before this existed.
type MeetingMover func(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, start time.Time, duration *int,
) (bool, error)

// WithMeetingMover returns a copy that moves a captured meeting when the
// calendar reschedules it.
func (s *Sink) WithMeetingMover(move MeetingMover) *Sink {
	c := *s
	c.moveMeeting = move
	return &c
}

// admitCalendarVerb holds the door a calendar verb carries no record through:
// the two rules admitRecord states for a write, about who is calling.
//
// First, a connector principal, which the registry mints and nothing else does
// — that is what keeps the verb off every other caller. Second, the provenance:
// the verb takes the source system as an argument and the natural key is what
// finds the row, so without it a connector for one provider could act on
// meetings captured by another — a Telegram or IMAP sync naming SourceSystem
// "gcal" would reach Google Calendar meetings it has no standing to touch.
func admitCalendarVerb(ctx context.Context, key connector.NaturalKey, verb string) error {
	if key.SourceSystem == "" || key.SourceID == "" {
		return fmt.Errorf("capture: to %s a meeting needs a natural key", verb)
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalConnector {
		return fmt.Errorf("capture: to %s a meeting requires a connector principal — the registry builds it, nothing else may", verb)
	}
	if want := connectorPrincipalID(key.SourceSystem); want != actor.ID {
		return fmt.Errorf(
			"capture: %q cannot %s a meeting captured by %q — a connector acts for its own provider and no other",
			actor.ID, verb, want)
	}
	return nil
}

// MoveMeeting moves the meeting captured under this natural key to a new start
// and length, satisfying connector.MeetingMover. A key naming no captured
// meeting writes nothing: an event beyond the horizon that was never captured
// stays uncaptured until it comes within it.
func (s *Sink) MoveMeeting(ctx context.Context, key connector.NaturalKey, start time.Time, duration *int) error {
	if s.moveMeeting == nil {
		return nil
	}
	if err := admitCalendarVerb(ctx, key, "move"); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.ActivityID
		err := tx.QueryRow(ctx, `
			SELECT id FROM activity
			 WHERE source_system = $1 AND source_id = $2
			   AND kind = 'meeting' AND restricted_at IS NULL AND archived_at IS NULL`,
			key.SourceSystem, key.SourceID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("capture: finding the meeting to move: %w", err)
		}
		_, err = s.moveMeeting(ctx, tx, id, start, duration)
		return err
	})
}

// moveOnReplay moves a meeting a calendar delivered again under its own key
// with a different start — a reschedule. Only a meeting, only with a start the
// provider stated: a zero start is one the calendar could not read, and the
// stored one is better than capture time. Read from the record as delivered,
// because captureActivity's own copy has a missing start filled with now.
//
// moveMeeting is the same seam WithMeetingMover carries, so the standing check
// travels with it here too: a replay reaches the row only when the acting seat
// already holds it.
func (s *Sink) moveOnReplay(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, viaIdentity bool,
) error {
	fields, ok := rec.Fields.(ActivityFields)
	if s.moveMeeting == nil || viaIdentity || !ok || fields.Kind != meetingKind || fields.OccurredAt.IsZero() {
		return nil
	}
	_, err := s.moveMeeting(ctx, tx, id, fields.OccurredAt, fields.DurationSeconds)
	return err
}

// settleMeetingReplay is what a proven replay of a meeting still owes the row:
// its current identity (reclaimMeetingIdentity) and its current start
// (moveOnReplay). The move is for a replay of THIS calendar's own key only; a
// row reached through the cross-door identity is another door's filing, and a
// second calendar still describing the old time would move it back.
func (s *Sink) settleMeetingReplay(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, viaIdentity bool,
) error {
	if err := s.reclaimMeetingIdentity(ctx, tx, id, rec); err != nil {
		return err
	}
	return s.moveOnReplay(ctx, tx, id, rec, viaIdentity)
}

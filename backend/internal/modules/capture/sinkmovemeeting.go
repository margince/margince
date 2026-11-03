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
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MeetingMover sets a captured meeting's start and length to what the calendar
// now says, reporting whether anything changed. activities owns the write;
// compose injects it. Nil is a Sink that never moves a meeting — what it did
// before this existed.
type MeetingMover func(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, slot connector.MeetingSlot,
) (bool, error)

// WithMeetingMover returns a copy that moves a captured meeting when the
// calendar reschedules it.
func (s *Sink) WithMeetingMover(move MeetingMover) *Sink {
	c := *s
	c.moveMeeting = move
	return &c
}

// MeetingFolder retires duplicate onto survivor when both hold one meeting and
// retiring it loses nothing, reporting whether it did. proven says the binding
// rule already tied the duplicate's seat to the survivor. compose injects it.
type MeetingFolder func(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID, proven bool) (bool, error)

// WithMeetingFolder returns a copy that folds a meeting it finds stored twice.
func (s *Sink) WithMeetingFolder(fold MeetingFolder) *Sink {
	c := *s
	c.foldMeeting = fold
	return &c
}

// MeetingHostKeeper names a calendar meeting's organizer as its host when the
// organizer is a colleague whose own calendar holds it. compose injects it.
type MeetingHostKeeper func(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID) error

// WithMeetingHostKeeper returns a copy that hands a meeting it replays to its
// organizer. A guest's calendar often syncs a meeting first and becomes its host.
func (s *Sink) WithMeetingHostKeeper(keep MeetingHostKeeper) *Sink {
	c := *s
	c.keepMeetingHost = keep
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
func (s *Sink) MoveMeeting(ctx context.Context, key connector.NaturalKey, slot connector.MeetingSlot) error {
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
		slot.Duration = StorableDuration(slot.Duration)
		_, err = s.moveMeeting(ctx, tx, id, slot)
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
	_, err := s.moveMeeting(ctx, tx, id, connector.MeetingSlot{
		Start: fields.OccurredAt, Duration: StorableDuration(fields.DurationSeconds), AllDay: fields.DateOnly,
		Agenda: fields.AgendaFingerprint,
	})
	return err
}

// settleMeetingReplay is what a proven replay of a meeting still owes the row:
// its current identity, its guests and its current start. Only a replay of this
// calendar's own key moves it. A row reached through the cross-door identity is
// another door's filing, and a second calendar on the old time would move it back.
func (s *Sink) settleMeetingReplay(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, viaIdentity bool,
) error {
	if err := s.reclaimMeetingIdentity(ctx, tx, id, rec); err != nil {
		return err
	}
	if err := s.stampLaterGuests(ctx, tx, id, rec); err != nil {
		return err
	}
	if err := s.moveOnReplay(ctx, tx, id, rec, viaIdentity); err != nil {
		return err
	}
	if s.keepMeetingHost == nil {
		return nil
	}
	return s.keepMeetingHost(ctx, tx, id)
}

// foldOwnDuplicate answers the row this capture is about when the identity
// resolved to known (alreadyFiled) while this seat's own key also holds a live meeting. The
// own row folds onto known. If a human worked it, known folds onto the own row
// instead, and the own row moves to the record's start. When neither retires
// cleanly both stay, and the pair is logged.
func (s *Sink) foldOwnDuplicate(
	ctx context.Context, tx pgx.Tx, known ids.ActivityID, alreadyFiled bool, rec connector.NormalizedRecord,
) (ids.ActivityID, error) {
	if !alreadyFiled || s.foldMeeting == nil || !rec.CrossDoorIdentity.Stated() {
		return known, nil
	}
	own, found, err := ownLiveMeetingTx(ctx, tx, rec.NaturalKey, actorUserID(ctx))
	if err != nil || !found || own == known {
		return known, err
	}
	if folded, err := s.foldMeeting(ctx, tx, own, known, true); err != nil || folded {
		return known, err
	}
	folded, err := s.foldMeeting(ctx, tx, known, own, false)
	if err != nil {
		return known, err
	}
	if !folded {
		slog.WarnContext(ctx, "capture: a meeting stays stored twice because neither row retires cleanly",
			"kept", own.String(), "holder", known.String())
		return known, nil
	}
	return own, s.moveOnReplay(ctx, tx, own, rec, false)
}

// ownLiveMeetingTx is the live meeting this seat's calendar stored under the
// record's own natural key.
func ownLiveMeetingTx(ctx context.Context, tx pgx.Tx, key connector.NaturalKey, seat ids.UUID) (ids.ActivityID, bool, error) {
	var own ids.ActivityID
	err := tx.QueryRow(ctx, `
		SELECT id FROM activity
		 WHERE source_system = $1 AND source_id = $2 AND kind = 'meeting'
		   AND archived_at IS NULL AND restricted_at IS NULL
		   AND `+auth.ConnectorSeatSQL("activity")+` = $3::text`,
		key.SourceSystem, key.SourceID, seat.String()).Scan(&own)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("capture: finding this calendar's own meeting row: %w", err)
	}
	return own, true, nil
}

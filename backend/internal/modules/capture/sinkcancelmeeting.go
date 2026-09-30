// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The Sink's second calendar verb: a meeting this workspace already captured is
// called off.
//
// Capture's usual answer to an event it does not want is to write nothing, and
// for an event never captured that is right. It is wrong for one already on the
// timeline: a cancelled meeting keeps its place on the reader's schedule, the
// provider stops listing it, and no later pull mentions it again — so the row
// stands as booked for good. This is the write that closes it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MeetingCloser marks a captured meeting cancelled, keyed by the natural key
// the capture landed under. The activities module owns the activity table and
// its status history, so it owns this write; compose injects it, because capture
// never imports a sibling — the same shape AudienceRecomputer and
// ParticipantNamer travel on, and for the same reason.
//
// It answers whether it found a meeting to cancel. False is the ordinary case
// rather than a fault: most cancelled events were never worth capturing, so
// there is nothing under the key.
//
// Nil is a Sink that captures meetings and cancels none — what every fixture is
// until it says otherwise, and what a deployment composing capture without the
// timeline gets.
type MeetingCloser func(
	ctx context.Context, tx pgx.Tx, key connector.NaturalKey, at time.Time,
) (ids.ActivityID, bool, error)

// MeetingCloserByID marks one known meeting cancelled: the row a calendar event
// resolved to through its cross-door identity. activities owns the write, as it
// owns MeetingCloser's; `key` is the calendar event's own natural key, carried
// as the transition's idempotency key.
type MeetingCloserByID func(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, key connector.NaturalKey, at time.Time,
) (bool, error)

// WithMeetingCloser returns a copy that closes a captured meeting when the
// calendar says it is off — by its natural key, and, when nothing was captured
// under that key, by the row its cross-door identity resolves to.
func (s *Sink) WithMeetingCloser(closeMeeting MeetingCloser, closeByID MeetingCloserByID) *Sink {
	c := *s
	c.cancelMeeting = closeMeeting
	c.cancelMeetingByID = closeByID
	return &c
}

// CancelMeeting marks the meeting captured under this natural key as cancelled,
// satisfying connector.MeetingCanceller.
//
// Idempotent in both directions. A key naming no captured meeting writes
// nothing, and so does a meeting already cancelled — the writer decides both,
// under the row's own lock, because deciding here would read a state that could
// change before the write.
//
// A Sink composed without the seam does nothing and says so by succeeding: the
// connector's alternative is to fail a whole calendar pull over a verb this
// deployment never wired.
func (s *Sink) CancelMeeting(ctx context.Context, key connector.NaturalKey, at time.Time) error {
	return s.CancelIdentifiedMeeting(ctx, key, connector.CrossDoorIdentity{}, at)
}

// CancelIdentifiedMeeting is CancelMeeting for an event that also states its
// cross-door identity, satisfying connector.IdentifiedMeetingCanceller.
//
// The natural key answers first, exactly as CancelMeeting does. Only when
// nothing was captured under it — the event was matched onto a meeting another
// door filed, so capture wrote no row of its own — does the identity resolve
// the row, through the SAME bindable resolver capture matched it with: a row
// held by another seat, or no longer live, answers not-found here as it did
// there, so this cancels only the meeting this calendar's own capture joined.
func (s *Sink) CancelIdentifiedMeeting(
	ctx context.Context, key connector.NaturalKey, identity connector.CrossDoorIdentity, at time.Time,
) error {
	if s.cancelMeeting == nil {
		return nil
	}
	if err := admitCalendarVerb(ctx, key, "cancel"); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		id, _, err := s.cancelMeeting(ctx, tx, key, at)
		if err != nil || !id.IsZero() || s.cancelMeetingByID == nil {
			// An error, a row found under the key (cancelled or already
			// answered), or no by-identity seam: the natural key had the word.
			return err
		}
		fallback, err := s.identityMayCancel(ctx, tx, key)
		if err != nil || !fallback {
			return err
		}
		matched, found, err := s.activityHoldingIdentity(ctx, tx, connector.NormalizedRecord{
			NaturalKey: key, CrossDoorIdentity: identity,
		})
		if err != nil || !found {
			return err
		}
		_, err = s.cancelMeetingByID(ctx, tx, matched, key, at)
		return err
	})
}

// identityMayCancel reports whether a cancellation may go past its natural key
// to the meeting its identity resolves to. Two conditions, each closing a way
// the fallback would reach a row it should not:
//
//   - The key names a CALENDAR provider. The provenance check above binds the
//     key to the acting connector, but any connector may state its own key; a
//     mail or chat connector has no calendar event to cancel, and letting it
//     state an identity would let it close meetings by UID. The calendar
//     providers are the ones calendarWriteScopes names.
//   - NO row exists under the key, in any state. The natural-key cancel skips
//     an archived or restricted row by design; that row is still this event's
//     meeting, and it being retired is not a reason to go and cancel another.
func (s *Sink) identityMayCancel(ctx context.Context, tx pgx.Tx, key connector.NaturalKey) (bool, error) {
	if _, calendar := calendarWriteScopes[key.SourceSystem]; !calendar {
		return false, nil
	}
	var captured bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity WHERE source_system = $1 AND source_id = $2)`,
		key.SourceSystem, key.SourceID).Scan(&captured); err != nil {
		return false, fmt.Errorf("capture: is anything captured under this calendar event: %w", err)
	}
	return !captured, nil
}

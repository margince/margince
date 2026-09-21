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
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// WithMeetingCloser returns a copy that closes a captured meeting when the
// calendar says it is off.
func (s *Sink) WithMeetingCloser(closeMeeting MeetingCloser) *Sink {
	c := *s
	c.cancelMeeting = closeMeeting
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
	if s.cancelMeeting == nil {
		return nil
	}
	if key.SourceSystem == "" || key.SourceID == "" {
		return fmt.Errorf("capture: cancelling a meeting needs a natural key")
	}
	// Both halves of the door admitRecord holds for a write, asked directly:
	// this verb carries no record to admit, and the two rules it needs are the
	// ones about who is calling.
	//
	// First, a connector principal, which the registry mints and nothing else
	// does — that is what keeps the verb off every other caller.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalConnector {
		return errors.New("capture: cancelling a meeting requires a connector principal — the registry builds it, nothing else may")
	}
	// Second, the provenance, which is the same rule admitRecord states as
	// "a connector cannot claim to be another one" — and the reason it binds
	// HERE too is that this verb takes the source system as an argument. The
	// natural key is what finds the row, so without this check a connector
	// principal for one provider could close meetings captured by another: a
	// Telegram or IMAP sync naming SourceSystem "gcal" would cancel Google
	// Calendar meetings it has no standing to touch.
	if want := connectorPrincipalID(key.SourceSystem); want != actor.ID {
		return fmt.Errorf(
			"capture: %q cannot cancel a meeting captured by %q — a connector acts for its own provider and no other",
			actor.ID, want)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, _, err := s.cancelMeeting(ctx, tx, key, at)
		return err
	})
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MoveOne hands one raw event to the sink as a MOVE rather than a capture: the
// meeting captured earlier under its natural key takes the event's current
// start and length, and nothing new is written.
//
// It is how a connector treats an event it will not capture — one past its
// capture horizon. The meeting may have been captured while it was nearer and
// moved out since; leaving it would keep a meeting on the schedule at a date
// it no longer has. A parse failure, or a sink that cannot move, is a no-op,
// the same rule CaptureOne follows.
func MoveOne(ctx context.Context, raw []byte, sink connector.Sink, owner, connectorName string, decode Decode) error {
	mover, ok := sink.(connector.MeetingMover)
	if !ok {
		return nil
	}
	ev, err := decode(raw, owner)
	if err != nil {
		return nil //nolint:nilerr // a single unparseable event is a skip, not a fatal pull error (mirrors CaptureOne)
	}
	m := Classify(ev, owner)
	key := connector.NaturalKey{SourceSystem: connectorName, SourceID: m.id}
	if key.SourceID == "" {
		return nil
	}
	if err := mover.MoveMeeting(ctx, key, m.occurredAt, m.durationSeconds); err != nil && !errors.Is(err, connector.ErrSkip) {
		return err
	}
	return nil
}

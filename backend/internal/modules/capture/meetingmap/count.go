// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"context"
	"errors"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The outcomes an attempted event ends on besides a Settle reason. A drop is
// counted as "dropped_" plus its reason's label. A cancellation handed to the
// writer is counted as its reason's label, whether or not a meeting was there
// to close.
const (
	OutcomeCaptured          = "captured"
	OutcomeUnreadable        = "unreadable"
	OutcomeSinkSkipped       = "sink_skipped"
	OutcomeFailed            = "failed"
	OutcomeBeyondHorizon     = "beyond_horizon"
	OutcomeCancelUnsupported = "cancel_unsupported"
	OutcomeMoveUnsupported   = "move_unsupported"
)

// CountListed counts each provider batch before processing. The gap to attempted
// outcomes includes in-flight work and items left after a failed write.
func CountListed(connectorName string, events int) {
	capturemetrics.ObserveCalendarListed(connectorName, events)
}

// settleWrite counts what one write to the sink came to. A skip lets the pull
// go on, and any other error stops it.
func settleWrite(ctx context.Context, connectorName, eventID, success string, err error) error {
	switch {
	case err == nil:
		countEvent(ctx, connectorName, eventID, success)
		return nil
	case errors.Is(err, connector.ErrSkip):
		countEvent(ctx, connectorName, eventID, OutcomeSinkSkipped)
		return nil
	default:
		countEvent(ctx, connectorName, eventID, OutcomeFailed)
		return err
	}
}

// countEvent counts one attempted event's outcome, and names the event in a
// debug line. An unreadable event has no id to name.
func countEvent(ctx context.Context, connectorName, eventID, outcome string) {
	capturemetrics.ObserveCalendarEvent(connectorName, outcome)
	slog.DebugContext(ctx, "calendar event settled", "provider", connectorName, "event_id", eventID, "outcome", outcome)
}

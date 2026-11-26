// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// MeetingNotesReading is the enqueue a role sets as CaptureConfig's
// TranscriptEnqueue. The notes land inside a sync's own transaction, so an
// insert-only client is all it needs.
func MeetingNotesReading(log *slog.Logger) (activities.TranscriptReadEnqueue, error) {
	inserter, err := jobs.NewTxInserter(log)
	if err != nil {
		return nil, fmt.Errorf("compose: the meeting-notes reading enqueue: %w", err)
	}
	return TranscriptReadOnLanding(inserter), nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestALaneIsCountedByWhyItWasDropped(t *testing.T) {
	t.Parallel()
	left, leave := context.WithCancel(t.Context())
	leave()
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		err    error
		reason string
	}{
		{"a statement past its budget", t.Context(), fmt.Errorf("lane: %w", &pgconn.PgError{Code: "57014"}), LaneTimedOut},
		{"a deadline that ran out", t.Context(), context.DeadlineExceeded, LaneTimedOut},
		{"a caller who left", left, &pgconn.PgError{Code: "57014"}, LaneCallerLeft},
		{"any other error", t.Context(), errors.New("connection reset"), LaneFailed},
		{"a refusal", t.Context(), apperrors.ErrPermissionDenied, ""},
		{"a lane that answered", t.Context(), nil, ""},
	} {
		reason, dropped := laneFailureReason(tc.ctx, tc.err)
		if reason != tc.reason || dropped != (tc.reason != "") {
			t.Errorf("%s: reason %q dropped %v, want %q", tc.name, reason, dropped, tc.reason)
		}
	}
}

// failingTail is a task backlog whose tail read fails.
type failingTail struct{ windowRecorder }

func (failingTail) TasksAfter(context.Context, TaskWindow, string, int) ([]Positioned[Task], bool, error) {
	return nil, false, errors.New("connection reset")
}

// Not parallel: the counters are process-wide, and a sequential test runs
// with no parallel test beside it, so the deltas are this test's alone.
func TestAFailedTailSourceIsCountedAndTimedAsItsOwnStage(t *testing.T) {
	svc := (&Service{}).WithTaskBacklog(&failingTail{})
	from := &backlogCursor{At: map[string]string{string(sourceTask): ""}}
	lane := LaneKey{Lane: "tail_" + string(sourceTask), Reason: LaneFailed}
	stage := StageKey{Read: ReadUnmetered, Stage: "tail_" + string(sourceTask)}
	droppedBefore, timedBefore := LaneFailures()[lane], StageTimings()[stage].Count()

	_, _, failed := svc.backlogTail(t.Context(), tailPage{scope: scopeMine, day: crmcontracts.Attention{AsOf: readInstant}}, 5, from, nil)

	if len(failed) != 1 || failed[0].Source != string(sourceTask) {
		t.Fatalf("the page named %+v, want the task source as failed", failed)
	}
	if got := LaneFailures()[lane] - droppedBefore; got != 1 {
		t.Errorf("the dropped tail counted %d times on /metrics, want 1", got)
	}
	if got := StageTimings()[stage].Count() - timedBefore; got != 1 {
		t.Errorf("the tail read timed %d runs of its stage, want 1", got)
	}
}

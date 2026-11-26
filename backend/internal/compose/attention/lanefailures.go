// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How often each degradable lane was dropped, kept for /metrics by lane and
// reason. The page names a dropped lane to its reader; this is the count an
// operator reads across every page.

import (
	"context"
	"errors"
	"maps"
	"sync"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The reasons a lane is dropped. A refusal is the caller's access, not a
// fault, so it is never counted.
const (
	LaneTimedOut   = "timeout"
	LaneCallerLeft = "caller_left"
	LaneFailed     = "failed"
)

// LaneKey names one series: the lane and why it was dropped. Lane is a name
// fixed in this package, never anything a request supplies.
type LaneKey struct{ Lane, Reason string }

var laneFailures = struct {
	mu     sync.Mutex
	counts map[LaneKey]int64
}{counts: map[LaneKey]int64{}}

// countLaneFailure counts err against lane when it dropped the lane.
func countLaneFailure(ctx context.Context, lane string, err error) {
	reason, dropped := laneFailureReason(ctx, err)
	if !dropped {
		return
	}
	laneFailures.mu.Lock()
	defer laneFailures.mu.Unlock()
	laneFailures.counts[LaneKey{Lane: lane, Reason: reason}]++
}

// laneFailureReason sorts a lane's error. A caller who left cancels the
// running statement too, so it is asked before the statement budget.
func laneFailureReason(ctx context.Context, err error) (string, bool) {
	switch {
	case err == nil, errors.Is(err, apperrors.ErrPermissionDenied):
		return "", false
	case httperr.CallerLeft(ctx):
		return LaneCallerLeft, true
	case storekit.IsQueryCanceled(err), errors.Is(err, context.DeadlineExceeded):
		return LaneTimedOut, true
	default:
		return LaneFailed, true
	}
}

// LaneFailures answers a copy of every dropped-lane count since process start.
func LaneFailures() map[LaneKey]int64 {
	laneFailures.mu.Lock()
	defer laneFailures.mu.Unlock()
	return maps.Clone(laneFailures.counts)
}

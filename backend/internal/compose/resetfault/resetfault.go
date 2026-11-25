// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package resetfault holds the answers a data reset gives when it stops before
// or after its work for a reason the admin can act on.
package resetfault

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// DrainTimedOut refuses a reset whose drain window expired with a job still
// running. It is raised before anything is deleted, so the deferred resume
// restores the fleet unchanged.
var DrainTimedOut error = drainTimedOut{}

type drainTimedOut struct{}

func (drainTimedOut) Error() string { return "data reset: a background job outlived the drain window" }

func (drainTimedOut) Unwrap() error { return apperrors.ErrConflict }

func (drainTimedOut) MessageFault() (code, message string) {
	return "reset_drain_timed_out", "Background jobs were still running, so nothing was deleted and the queues are running again. Run the reset again."
}

// ResumeFailed reports that the fleet pause a reset took could not be lifted.
// It outranks the reset's own answer: an admin told only to retry would wait on
// queues that never run. Completed says whether the reset had finished.
type ResumeFailed struct {
	Cause     error
	Completed bool
}

func (e ResumeFailed) Error() string {
	return fmt.Sprintf("data reset: resuming the queues: %v", e.Cause)
}

func (ResumeFailed) Unwrap() error { return apperrors.ErrConflict }

// MessageFault answers a reset whose queues could not be started again.
func (e ResumeFailed) MessageFault() (code, message string) {
	state := "The reset did not finish."
	if e.Completed {
		state = "The reset finished."
	}
	return "reset_queues_not_resumed", state + " The background job queues could not be started again and may still be paused." +
		" Run the reset again: it pauses and then resumes the queues. If that fails too, check that the database is reachable and restart the worker."
}

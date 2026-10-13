// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
)

// The sentences a stopped run carries. The error that stopped it can hold a
// driver or SQL message, so it goes to the log and never to the run.
const (
	CauseStopped    = "the import stopped on an error; approve it again to continue from its checkpoint"
	CauseOverBudget = "the import ran past its time budget; approve it again to continue from its checkpoint"
	// CauseValidationStopped is a test run that could not finish; nothing was
	// written, so the way on is a new upload.
	CauseValidationStopped = "the test run stopped on an error; upload the file again to start a new import"
	// CauseSuperseded is a run a newer test run of its source replaced. The
	// import-session migration writes the same text.
	CauseSuperseded = "a newer test run of this source replaced this one"
)

func stopCause(cause error) string {
	if errors.Is(cause, context.DeadlineExceeded) {
		return CauseOverBudget
	}
	return CauseStopped
}

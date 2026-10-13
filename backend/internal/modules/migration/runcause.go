// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The sentences a stopped run carries. The error that stopped it can hold a
// driver or SQL message, so it goes to the log and never to the run.
const (
	CauseStopped    = "the import stopped on an error; approve it again to continue from its checkpoint"
	CauseOverBudget = "the import ran past its time budget; approve it again to continue from its checkpoint"
	// CauseValidationStopped is matched, never written: a legacy row carries it
	// for a test run that stopped. The backfill and the sweep cancel such a run.
	CauseValidationStopped = "the test run stopped on an error; upload the file again to start a new import"
	// CauseSuperseded is a run a newer test run of its source replaced. The
	// import-session backfill writes it too, so the text never changes.
	CauseSuperseded = "a newer test run of this source replaced this one"
	// CauseSourceTaken is a stopped commit the import-session backfill released
	// because another run of its upload kept the file. Only the backfill writes it.
	CauseSourceTaken = "another run of this upload holds its file, so this one cannot resume; upload the file again to go on"
	// CauseReplacedByUndo is a report an undo of another run of its source
	// name closed: the undo needs the name, and the report wrote nothing.
	CauseReplacedByUndo = "an undo of another import of this source name closed this test run; upload the file again to import it"
	// CauseTestRunStopped is a draft whose test run lost its process; the
	// choices are kept, so the way on is to test again.
	CauseTestRunStopped = "the test run stopped before it finished and imported nothing; run the test again"
	// CausePermission is an attempt whose principal lost the access it was
	// queued with: a seat gone or read-only, a passport revoked, a grant taken.
	CausePermission = "whoever started this no longer has the access it needs; someone who does must start it again"
	// CauseStorageUnavailable is a commit on a worker that stores no files, so
	// it cannot read the upload.
	CauseStorageUnavailable = "the server running this import has no file storage, so it cannot read the upload; approve it again once storage is configured"
	// CauseDraftExpired is a draft or report left untouched past its time to live.
	CauseDraftExpired = "this import was left unfinished too long, so it was closed and its file deleted"
)

// RunStop stops an attempt with the sentence its run carries.
type RunStop struct {
	Cause string
	Err   error
}

func (s RunStop) Error() string { return s.Err.Error() }
func (s RunStop) Unwrap() error { return s.Err }

// StopCause is the one sentence a run carries for the error that stopped it,
// whichever of the walk or its job records the stop.
func StopCause(err error) string {
	var stop RunStop
	switch {
	case errors.As(err, &stop):
		return stop.Cause
	case errors.Is(err, context.DeadlineExceeded):
		return CauseOverBudget
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return CausePermission
	default:
		return CauseStopped
	}
}

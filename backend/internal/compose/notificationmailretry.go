// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/mailer"
)

// answerRelayFailure decides what one failed send costs.
//
// A relay that never took the message (mailer.NotTaken) frees the claims, and
// the returned cause puts the job on River's next rung; Work faults it. No relay holds a copy.
// first is released too, since the next rung claims the burst through it.
//
// Any other failure, or the last rung, is final. The cause goes onto each
// notice, and the cancelled job is counted dead in job health.
func (w *notificationMailWorker) answerRelayFailure(
	ctx context.Context, first notices.EmailAttempt, sent []approvalMailItem, err error, lastRung bool,
) error {
	if mailer.NotTaken(err) && !lastRung {
		carried := []notices.EmailAttempt{first}
		for _, item := range sent {
			carried = append(carried, item.attempt)
		}
		releaseErr := w.notices.ReleaseEmailAttempts(ctx, carried)
		if releaseErr == nil {
			w.log.WarnContext(ctx, "the relay did not take the notice; the next attempt sends it",
				"notice", first.Notice.ID, "carried", len(sent), "cause", err)
			return err
		}
		w.log.WarnContext(ctx, "the notice's claim could not be released for a retry",
			"notice", first.Notice.ID, "cause", releaseErr)
	}
	w.log.WarnContext(ctx, "the notice was attempted and did not go out",
		"notice", first.Notice.ID, "carried", len(sent), "cause", err)
	for _, item := range sent {
		w.recordMailFailure(ctx, item.attempt.Notice.ID, err.Error())
	}
	return river.JobCancel(jobs.FaultContext(ctx, err))
}

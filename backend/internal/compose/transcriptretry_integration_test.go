// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// tryTranscript queues one reading and runs the production worker over it once
// per brain, from try first on. It returns the reading and each try's error.
func (e *transcriptEnv) tryTranscript(t *testing.T, first int, brains ...completer) (activities.TranscriptRead, []error) {
	t.Helper()
	started, _, err := e.Activities.StartTranscriptReadQueued(e.ctx, e.activity, "human:"+e.Rep1.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	quiet, now := slog.New(slog.DiscardHandler), func() time.Time { return clocktest.Now(t) }
	args := TranscriptProposeArgs{Workspace: e.WS, ActivityID: e.activity.UUID, TranscriptReadID: started.ID, RequestedBy: "human:" + e.Rep1.String()}
	var errs []error
	for i, brain := range brains {
		worker := &transcriptProposeWorker{language: installationLanguage(e.Pool), activities: e.Activities, log: quiet}
		worker.proposer = NewTranscriptProposer(e.Pool, brain, e.svc, now, quiet)
		row := &rivertype.JobRow{Attempt: first + i, MaxAttempts: oneOffJobMaxAttempts}
		ctx, cancel := context.WithCancel(context.Background())
		if timeout, ok := brain.(*timingOutBrain); ok {
			timeout.cancel = cancel
		}
		errs = append(errs, worker.Work(ctx, &river.Job[TranscriptProposeArgs]{JobRow: row, Args: args}))
		cancel()
	}
	read, err := e.Activities.GetTranscriptRead(e.ctx, e.activity, started.ID)
	if err != nil {
		t.Fatalf("reading back the run record: %v", err)
	}
	return read, errs
}

// timingOutBrain is a model call the job's timeout cut off: the try's context
// is cancelled before the worker hands the reading back.
type timingOutBrain struct{ cancel context.CancelFunc }

func (b *timingOutBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.cancel()
	return model.Response{}, context.Canceled
}

// River repeats a failed try a second later, inside the claim's lease. The
// repeat must find a reading it can claim. Else it ends as success, and the
// rep watches "reading…" with no job behind it.
func TestATranscriptReadingWhoseModelCallFailedIsReadOnTheNextTry(t *testing.T) {
	e := setupTranscript(t)
	for name, failing := range map[string]completer{
		"model failed": cannedBrain{err: ai.ErrAllTiersFailed}, "try timed out": &timingOutBrain{},
	} {
		read, errs := e.tryTranscript(t, 1, failing, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)})
		if errs[0] == nil || errs[1] != nil || read.Status != activities.TranscriptReadDone {
			t.Errorf("%s: tries = %v, reading %q; want the fault for River, then done on the next try", name, errs, read.Status)
		}
	}
}

// The last try leaves no job to hand the reading back to. It closes the
// reading with a sentence, and the rep can press Read again.
func TestATranscriptReadingWhoseLastTryFailedSaysSo(t *testing.T) {
	e := setupTranscript(t)
	for brain, code := range map[completer]string{
		cannedBrain{err: ai.ErrAllTiersFailed}: activities.TranscriptFailureUnanswered,
		&timingOutBrain{}:                      activities.TranscriptFailureUnfinished,
	} {
		read, errs := e.tryTranscript(t, oneOffJobMaxAttempts, brain)
		if errs[0] == nil || read.Status != activities.TranscriptReadFailed || failureCode(read) != code {
			t.Errorf("err = %v, reading %q coded %q; want the fault on the job and the reading failed as %q",
				errs[0], read.Status, failureCode(read), code)
		}
	}
}

func failureCode(read activities.TranscriptRead) string {
	if read.Outcome == nil || read.Outcome.Failure == nil {
		return ""
	}
	return read.Outcome.Failure.Code
}

// The budget stop is not a failed try. Even on its last try the job sleeps
// until the next budget window, and the reading waits queued.
func TestATranscriptReadingToldNotNowSleepsInsteadOfFailing(t *testing.T) {
	e := setupTranscript(t)
	deferral := &ai.BudgetDeferralError{Task: ai.TaskTranscriptPropose, NextAttemptAt: clocktest.Now(t).Add(240 * time.Hour)}
	read, errs := e.tryTranscript(t, oneOffJobMaxAttempts, cannedBrain{err: deferral})
	if _, snoozed := errors.AsType[*river.JobSnoozeError](errs[0]); !snoozed || read.Status != activities.TranscriptReadQueued {
		t.Errorf("err = %v, reading %q; want a snooze, which spends no try, and the reading queued", errs[0], read.Status)
	}
}

// stolenClaimBrain fails the call after another claim took the reading, so
// the worker's release misses and its last-try close is refused.
type stolenClaimBrain struct{ steal func() }

func (b stolenClaimBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.steal()
	return model.Response{}, ai.ErrAllTiersFailed
}

// A last try that cannot close its reading keeps a job behind it: the job
// sleeps one lease rather than ending with the reading left running.
func TestALastTryThatCannotCloseItsReadingWaitsForIt(t *testing.T) {
	e := setupTranscript(t)
	brain := stolenClaimBrain{steal: func() {
		if _, err := e.owner.Exec(t.Context(), `UPDATE transcript_read SET started_at = started_at - interval '1 second'
			WHERE activity_id = $1 AND status = 'running'`, e.activity.UUID); err != nil {
			t.Errorf("taking the claim: %v", err)
		}
	}}
	read, errs := e.tryTranscript(t, oneOffJobMaxAttempts, brain)
	snooze, snoozed := errors.AsType[*river.JobSnoozeError](errs[0])
	if !snoozed || snooze.Duration != activities.TranscriptReadLease || read.Status != activities.TranscriptReadRunning {
		t.Errorf("err = %v, reading %q; want a snooze of one lease while the other claim holds the reading", errs[0], read.Status)
	}
}

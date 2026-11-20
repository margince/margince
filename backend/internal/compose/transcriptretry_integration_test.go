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
		ctx, cancel := context.WithCancel(context.Background())
		if hook, ok := brain.(*interruptedBrain); ok && hook.before == nil {
			hook.before = cancel
		}
		row := &rivertype.JobRow{Attempt: first + i, MaxAttempts: oneOffJobMaxAttempts}
		errs = append(errs, worker.Work(ctx, &river.Job[TranscriptProposeArgs]{JobRow: row, Args: args}))
		cancel()
	}
	read, err := e.Activities.GetTranscriptRead(e.ctx, e.activity, started.ID)
	if err != nil {
		t.Fatalf("reading back the run record: %v", err)
	}
	return read, errs
}

// interruptedBrain runs before, then fails the call with err. With no before,
// tryTranscript cancels the try's context: the job's timeout cut the call off.
type interruptedBrain struct {
	before func()
	err    error
}

func (b *interruptedBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.before()
	return model.Response{}, b.err
}

// River repeats a failed try a second later, inside the claim's lease. The
// repeat must find a reading it can claim, or "reading…" has no job behind it.
func TestATranscriptReadingWhoseModelCallFailedIsReadOnTheNextTry(t *testing.T) {
	e := setupTranscript(t)
	for name, failing := range map[string]completer{
		"model failed": cannedBrain{err: ai.ErrAllTiersFailed}, "try timed out": &interruptedBrain{err: context.Canceled},
	} {
		read, errs := e.tryTranscript(t, 1, failing, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)})
		if errs[0] == nil || errs[1] != nil || read.Status != activities.TranscriptReadDone {
			t.Errorf("%s: tries = %v, reading %q; want the fault for River, then done on the next try", name, errs, read.Status)
		}
	}
}

// The last try closes its reading with a reason chosen from the cause. The
// budget stop sleeps; a close another claim refuses sleeps one lease.
func TestATranscriptReadingsLastTry(t *testing.T) {
	var e *transcriptEnv // each case's own meeting, so no reading carries over
	steal := func() {
		if _, err := e.owner.Exec(t.Context(), `UPDATE transcript_read SET started_at = started_at - interval '1 second'
			WHERE activity_id = $1 AND status = 'running'`, e.activity.UUID); err != nil {
			t.Errorf("taking the claim: %v", err)
		}
	}
	deferral := &ai.BudgetDeferralError{Task: ai.TaskTranscriptPropose, NextAttemptAt: clocktest.Now(t).Add(240 * time.Hour)}
	for _, tc := range []struct {
		name, status, code string
		brain              completer
		snooze             bool
	}{
		{"model failed", activities.TranscriptReadFailed, activities.TranscriptFailureUnanswered, cannedBrain{err: ai.ErrAllTiersFailed}, false},
		{"try timed out", activities.TranscriptReadFailed, activities.TranscriptFailureUnfinished, &interruptedBrain{err: context.Canceled}, false},
		{"budget stop", activities.TranscriptReadQueued, "", cannedBrain{err: deferral}, true},
		{"claim taken", activities.TranscriptReadRunning, "", &interruptedBrain{before: steal, err: ai.ErrAllTiersFailed}, true},
	} {
		e = setupTranscript(t)
		read, errs := e.tryTranscript(t, oneOffJobMaxAttempts, tc.brain)
		_, snoozed := errors.AsType[*river.JobSnoozeError](errs[0])
		code := ""
		if read.Outcome != nil && read.Outcome.Failure != nil {
			code = read.Outcome.Failure.Code
		}
		if errs[0] == nil || snoozed != tc.snooze || read.Status != tc.status || code != tc.code {
			t.Errorf("%s: err = %v, reading %q coded %q; want snooze %v, %q coded %q", tc.name, errs[0], read.Status, code, tc.snooze, tc.status, tc.code)
		}
	}
}

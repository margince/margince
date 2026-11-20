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
		errs = append(errs, worker.Work(context.Background(), &river.Job[TranscriptProposeArgs]{JobRow: row, Args: args}))
	}
	read, err := e.Activities.GetTranscriptRead(e.ctx, e.activity, started.ID)
	if err != nil {
		t.Fatalf("reading back the run record: %v", err)
	}
	return read, errs
}

// River repeats a failed try a second later, inside the claim's lease. The
// repeat must find a reading it can claim. Else it ends as success, and the
// rep watches "reading…" with no job behind it.
func TestATranscriptReadingWhoseModelCallFailedIsReadOnTheNextTry(t *testing.T) {
	e := setupTranscript(t)
	read, errs := e.tryTranscript(t, 1, cannedBrain{err: ai.ErrAllTiersFailed}, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)})
	if errs[0] == nil || errs[1] != nil || read.Status != activities.TranscriptReadDone {
		t.Errorf("tries = %v, reading %q; want the fault for River, then a reading done on the next try", errs, read.Status)
	}
}

// The last try leaves no job to hand the reading back to. It closes the
// reading with a sentence, and the rep can press Read again.
func TestATranscriptReadingWhoseLastTryFailedSaysSo(t *testing.T) {
	e := setupTranscript(t)
	read, errs := e.tryTranscript(t, oneOffJobMaxAttempts, cannedBrain{err: ai.ErrAllTiersFailed})
	if errs[0] == nil || read.Status != activities.TranscriptReadFailed || read.StatusDetail == nil || *read.StatusDetail == "" {
		t.Errorf("err = %v, reading %q with detail %v; want the fault on the job and the reading failed with a sentence",
			errs[0], read.Status, read.StatusDetail)
	}
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

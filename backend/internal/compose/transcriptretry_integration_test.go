// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A transcript reading's failed model call, try by try: handed back for the
// next try, closed with a reason on the last, asleep when told "not now".

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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// workTranscriptTry runs the production worker once over a reading, as try
// attempt of oneOffJobMaxAttempts, and returns the reading after and its error.
func (e *transcriptEnv) workTranscriptTry(t *testing.T, readID ids.UUID, brain completer, attempt int) (activities.TranscriptRead, error) {
	t.Helper()
	now := func() time.Time { return clocktest.Now(t) }
	worker := &transcriptProposeWorker{
		language: installationLanguage(e.Pool), activities: e.Activities, log: slog.New(slog.DiscardHandler),
		proposer: NewTranscriptProposer(e.Pool, brain, e.svc, now, slog.New(slog.DiscardHandler)),
	}
	err := worker.Work(context.Background(), &river.Job[TranscriptProposeArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: oneOffJobMaxAttempts},
		Args: TranscriptProposeArgs{
			Workspace: e.WS, ActivityID: e.activity.UUID, TranscriptReadID: readID,
			RequestedBy: "human:" + e.Rep1.String(),
		},
	})
	read, readErr := e.Activities.GetTranscriptRead(e.ctx, e.activity, readID)
	if readErr != nil {
		t.Fatalf("reading back the run record: %v", readErr)
	}
	return read, err
}

func (e *transcriptEnv) queueReading(t *testing.T) ids.UUID {
	t.Helper()
	started, _, err := e.Activities.StartTranscriptReadQueued(e.ctx, e.activity, "human:"+e.Rep1.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	return started.ID
}

// River repeats a failed try a second later, inside the claim's lease. The
// repeat must find a reading it can claim, or it takes it for somebody else's,
// ends as success, and the rep watches "reading…" with no job behind it.
func TestATranscriptReadingWhoseModelCallFailedIsReadOnTheNextTry(t *testing.T) {
	e := setupTranscript(t)
	readID := e.queueReading(t)
	read, err := e.workTranscriptTry(t, readID, cannedBrain{err: ai.ErrAllTiersFailed}, 1)
	if err == nil || read.Status != activities.TranscriptReadQueued {
		t.Fatalf("failed try: err = %v, reading %q; want the fault for River and the reading handed back as queued", err, read.Status)
	}
	if read, err = e.workTranscriptTry(t, readID, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)}, 2); err != nil || read.Status != activities.TranscriptReadDone {
		t.Errorf("second try: err = %v, reading %q; want done", err, read.Status)
	}
}

// The last try leaves no job to hand the reading back to, so it closes the
// reading with a sentence, and the rep can press Read again.
func TestATranscriptReadingWhoseLastTryFailedSaysSo(t *testing.T) {
	e := setupTranscript(t)
	read, err := e.workTranscriptTry(t, e.queueReading(t), cannedBrain{err: ai.ErrAllTiersFailed}, oneOffJobMaxAttempts)
	if err == nil || read.Status != activities.TranscriptReadFailed || read.StatusDetail == nil || *read.StatusDetail == "" {
		t.Errorf("err = %v, reading %q with detail %v; want the fault on the job and the reading failed with a sentence",
			err, read.Status, read.StatusDetail)
	}
}

// The budget stop is not a failed try: even on its last try the job sleeps
// until the next budget window, and the reading waits queued for it.
func TestATranscriptReadingToldNotNowSleepsInsteadOfFailing(t *testing.T) {
	e := setupTranscript(t)
	deferral := &ai.BudgetDeferralError{Task: ai.TaskTranscriptPropose, NextAttemptAt: clocktest.Now(t).Add(240 * time.Hour)}
	read, err := e.workTranscriptTry(t, e.queueReading(t), cannedBrain{err: deferral}, oneOffJobMaxAttempts)
	if _, snoozed := errors.AsType[*river.JobSnoozeError](err); !snoozed || read.Status != activities.TranscriptReadQueued {
		t.Errorf("err = %v, reading %q; want a snooze, which spends no try, and the reading queued", err, read.Status)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What River sees when a transcript reading's model call fails, try by try:
// the reading is handed back for the next try, closed with a reason on the
// last, and put to sleep, not failed, when the router says "not now".

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// workTranscriptTry runs the production worker once over the reading, as try
// attempt of max.
func (e *transcriptEnv) workTranscriptTry(t *testing.T, readID ids.UUID, brain completer, attempt, maxAttempts int) error {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(os.Stderr, nil))
	worker := &transcriptProposeWorker{
		language:   installationLanguage(e.Pool),
		activities: e.Activities,
		proposer:   NewTranscriptProposer(e.Pool, brain, e.svc, time.Now, quiet),
		log:        quiet,
	}
	return worker.Work(context.Background(), &river.Job[TranscriptProposeArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: maxAttempts},
		Args: TranscriptProposeArgs{
			Workspace: e.WS, ActivityID: e.activity.UUID, TranscriptReadID: readID,
			RequestedBy: "human:" + e.Rep1.String(),
		},
	})
}

func (e *transcriptEnv) queueReading(t *testing.T) ids.UUID {
	t.Helper()
	started, _, err := e.Activities.StartTranscriptReadQueued(e.ctx, e.activity, "human:"+e.Rep1.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	return started.ID
}

func (e *transcriptEnv) readingNow(t *testing.T, readID ids.UUID) activities.TranscriptRead {
	t.Helper()
	read, err := e.Activities.GetTranscriptRead(e.ctx, e.activity, readID)
	if err != nil {
		t.Fatalf("reading back the run record: %v", err)
	}
	return read
}

// One failed model call is a try River repeats a second later. The repeat must
// find the reading it can claim, or it takes the reading for somebody else's,
// ends as success, and the rep watches "reading…" with no job left behind it.
func TestATranscriptReadingWhoseModelCallFailedIsReadOnTheNextTry(t *testing.T) {
	e := setupTranscript(t)
	readID := e.queueReading(t)

	if err := e.workTranscriptTry(t, readID, cannedBrain{err: ai.ErrAllTiersFailed}, 1, oneOffJobMaxAttempts); err == nil {
		t.Fatal("a failed model call is a fault for River to retry, got nil")
	}
	if got := e.readingNow(t, readID).Status; got != activities.TranscriptReadQueued {
		t.Fatalf("after the failed try the reading is %q, want it handed back as queued", got)
	}

	if err := e.workTranscriptTry(t, readID, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)}, 2, oneOffJobMaxAttempts); err != nil {
		t.Fatalf("the second try: %v", err)
	}
	if got := e.readingNow(t, readID).Status; got != activities.TranscriptReadDone {
		t.Errorf("after the second try the reading is %q, want done", got)
	}
}

// The last try leaves no job to hand the reading back to, so it closes the
// reading as failed with a sentence, and the rep can press Read again.
func TestATranscriptReadingWhoseLastTryFailedSaysSo(t *testing.T) {
	e := setupTranscript(t)
	readID := e.queueReading(t)

	err := e.workTranscriptTry(t, readID, cannedBrain{err: ai.ErrAllTiersFailed}, oneOffJobMaxAttempts, oneOffJobMaxAttempts)
	if err == nil {
		t.Fatal("the last failed try still reports its fault on the job, got nil")
	}
	read := e.readingNow(t, readID)
	if read.Status != activities.TranscriptReadFailed || read.StatusDetail == nil || *read.StatusDetail == "" {
		t.Errorf("reading = %q with detail %v, want failed with a sentence the rep can act on", read.Status, read.StatusDetail)
	}
}

// The budget stop is not a failed try: the job sleeps until the next budget
// window, even on its last try, and the reading waits queued for it.
func TestATranscriptReadingToldNotNowSleepsInsteadOfFailing(t *testing.T) {
	e := setupTranscript(t)
	readID := e.queueReading(t)
	deferral := &ai.BudgetDeferralError{Task: ai.TaskTranscriptPropose, NextAttemptAt: time.Now().Add(10 * 24 * time.Hour)}

	err := e.workTranscriptTry(t, readID, cannedBrain{err: deferral}, oneOffJobMaxAttempts, oneOffJobMaxAttempts)
	if _, snoozed := errors.AsType[*river.JobSnoozeError](err); !snoozed {
		t.Fatalf("err = %v, want a snooze: a deferral spends no try", err)
	}
	if got := e.readingNow(t, readID).Status; got != activities.TranscriptReadQueued {
		t.Errorf("the deferred reading is %q, want queued for the job that wakes", got)
	}
}

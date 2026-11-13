// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PullJob is the args of the chat_pull job, which compose declares.
type PullJob interface {
	jobs.WorkspaceScoped
	Connection() ids.UUID
}

// noticeRerun is how soon a pull runs again after a notice arrived while it ran.
const noticeRerun = 5 * time.Second

// PullWorker runs chat_pull for one bot connection. Marks, when set, holds the
// notices. A notice's own enqueue adds nothing while this pull runs. So a
// notice that arrived during the run makes the pull run once more.
type PullWorker[T PullJob] struct {
	Bind   func(context.Context, jobs.WorkspaceScoped) (context.Context, error)
	Puller *Puller
	Marks  Marks
}

// Work implements River's worker for the pull.
func (w *PullWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	// Bind answers no context when it refuses, so the fault rides the job's own.
	bound, err := w.Bind(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// Marks are held to the millisecond, so the start is compared at that grain.
	started := w.Puller.Now().Truncate(time.Millisecond)
	if err := w.Puller.Pull(bound, job.Args.Connection()); err != nil {
		return jobs.FaultContext(bound, err)
	}
	if w.Marks == nil {
		return nil
	}
	// A mark that cannot be read may hide a notice from the run. So the pull
	// runs once more, instead of leaving it to the hourly sweep.
	notice, err := w.Marks.Last(bound, noticeKey(job.Args.Connection()))
	if err != nil || !notice.Before(started) {
		return river.JobSnooze(noticeRerun)
	}
	return nil
}

// Sweeper decides which connections chat_pull_sweep reads on this tick, from
// the moments held in Marks. With no Marks every connection is read.
type Sweeper struct {
	Marks Marks
	Now   func() time.Time
	Log   *slog.Logger
}

// Due reports whether the sweep reads connection now. A moment it cannot
// read counts as no notice, so a broken store falls back to polling.
func (s Sweeper) Due(ctx context.Context, connection ids.UUID) bool {
	if s.Marks == nil {
		return true
	}
	notice, noticeErr := s.Marks.Last(ctx, noticeKey(connection))
	swept, sweptErr := s.Marks.Last(ctx, sweepKey(connection))
	if err := errors.Join(noticeErr, sweptErr); err != nil {
		s.Log.WarnContext(ctx, "chat pull sweep: reading the notice marks failed; reading the channels now", "err", err)
		return true
	}
	return SweepDue(s.Now(), notice, swept)
}

// Swept marks that the sweep queued connection's pull now.
func (s Sweeper) Swept(ctx context.Context, connection ids.UUID) {
	if s.Marks == nil {
		return
	}
	if err := s.Marks.Mark(ctx, sweepKey(connection), s.Now(), PushedPullEvery); err != nil {
		s.Log.WarnContext(ctx, "chat pull sweep: the sweep was not marked; the next tick reads again", "err", err)
	}
}

// NewPuller builds the pull of a worker role over the chat sessions of chats.
func NewPuller(pool *pgxpool.Pool, store *chatlinks.Store, chats LiveChats, lane ai.Completer, log *slog.Logger) *Puller {
	return &Puller{Pool: pool, Store: store, Chats: chats, Activities: activities.NewStore(chats.DB),
		Lane: lane, Now: time.Now, Zone: time.UTC, Log: log}
}

// NewFiler builds the file_channel_message engine of an api role.
func NewFiler(pool *pgxpool.Pool, store *chatlinks.Store, chats LiveChats, lane ai.Completer) *Filer {
	return &Filer{Pool: pool, Store: store, Chats: chats, Activities: activities.NewStore(chats.DB), Lane: lane}
}

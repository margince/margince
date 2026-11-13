// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
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

// PullWorker runs chat_pull for one bot connection.
type PullWorker[T PullJob] struct {
	Bind   func(context.Context, jobs.WorkspaceScoped) (context.Context, error)
	Puller *Puller
}

// Work implements River's worker for the pull.
func (w *PullWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	// Bind answers no context when it refuses, so the fault rides the job's own.
	bound, err := w.Bind(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(bound, w.Puller.Pull(bound, job.Args.Connection()))
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

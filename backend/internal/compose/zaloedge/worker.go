// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zaloedge

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatedge"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IngestJob is the args of the ingest job, which compose declares.
type IngestJob = chatedge.IngestJob

// Worker files one stored message through the shared chat worker. Actor is the
// connector principal the filing runs as.
type Worker[T IngestJob] struct {
	Bind  func(context.Context, jobs.WorkspaceScoped) (context.Context, error)
	Pool  *pgxpool.Pool
	Sink  connector.Sink
	Actor principal.Principal
	Log   *slog.Logger
}

// Work implements River's worker for the ingest job.
func (w *Worker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	shared := chatedge.Worker[T]{
		Name: "zalo_oa_ingest", Bind: w.Bind, Pool: w.Pool, Sink: w.Sink, Actor: w.Actor, Log: w.Log, Decode: decode,
	}
	return jobs.FaultContext(ctx, shared.Work(ctx, job))
}

// decode reads one stored message. An event that is no message has its raw row
// destroyed.
func decode(payload []byte) (chatedge.Decoded, error) {
	ev, err := zalo.Decode(payload)
	if err != nil {
		return chatedge.Decoded{}, err
	}
	if ev.Kind != zalo.KindMessage {
		return chatedge.Decoded{Outcome: chatedge.Discard}, nil
	}
	chat, err := zalo.ChatMessage(ev)
	if err != nil {
		return chatedge.Decoded{}, fmt.Errorf("mapping the message: %w", err)
	}
	return chatedge.Decoded{
		Outcome: chatedge.File, Message: chat,
		Binding: func(link connector.ChatLink) connector.ChatBinding { return zalo.ChatBinding(ev, link) },
	}, nil
}

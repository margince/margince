// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsappedge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatedge"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IngestJob is the args of the ingest job, which compose declares.
type IngestJob = chatedge.IngestJob

// Worker files one stored item through the shared chat worker. Actor is the
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
		Name: "whatsapp_ingest", Bind: w.Bind, Pool: w.Pool, Sink: w.Sink, Actor: w.Actor, Log: w.Log, Decode: decode,
	}
	return jobs.FaultContext(ctx, shared.Work(ctx, job))
}

// decode reads one stored item. A delivery status is no message, and a reaction
// changes nothing. An edit or a revoke goes through the change path.
func decode(payload []byte) (chatedge.Decoded, error) {
	res, err := whatsapp.Decode(payload)
	if err != nil {
		return chatedge.Decoded{}, err
	}
	if len(res.Messages) != 1 {
		return chatedge.Decoded{Outcome: chatedge.Discard}, nil
	}
	msg := res.Messages[0]
	binding := func(link connector.ChatLink) connector.ChatBinding { return whatsapp.ChatBinding(msg, link) }
	chat, err := whatsapp.ChatMessage(msg)
	if err == nil {
		return chatedge.Decoded{Outcome: chatedge.File, Message: chat, Binding: binding}, nil
	}
	if !errors.Is(err, connector.ErrSkip) {
		return chatedge.Decoded{}, fmt.Errorf("mapping the message: %w", err)
	}
	change, err := whatsapp.ChatChange(msg)
	if errors.Is(err, connector.ErrSkip) {
		return chatedge.Decoded{Outcome: chatedge.Discard}, nil
	}
	if err != nil {
		return chatedge.Decoded{}, fmt.Errorf("mapping the change: %w", err)
	}
	return chatedge.Decoded{Outcome: chatedge.Change, Change: change, Binding: binding}, nil
}

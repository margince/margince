// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zaloedge

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatedge"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IngestJob is the args of the ingest job, which compose declares.
type IngestJob = chatedge.IngestJob

// NewWorker is the shared chat worker for this network's ingest job. It files
// through the live connection as the connector principal actor, and decodes a
// stored item with decode.
func NewWorker[T IngestJob](bind func(context.Context, jobs.WorkspaceScoped) (context.Context, error),
	pool *pgxpool.Pool, sink connector.Sink, actor principal.Principal, log *slog.Logger,
) *chatedge.Worker[T] {
	return &chatedge.Worker[T]{Name: "zalo_oa_ingest", Bind: bind, Pool: pool, Sink: sink, Actor: actor, Log: log, Decode: decode}
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

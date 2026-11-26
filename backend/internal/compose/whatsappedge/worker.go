// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsappedge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatedge"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
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
	return &chatedge.Worker[T]{Name: "whatsapp_ingest", Bind: bind, Pool: pool, Sink: sink, Actor: actor, Log: log, Decode: decode}
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

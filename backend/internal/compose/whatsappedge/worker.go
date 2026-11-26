// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsappedge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IngestJob is the args of the ingest job, which compose declares.
type IngestJob interface {
	jobs.WorkspaceScoped
	RawCapture() ids.UUID
	Connection() ids.UUID
}

// Worker files one stored item through capture.FileChatMessage while its
// connection is still live. Nothing the item carries is logged, because a
// message's text and its sender's number are the customer's. Actor is the
// connector principal the filing runs as.
type Worker[T IngestJob] struct {
	Bind  func(context.Context, jobs.WorkspaceScoped) (context.Context, error)
	Pool  *pgxpool.Pool
	Sink  connector.Sink
	Actor principal.Principal
	Log   *slog.Logger
}

// Work implements River's worker for the ingest job. A failure other than a
// refusal is returned, and River's retry is the recovery, because the webhook
// delivered the item once.
func (w *Worker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	bound, err := w.Bind(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	rawID := job.Args.RawCapture()
	var payload []byte
	err = database.WithWorkspaceTx(bound, w.Pool, func(tx pgx.Tx) error {
		var err error
		payload, err = capture.GetRawCapturePayloadTx(bound, tx, rawID)
		return err
	})
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("whatsapp_ingest: reading the stored item: %w", err))
	}
	res, err := whatsapp.Decode(payload)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("whatsapp_ingest: decoding the stored item: %w", err))
	}
	if len(res.Messages) != 1 {
		w.Log.InfoContext(bound, "whatsapp_ingest: discarded a stored item that is no message", "statuses", len(res.Statuses))
		return jobs.FaultContext(ctx, w.discard(bound, rawID))
	}
	return jobs.FaultContext(ctx, w.file(bound, job.Args.Connection(), rawID, res.Messages[0]))
}

func (w *Worker[T]) discard(ctx context.Context, rawID ids.UUID) error {
	return database.WithWorkspaceTx(ctx, w.Pool, func(tx pgx.Tx) error {
		return capture.DeleteRawCaptureTx(ctx, tx, rawID)
	})
}

// file files the message while its connection is live. It destroys the stored item
// when the connection is not live or the customer has been erased. An edit or a
// revoke goes through the change path. A reaction changes nothing, so its item
// is destroyed.
func (w *Worker[T]) file(ctx context.Context, connection, rawID ids.UUID, msg whatsapp.Message) error {
	chat, err := whatsapp.ChatMessage(msg)
	if errors.Is(err, connector.ErrSkip) {
		return w.change(ctx, connection, rawID, msg)
	}
	if err != nil {
		return fmt.Errorf("whatsapp_ingest: mapping the message: %w", err)
	}
	chat.StoredOriginal = rawID
	actorCtx := principal.WithCorrelationID(principal.WithActor(ctx, w.Actor), ids.NewV7())
	live, err := capture.FileThroughLiveChatLink(actorCtx, w.Pool, connection.String(), rawID, func(link connector.ChatLink) error {
		if _, err := capture.FileChatMessage(actorCtx, w.Sink, whatsapp.ChatBinding(msg, link), chat); err != nil {
			if errors.Is(err, connector.ErrSkip) {
				w.Log.InfoContext(actorCtx, "whatsapp_ingest: refused a record naming an erased channel account")
				return capture.ErrDiscardOriginal
			}
			// The natural key names the customer, so the key stays out of the error.
			return fmt.Errorf("whatsapp_ingest: capturing a message: %w", err)
		}
		return nil
	})
	if err == nil && !live {
		w.Log.InfoContext(ctx, "whatsapp_ingest: discarded a message whose connection is no longer live")
	}
	return err
}

// change applies an edit or a revoke to the message it names, through
// capture.ChangeChatMessage. The stored item is the
// message's original when the edit lands, and is deleted when it changes
// nothing. An edit whose original has not landed fails the job, so River
// retries it after the original.
func (w *Worker[T]) change(ctx context.Context, connection, rawID ids.UUID, msg whatsapp.Message) error {
	change, err := whatsapp.ChatChange(msg)
	if errors.Is(err, connector.ErrSkip) {
		w.Log.InfoContext(ctx, "whatsapp_ingest: discarded a stored item that changes no message", "kind", string(msg.Kind))
		return w.discard(ctx, rawID)
	}
	if err != nil {
		return fmt.Errorf("whatsapp_ingest: mapping the change: %w", err)
	}
	sink, ok := w.Sink.(capture.ChatSink)
	if !ok {
		return errors.New("whatsapp_ingest: the sink acts on no chat changes")
	}
	change.StoredOriginal = rawID
	actorCtx := principal.WithCorrelationID(principal.WithActor(ctx, w.Actor), ids.NewV7())
	live, err := capture.FileThroughLiveChatLink(actorCtx, w.Pool, connection.String(), rawID, func(link connector.ChatLink) error {
		outcome, err := capture.ChangeChatMessage(actorCtx, sink, whatsapp.ChatBinding(msg, link), change)
		if err != nil {
			return fmt.Errorf("whatsapp_ingest: changing a message: %w", err)
		}
		if outcome == connector.ChatChangeNotFiled {
			return errors.New("whatsapp_ingest: the message this change is about has not been filed yet")
		}
		return nil
	})
	if err == nil && !live {
		w.Log.InfoContext(ctx, "whatsapp_ingest: discarded a change whose connection is no longer live")
	}
	return err
}

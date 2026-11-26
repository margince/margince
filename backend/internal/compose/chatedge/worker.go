// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatedge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IngestJob is the args of an ingest job, which compose declares.
type IngestJob interface {
	jobs.WorkspaceScoped
	RawCapture() ids.UUID
	Connection() ids.UUID
}

// Outcome says what a decoded stored item is for.
type Outcome int

const (
	// Discard is an item that changes nothing, such as a delivery status or a
	// reaction. Its raw row is destroyed.
	Discard Outcome = iota
	// File is a message to file.
	File
	// Change is an edit or a revoke of a message, applied through the shared
	// change path.
	Change
)

// Decoded is a stored item as the network read it back.
type Decoded struct {
	Outcome Outcome
	// Message is set for File and Change is set for Change.
	Message connector.ChatMessage
	Change  connector.ChatChange
	// Binding states what the live connection says about the item.
	Binding func(connector.ChatLink) connector.ChatBinding
	// Media lists the files a File message carried. The worker downloads them
	// into the message's Parts.
	Media []MediaRef
}

// Worker files one stored item through capture.FileChatMessage while its
// connection is still live. Nothing the item carries is logged, because a
// message's text and its sender are the customer's. Actor is the connector
// principal the filing runs as, and Decode is the network's own reading.
type Worker[T IngestJob] struct {
	// Name prefixes errors and logs, such as "whatsapp_ingest".
	Name   string
	Bind   func(context.Context, jobs.WorkspaceScoped) (context.Context, error)
	Pool   *pgxpool.Pool
	Sink   connector.Sink
	Actor  principal.Principal
	Log    *slog.Logger
	Decode func(payload []byte) (Decoded, error)
}

// Work implements River's worker for the ingest job. A failure other than a
// refusal is returned, and River's retry is the recovery, because the webhook
// delivered the item once. A file that stays unreachable through the last
// attempt is dropped, so the message files without it.
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
		return jobs.FaultContext(ctx, fmt.Errorf("%s: reading the stored item: %w", w.Name, err))
	}
	decoded, err := w.Decode(payload)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("%s: decoding the stored item: %w", w.Name, err))
	}
	switch decoded.Outcome {
	case Discard:
		w.Log.InfoContext(bound, w.Name+": discarded a stored item that is no message")
		return jobs.FaultContext(ctx, database.WithWorkspaceTx(bound, w.Pool, func(tx pgx.Tx) error {
			return capture.DeleteRawCaptureTx(bound, tx, rawID)
		}))
	case Change:
		return jobs.FaultContext(ctx, w.change(bound, job.Args.Connection(), rawID, decoded))
	}
	return jobs.FaultContext(ctx, w.file(bound, job.Args.Connection(), rawID, decoded, job.Attempt >= job.MaxAttempts))
}

// file files the message while its connection is live. It destroys the stored item
// when the connection is not live or the customer has been erased.
func (w *Worker[T]) file(ctx context.Context, connection, rawID ids.UUID, decoded Decoded, lastAttempt bool) error {
	decoded.Message.StoredOriginal = rawID
	// The files come before the transaction, so no database transaction waits on
	// a provider's server.
	if err := attach(ctx, connection, decoded.Media, lastAttempt, &decoded.Message); err != nil {
		return fmt.Errorf("%s: downloading a message's file: %w", w.Name, err)
	}
	for _, drop := range decoded.Message.PartDrops {
		w.Log.InfoContext(ctx, w.Name+": files of a message were not kept", "reason", drop.Reason, "count", drop.Count)
	}
	actorCtx := principal.WithCorrelationID(principal.WithActor(ctx, w.Actor), ids.NewV7())
	live, err := capture.FileThroughLiveChatLink(actorCtx, w.Pool, connection.String(), rawID, func(link connector.ChatLink) error {
		if _, err := capture.FileChatMessage(actorCtx, w.Sink, decoded.Binding(link), decoded.Message); err != nil {
			if errors.Is(err, connector.ErrSkip) {
				w.Log.InfoContext(actorCtx, w.Name+": refused a record naming an erased channel account")
				return capture.ErrDiscardOriginal
			}
			// The natural key names the customer, so the key stays out of the error.
			return fmt.Errorf("%s: capturing a message: %w", w.Name, err)
		}
		return nil
	})
	if err == nil && !live {
		w.Log.InfoContext(ctx, w.Name+": discarded a message whose connection is no longer live")
	}
	return err
}

// change applies an edit or a revoke to the message it names, through
// capture.ChangeChatMessage. The stored item is the message's original when the
// edit lands. A change whose original has not landed fails the job, so River
// retries it after the original.
func (w *Worker[T]) change(ctx context.Context, connection, rawID ids.UUID, decoded Decoded) error {
	sink, ok := w.Sink.(capture.ChatSink)
	if !ok {
		return fmt.Errorf("%s: the sink acts on no chat changes", w.Name)
	}
	decoded.Change.StoredOriginal = rawID
	actorCtx := principal.WithCorrelationID(principal.WithActor(ctx, w.Actor), ids.NewV7())
	live, err := capture.FileThroughLiveChatLink(actorCtx, w.Pool, connection.String(), rawID, func(link connector.ChatLink) error {
		outcome, err := capture.ChangeChatMessage(actorCtx, sink, decoded.Binding(link), decoded.Change)
		if err != nil {
			return fmt.Errorf("%s: changing a message: %w", w.Name, err)
		}
		if outcome == connector.ChatChangeNotFiled {
			return fmt.Errorf("%s: the message this change is about has not been filed yet", w.Name)
		}
		return nil
	})
	if err == nil && !live {
		w.Log.InfoContext(ctx, w.Name+": discarded a change whose connection is no longer live")
	}
	return err
}

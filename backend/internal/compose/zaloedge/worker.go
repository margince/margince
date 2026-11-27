// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zaloedge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatedge"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IngestJob is the args of the ingest job, which compose declares.
type IngestJob = chatedge.IngestJob

// NewWorker is the shared chat worker for this network's ingest job. It files
// through the live connection as the connector principal actor, and decodes a
// stored item with decode. The client downloads the message's attachments.
func NewWorker[T IngestJob](bind func(context.Context) (context.Context, error),
	pool *pgxpool.Pool, sink connector.Sink, actor principal.Principal, log *slog.Logger, client *zalo.Client,
) *chatedge.Worker[T] {
	if client == nil {
		client = zalo.NewClient(nil, "", "")
	}
	return &chatedge.Worker[T]{Name: "zalo_oa_ingest", Bind: bind, Pool: pool, Sink: sink, Actor: actor, Log: log, Decode: decoder{client}.decode}
}

type decoder struct{ client *zalo.Client }

// decode reads one stored message. An event that is no message has its raw row
// destroyed.
func (d decoder) decode(payload []byte) (chatedge.Decoded, error) {
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
		Media:   d.refs(ev.Message),
	}, nil
}

// refs names each attachment that is a file. Its ordinal counts every
// attachment, so a link before a file does not shift the file's identity.
func (d decoder) refs(msg *zalo.Message) []chatedge.MediaRef {
	var refs []chatedge.MediaRef
	for i, a := range msg.Attachments {
		if !a.Downloadable() {
			continue
		}
		refs = append(refs, chatedge.MediaRef{
			Ordinal: i + 1, Name: a.Name,
			Download: func(ctx context.Context, _ ids.UUID, limit int64) ([]byte, error) {
				data, err := d.client.Download(ctx, a, limit)
				return data, refusal(err)
			},
		})
	}
	return refs
}

// refusal marks a download failure that repeating cannot cure. A throttle and an
// unreachable host may pass, so they stay plain errors for River to retry.
func refusal(err error) error {
	switch {
	case err == nil, errors.Is(err, zalo.ErrRateLimited), errors.Is(err, zalo.ErrUnreachable):
		return err
	case errors.Is(err, zalo.ErrMediaTooLarge):
		return chatedge.Refused(chatedge.DropTooLarge, err)
	case errors.Is(err, zalo.ErrMediaChecksum):
		return chatedge.Refused(chatedge.DropChecksum, err)
	}
	return chatedge.Refused(chatedge.DropRefused, err)
}

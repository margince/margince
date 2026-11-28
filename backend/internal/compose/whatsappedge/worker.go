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

	"github.com/margince/margince/backend/internal/compose/chatedge"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IngestJob is the args of the ingest job, which compose declares.
type IngestJob = chatedge.IngestJob

// Media is what the worker downloads a message's files with. Token reads the
// number's access token, which the Graph API wants on every media call.
type Media struct {
	Client *whatsapp.Client
	Token  func(ctx context.Context, connection ids.UUID) (string, error)
}

// NewMedia builds the downloads of a worker. A nil client is the Graph API's
// own, and the token is read from the connection's sealed credential.
func NewMedia(client *whatsapp.Client, pool *pgxpool.Pool, vault keyvault.Vault) Media {
	if client == nil {
		client = whatsapp.NewClient(nil, "", "")
	}
	return Media{Client: client, Token: capture.ChatCredentials{Pool: pool, Vault: vault}.WhatsAppAccessToken}
}

// NewWorker is the shared chat worker for this network's ingest job. It files
// through the live connection as the connector principal actor, and decodes a
// stored item with media.decode.
func NewWorker[T IngestJob](pool *pgxpool.Pool, sink connector.Sink, actor principal.Principal, log *slog.Logger, media Media,
) *chatedge.Worker[T] {
	return &chatedge.Worker[T]{Name: "whatsapp_ingest", Pool: pool, Sink: sink, Actor: actor, Log: log, Decode: media.decode}
}

// decode reads one stored item. A delivery status is no message, and a reaction
// changes nothing. An edit or a revoke goes through the change path.
func (m Media) decode(payload []byte) (chatedge.Decoded, error) {
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
		return chatedge.Decoded{Outcome: chatedge.File, Message: chat, Binding: binding, Media: m.refs(msg)}, nil
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

// refs names the file a media message carries. A message whose payload gives
// neither a media id nor an address has nothing to fetch.
func (m Media) refs(msg whatsapp.Message) []chatedge.MediaRef {
	media := msg.Media
	if msg.Kind != whatsapp.KindMedia || media == nil || (media.ID == "" && media.URL == "") {
		return nil
	}
	return []chatedge.MediaRef{{
		Ordinal: 1, Name: media.Filename, DeclaredType: media.MimeType,
		Download: func(ctx context.Context, connection ids.UUID, limit int64) ([]byte, error) {
			token, err := m.Token(ctx, connection)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, chatedge.Refused(chatedge.DropRefused, err)
			}
			if err != nil {
				return nil, err
			}
			var dl whatsapp.Download
			if media.URL != "" {
				dl, err = m.Client.DownloadURL(ctx, token, media.URL, limit, media.SHA256)
			} else {
				dl, err = m.Client.DownloadMedia(ctx, token, media.ID, limit, media.SHA256)
			}
			return dl.Data, refusal(err)
		},
	}}
}

// refusal marks a Graph failure that repeating cannot cure. A throttle and an
// unreachable host may pass, so they stay plain errors for River to retry.
func refusal(err error) error {
	switch {
	case err == nil, whatsapp.IsRetryable(err):
		return err
	case errors.Is(err, whatsapp.ErrMediaTooLarge):
		return chatedge.Refused(chatedge.DropTooLarge, err)
	case errors.Is(err, whatsapp.ErrMediaChecksum):
		return chatedge.Refused(chatedge.DropChecksum, err)
	}
	return chatedge.Refused(chatedge.DropRefused, err)
}

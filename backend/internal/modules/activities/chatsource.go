// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const safeChatMetadataSQL = `CASE WHEN a.raw IS NULL THEN NULL ELSE jsonb_strip_nulls(jsonb_build_object('provider',a.raw->'provider','channel_name',a.raw->'channel_name','message_count',a.raw->'message_count','ended_at',a.raw->'ended_at','source_deleted',a.raw->'source_deleted','source_changed',a.raw->'source_changed')) END`

// The reserved recap namespace survives record-channel normalization.
func chatSummarySourceSQL() string {
	return "a.source_system='" + provenance.ChatSummarySource + "'"
}

func chatSourceReadSQL(ctx context.Context, visible, withheld string) string {
	actor, _ := principal.Actor(ctx)
	if actor.Type == principal.PrincipalSystem {
		return visible
	}
	return "CASE WHEN " + chatSummarySourceSQL() + " THEN " + withheld + " ELSE " + visible + " END"
}

// ChatSourceBinding keeps internal provider references separate from the activity response.
type ChatSourceBinding struct {
	Provider   string
	Connection ids.UUID
	Channel    string
	Root       string
}

// ChatSource reads a recap's source binding through its record authority.
func (s *Store) ChatSource(ctx context.Context, id ids.ActivityID) (ChatSourceBinding, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return ChatSourceBinding{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ChatSourceBinding{}, err
	}
	var out ChatSourceBinding
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
			return err
		}
		var connection string
		if err := tx.QueryRow(ctx, `SELECT coalesce(a.raw->>'provider','dispact'),coalesce(a.raw->>'connection_id',''),coalesce(a.raw->>'channel_id',''),coalesce(a.raw->>'root_id','') FROM activity a WHERE a.id=@id AND `+chatSummarySourceSQL()+` AND coalesce(a.raw->>'source_deleted','false')<>'true'`, pgx.StrictNamedArgs{"id": id.UUID}).Scan(&out.Provider, &connection, &out.Channel, &out.Root); err != nil {
			return err
		}
		parsed, err := ids.Parse(connection)
		if err != nil || out.Channel == "" || out.Root == "" {
			return apperrors.ErrNotFound
		}
		out.Connection = parsed
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	return out, err
}

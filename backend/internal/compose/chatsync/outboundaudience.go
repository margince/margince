// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type outboundAudience struct {
	key          string
	readers      []principal.Principal
	conversation messaging.Conversation
	bot          string
}

func (w *Flusher) outboundAudience(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link) (outboundAudience, error) {
	reader, ok := chat.(chatwire.RootReader)
	if !ok {
		return outboundAudience{}, messaging.ErrUnsupported
	}
	bot, err := chat.Me(ctx)
	if err != nil {
		return outboundAudience{}, err
	}
	audience, err := reader.Audience(ctx, link.ChannelSlug)
	if err != nil {
		return outboundAudience{}, err
	}
	key, err := messaging.AudienceKey(audience)
	if err != nil || audience.External || !audience.AudienceKnown {
		return outboundAudience{}, chatlinks.ErrAudienceNeedsReview
	}
	out := outboundAudience{key: key, conversation: audience, bot: bot.ID}
	err = database.WithTx(ctx, w.Pool, func(tx pgx.Tx) error {
		readers, err := outboundReaders(ctx, tx, link, audience, bot.ID)
		out.readers = readers
		return err
	})
	return out, err
}

func (w *Flusher) outboundSource(ctx context.Context, id ids.UUID, audience outboundAudience) (sharedText, error) {
	var out sharedText
	err := database.WithTx(ctx, w.Pool, func(tx pgx.Tx) error {
		if err := auth.RequireReadForReaders(ctx, "activity", audience.readers...); err != nil {
			return err
		}
		for group := range auth.ReaderGroups(audience.readers) {
			args := []any{id}
			arg := func(value any) int { args = append(args, value); return len(args) }
			scope, err := auth.ScopeClauseForReaders(ctx, "activity", "a", arg, group...)
			if err != nil {
				return err
			}
			where := "a.id=$1"
			if scope != "" {
				where += " AND (" + scope + ")"
			}
			out, err = sharedRow(ctx, tx, where, "a.id", args...)
			if err != nil || !out.ok {
				return err
			}
		}
		return nil
	})
	return out, err
}

func outboundReaders(ctx context.Context, tx pgx.Tx, link chatlinks.Link, audience messaging.Conversation, botID string) ([]principal.Principal, error) {
	ref := linkRecord(link)
	viewers, err := Viewers(ctx, tx, ref.Type, ref.ID)
	if err != nil {
		return nil, err
	}
	byEmail := map[string]ids.UUID{}
	for _, viewer := range viewers {
		byEmail[correspondence.Fold(viewer.Email)] = viewer.ID
	}
	members := []ids.UUID{}
	seen := map[ids.UUID]bool{}
	for _, member := range audience.Members {
		if member.ID == botID {
			continue
		}
		id, found := byEmail[correspondence.Fold(member.Email)]
		if !found || !member.Active || member.External || member.Bot || seen[id] {
			return nil, chatlinks.ErrAudienceNeedsReview
		}
		seen[id] = true
		members = append(members, id)
	}
	if len(members) == 0 {
		return nil, chatlinks.ErrAudienceNeedsReview
	}
	principals, err := identity.MemberPrincipalsTx(ctx, tx, members)
	if err != nil {
		return nil, err
	}
	readers := make([]principal.Principal, 0, len(members))
	for _, id := range members {
		member, live := principals[id]
		if !live {
			return nil, chatlinks.ErrAudienceNeedsReview
		}
		readers = append(readers, member)
	}
	return readers, nil
}

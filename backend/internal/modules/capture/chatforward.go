// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A workspace member who forwards a customer's message from their own chat app
// to a company account files the customer's message, never a contact for the
// member. The check sits in FileChatMessage, so every network takes the same
// path: Telegram says who wrote the message, WhatsApp and Zalo say only that it
// was forwarded. A forward from anyone who is not a member is ordinary
// correspondence of that sender.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MemberIdentities says which workspace member owns an account on a chat
// network. A network with no verified member accounts answers no member, and
// every forward on it files as the sender's own correspondence.
type MemberIdentities interface {
	MemberOf(ctx context.Context, tx pgx.Tx, provider, channelUserID string) (ids.UUID, bool, error)
}

// WithMemberIdentities returns a copy that recognises members by finder. Compose passes ApprovedAccountMembers.
// A sink with none recognises nobody.
func (s *Sink) WithMemberIdentities(finder MemberIdentities) *Sink {
	c := *s
	c.members = finder
	return &c
}

// ChatMember satisfies connector.ChatMemberFinder.
func (s *Sink) ChatMember(ctx context.Context, provider, channelUserID string) (ids.UUID, bool, error) {
	if s.members == nil {
		return ids.Nil, false, nil
	}
	var member ids.UUID
	var found bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		member, found, err = s.members.MemberOf(ctx, tx, provider, channelUserID)
		return err
	})
	return member, found, err
}

// ApprovedAccountMembers recognises a member by a chat account an admin approved
// as theirs. The provider vouched for the account when its owner linked it, so
// no code has to be typed to prove it. Only Telegram Business has such accounts
// today. A member's WhatsApp number or Zalo account is not verified anywhere,
// so those networks recognise no member.
type ApprovedAccountMembers struct{}

func (ApprovedAccountMembers) MemberOf(ctx context.Context, tx pgx.Tx, provider, channelUserID string) (ids.UUID, bool, error) {
	if provider != telegram.Provider {
		return ids.Nil, false, nil
	}
	var member ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT c.owner_user_id FROM channel_connection c
		  JOIN app_user u ON u.id = c.owner_user_id AND u.status = 'active' AND NOT u.is_agent
		 WHERE c.provider = $1 AND c.account_id = $2 AND c.ownership = 'member'
		   AND c.parent_connection_id IS NOT NULL AND c.approved_at IS NOT NULL
		   AND c.status = $3 AND c.archived_at IS NULL`,
		telegram.BusinessProvider, channelUserID, channelStatusConnected).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("capture: reading the member behind a chat account: %w", err)
	}
	return member, true, nil
}

// forwardRoute is what FileChatMessage does with a message after it knows who
// forwarded it.
type forwardRoute struct {
	msg connector.ChatMessage
	// held says the message names no author, so it is filed for the forwarding
	// member alone and no contact is made.
	held bool
}

// routeForward turns a member's forward into the message it carries. Anything
// else comes back unchanged, with ctx unchanged. The returned context acts for
// the member, so the filed message names them as its submitter.
func routeForward(ctx context.Context, sink connector.Sink, binding connector.ChatBinding, msg connector.ChatMessage) (context.Context, forwardRoute, error) {
	unchanged := forwardRoute{msg: msg}
	finder, ok := sink.(connector.ChatMemberFinder)
	if msg.Forward == nil || msg.Direction != connector.DirectionInbound || !ok {
		return ctx, unchanged, nil
	}
	member, isMember, err := finder.ChatMember(ctx, binding.Provider, msg.Counterparty.UserID)
	if err != nil || !isMember {
		return ctx, unchanged, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ctx, unchanged, fmt.Errorf("capture: a forwarded chat message arrived with no connector on the context")
	}
	actor.UserID = member
	ctx = principal.WithActor(ctx, actor)
	forward := msg.Forward
	if forward.Author == nil {
		return ctx, forwardRoute{msg: msg, held: true}, nil
	}
	// The forward is its own conversation, so it never reads as the member's
	// answer inside the member's chat with the account.
	keys := chatKeys(binding, msg)
	keys.ThreadKey = keys.SourceID
	msg.Keys = &keys
	msg.Counterparty = *forward.Author
	if !forward.OriginalAt.IsZero() {
		msg.OccurredAt = forward.OriginalAt
	}
	return ctx, forwardRoute{msg: msg}, nil
}

// heldForwardRecord strips the record of the forwarder's identity, so the sink
// creates no contact. Held for the member by the sink's own rule for a message
// that names nobody a record could be made for.
func heldForwardRecord(rec connector.NormalizedRecord) connector.NormalizedRecord {
	rec.Counterparty = connector.Counterparty{Direction: rec.Counterparty.Direction}
	return rec
}

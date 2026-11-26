// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A member who forwards a customer's message from their own chat app to a
// company account files the customer's message, never a contact for the member.
// The check sits in FileChatMessage, so every network takes one path.
// Telegram says who wrote the message. WhatsApp and Zalo say only that it was
// forwarded. A forward from a non-member is that sender's own correspondence.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MemberIdentities says which workspace member verified an account on a chat
// network. Zalo ids are per official account, so scope names it; it is empty elsewhere. A network with no verified member accounts answers no member, and
// every forward on it files as the sender's own correspondence.
type MemberIdentities interface {
	MemberOf(ctx context.Context, tx pgx.Tx, provider, scope, channelUserID string) (ids.UUID, bool, error)
}

// ChatCodeRedeemer verifies the account that sent a member's one-time code. It
// reports whether the text was a pending code, and so is no message to file.
type ChatCodeRedeemer interface {
	RedeemCodeTx(ctx context.Context, tx pgx.Tx, provider, scope, channelUserID, text string) (bool, error)
}

// ErrChatCodeRedeemed answers a message that was a verification code. It is
// skipped, so the stored original is destroyed and nothing is filed.
var ErrChatCodeRedeemed = fmt.Errorf("capture: the message was a verification code: %w", connector.ErrSkip)

// WithChatCodeRedeemer returns a copy that verifies member accounts by code.
func (s *Sink) WithChatCodeRedeemer(redeemer ChatCodeRedeemer) *Sink {
	c := *s
	c.codes = redeemer
	return &c
}

// RedeemChatCode satisfies connector.ChatCodeRedeemer.
func (s *Sink) RedeemChatCode(ctx context.Context, provider, scope, channelUserID, text string, original ids.UUID) (bool, error) {
	if s.codes == nil {
		return false, nil
	}
	var redeemed bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		redeemed, err = s.codes.RedeemCodeTx(ctx, tx, provider, scope, channelUserID, text)
		if err != nil || !redeemed || original.IsZero() {
			return err
		}
		return DeleteRawCaptureTx(ctx, tx, original)
	})
	return redeemed, err
}

// memberAccounts is what a sink knows of members' chat accounts. A nil members
// recognises nobody, so every forward files as its sender's own message. A nil
// codes confirms no account.
type memberAccounts struct {
	members MemberIdentities
	codes   ChatCodeRedeemer
}

// WithMemberIdentities returns a copy that recognises members by finder. Compose passes ApprovedAccountMembers.
// A sink with none recognises nobody.
func (s *Sink) WithMemberIdentities(finder MemberIdentities) *Sink {
	c := *s
	c.members = finder
	return &c
}

// ChatMember satisfies connector.ChatMemberFinder.
func (s *Sink) ChatMember(ctx context.Context, provider, scope, channelUserID string) (ids.UUID, bool, error) {
	if s.members == nil {
		return ids.Nil, false, nil
	}
	var member ids.UUID
	var found bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		member, found, err = s.members.MemberOf(ctx, tx, provider, scope, channelUserID)
		return err
	})
	return member, found, err
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
	member, isMember, err := finder.ChatMember(ctx, binding.Provider, binding.AccountScope, msg.Counterparty.UserID)
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

// redeemChatCode reports whether an inbound message is a member's pending
// verification code and nothing else. Such a message is never filed.
func redeemChatCode(ctx context.Context, sink connector.Sink, binding connector.ChatBinding, msg connector.ChatMessage) (bool, error) {
	redeemer, ok := sink.(connector.ChatCodeRedeemer)
	if !ok || msg.Direction != connector.DirectionInbound || msg.Forward != nil {
		return false, nil
	}
	return redeemer.RedeemChatCode(ctx, binding.Provider, binding.AccountScope, msg.Counterparty.UserID, msg.Body, msg.StoredOriginal)
}

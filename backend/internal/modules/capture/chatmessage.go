// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The one entry every chat network files a message through. An adapter states
// what the provider said (connector.ChatMessage) and what its authenticated
// connection is (connector.ChatBinding). The stored record, its key and who may
// read it are derived here, so no network files by a rule of its own.

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ChatMessageKind is the interaction kind every chat message files under.
const ChatMessageKind = "message"

// FileChatMessage stores one chat message through the sink's Upsert. It takes
// the port rather than *Sink so a test double sink is filed through the same
// function.
//
// A message on a member-owned connection is that member's own correspondence.
// The acting principal carries the member. The sink then handles it like a
// message on a per-member transport and records the member's import. It derives
// the audience from that, as for mail. A workspace-owned connection is the
// company's. The adapter never states an audience.
func FileChatMessage(
	ctx context.Context, sink connector.Sink, binding connector.ChatBinding, msg connector.ChatMessage,
) (datasource.EntityRef, error) {
	rec, err := chatRecord(binding, msg)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	if binding.Ownership == connector.ChatOwnedByMember {
		actor, ok := principal.Actor(ctx)
		if !ok || binding.OwnerUserID.IsZero() {
			return datasource.EntityRef{}, fmt.Errorf("%w: a member-owned chat connection names no member to file for", apperrors.ErrInvalidArgument)
		}
		actor.UserID = binding.OwnerUserID
		ctx = principal.WithActor(ctx, actor)
	}
	return sink.Upsert(ctx, rec)
}

// chatRecord is the pure mapping from a chat message to the record the sink
// stores.
func chatRecord(binding connector.ChatBinding, msg connector.ChatMessage) (connector.NormalizedRecord, error) {
	if err := validChatKeyParts(binding, msg); err != nil {
		return connector.NormalizedRecord{}, err
	}
	sourceID := binding.KeyScope + ":" + msg.ConversationID + ":" + msg.IdempotencyKey
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{
			SourceSystem: binding.Provider,
			SourceID:     sourceID,
			// The conversation id of a private chat is the person's own account
			// id, so the pipeline trace stores a hash of the key.
			SourceIDNamesAContact: true,
		},
		Fields: ActivityFields{
			Kind:            ChatMessageKind,
			ChannelProvider: binding.Provider,
			Body:            msg.Body,
			OccurredAt:      msg.OccurredAt,
			Direction:       msg.Direction,
			memberOwned:     binding.Ownership == connector.ChatOwnedByMember,
		},
		Source:         binding.Provider + ":" + sourceID,
		CapturedBy:     binding.CapturedBy,
		StoredOriginal: msg.StoredOriginal,
		Raw:            msg.Raw,
		Counterparty: connector.Counterparty{
			Direction:   msg.Direction,
			DisplayName: msg.Counterparty.DisplayName,
			ChannelIdentity: connector.ChannelIdentity{
				Provider:      binding.Provider,
				AccountScope:  binding.AccountScope,
				ChannelUserID: msg.Counterparty.UserID,
				Username:      msg.Counterparty.Username,
			},
		},
		ThreadKey: binding.Provider + ":" + binding.ThreadScope + ":" + msg.ConversationID,
	}, nil
}

// validChatKeyParts refuses what would make two messages share a key or one
// message's key read as another's. The conversation and message ids are the
// provider's own and are joined with colons, so neither may hold one.
func validChatKeyParts(binding connector.ChatBinding, msg connector.ChatMessage) error {
	switch {
	case binding.Provider == "" || binding.CapturedBy == "" || binding.KeyScope == "" || binding.ThreadScope == "":
		return fmt.Errorf("%w: a chat binding names no provider, capturer or key scope", apperrors.ErrInvalidArgument)
	case msg.ConversationID == "" || msg.IdempotencyKey == "" || msg.Counterparty.UserID == "":
		return fmt.Errorf("%w: a chat message names no conversation, message or counterparty", apperrors.ErrInvalidArgument)
	case strings.Contains(msg.ConversationID, ":") || strings.Contains(msg.IdempotencyKey, ":"):
		return fmt.Errorf("%w: a chat conversation or message id holds a colon, which would make its key ambiguous", apperrors.ErrInvalidArgument)
	case msg.Direction != connector.DirectionInbound && msg.Direction != connector.DirectionOutbound:
		return fmt.Errorf("%w: a chat message is neither inbound nor outbound", apperrors.ErrInvalidArgument)
	}
	return nil
}

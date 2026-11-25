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
// message on a per-member transport and records the member's import. A message
// on a company account is imported by the account's team seat, as a shared
// mailbox's mail is. Either way the seat's posture and the recompute derive the
// audience, as for mail, and the adapter never states one. A workspace-owned
// connection with no seat is the company's own bot, and its messages are open.
func FileChatMessage(
	ctx context.Context, sink connector.Sink, binding connector.ChatBinding, msg connector.ChatMessage,
) (datasource.EntityRef, error) {
	rec, err := chatRecord(binding, msg)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	if binding.SeatID.IsZero() && connector.IsCaptureOnlyThread(binding.Provider, rec.ThreadKey) {
		return datasource.EntityRef{}, fmt.Errorf("%w: a business account's message names no seat, so no posture could decide who reads it", apperrors.ErrInvalidArgument)
	}
	ctx, err = chatActingContext(ctx, binding)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	return sink.Upsert(ctx, rec)
}

// chatRecord is the pure mapping from a chat message to the record the sink
// stores.
func chatRecord(binding connector.ChatBinding, msg connector.ChatMessage) (connector.NormalizedRecord, error) {
	if err := validChatKeyParts(binding, msg); err != nil {
		return connector.NormalizedRecord{}, err
	}
	keys := chatKeys(binding, msg)
	system := chatSourceSystem(binding)
	// A source with keys of its own is stamped with its source system alone; a
	// derived key is stamped "<provider>:<key>".
	source := binding.Provider + ":" + keys.SourceID
	if msg.Keys != nil {
		source = system
	}
	counterparty := connector.Counterparty{
		Email:       msg.Counterparty.Email,
		Domain:      msg.Counterparty.Domain,
		Direction:   msg.Direction,
		DisplayName: msg.Counterparty.DisplayName,
		ChannelIdentity: connector.ChannelIdentity{
			Provider:      binding.Provider,
			AccountScope:  binding.AccountScope,
			ChannelUserID: msg.Counterparty.UserID,
			Username:      msg.Counterparty.Username,
			Alias:         chatAlias(binding.Provider, msg.Counterparty.Alias),
		},
	}.WithDeclaredEmailMerge(msg.Counterparty.EmailMayMatch).WithChosenSubject(msg.ChosenSubject)
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{
			SourceSystem: system,
			SourceID:     keys.SourceID,
			// The conversation id of a private chat is the contact's own account
			// id, so the pipeline trace stores a hash of the key.
			SourceIDNamesAContact: keys.SourceIDNamesAContact,
		},
		Fields: ActivityFields{
			Kind:            ChatMessageKind,
			ChannelProvider: binding.Provider,
			ProviderPrivate: msg.ProviderPrivate,
			Subject:         msg.Subject,
			Body:            msg.Body,
			OccurredAt:      msg.OccurredAt,
			DateOnly:        msg.OccurredDay,
			Direction:       msg.Direction,
			memberOwned:     binding.Ownership == connector.ChatOwnedByMember,
			pinnedPosture:   binding.PinnedPosture,
			sourceRevision:  msg.Revision,
		},
		Source:         source,
		CapturedBy:     binding.CapturedBy,
		StoredOriginal: msg.StoredOriginal,
		Raw:            msg.Raw,
		Counterparty:   counterparty,
		Addresses:      msg.Addresses,
		Participants:   msg.Participants,
		Parts:          msg.Parts,
		PartDrops:      msg.PartDrops,
		ThreadKey:      keys.ThreadKey,
	}, nil
}

// chatKeys is the natural key's source id and the thread key: the source's own
// when it supplied them, otherwise derived from the connection's scopes.
func chatKeys(binding connector.ChatBinding, msg connector.ChatMessage) connector.ChatOwnKeys {
	if msg.Keys != nil {
		return *msg.Keys
	}
	return connector.ChatOwnKeys{
		SourceID:              binding.KeyScope + ":" + msg.ConversationID + ":" + msg.IdempotencyKey,
		ThreadKey:             binding.Provider + ":" + binding.ThreadScope + ":" + msg.ConversationID,
		SourceIDNamesAContact: true,
	}
}

// validChatKeyParts refuses what would make two messages share a key or one
// message's key read as another's. The conversation and message ids are the
// provider's own and are joined with colons, so neither may hold one. A source
// that supplies its own keys is held to the same bounds the unit door applies.
func validChatKeyParts(binding connector.ChatBinding, msg connector.ChatMessage) error {
	if err := validChatBinding(binding); err != nil {
		return err
	}
	if msg.Counterparty.UserID == "" {
		return fmt.Errorf("%w: a chat message names no counterparty", apperrors.ErrInvalidArgument)
	}
	if msg.Keys != nil {
		return validOwnKeys(msg)
	}
	return validScopedKeys(binding, msg)
}

// validChatBinding holds what every chat connection must state, whether it
// files a message or changes one.
func validChatBinding(binding connector.ChatBinding) error {
	if err := validChatOwnership(binding); err != nil {
		return err
	}
	if binding.Provider == "" || binding.CapturedBy == "" {
		return fmt.Errorf("%w: a chat binding names no provider or capturer", apperrors.ErrInvalidArgument)
	}
	return validPinnedPosture(binding.PinnedPosture)
}

// validOwnKeys holds a message whose source supplied its own keys. Direction may
// be empty for a record that has no direction.
func validOwnKeys(msg connector.ChatMessage) error {
	switch {
	case msg.Keys.SourceID == "":
		return fmt.Errorf("%w: a chat message's own keys name no source id", apperrors.ErrInvalidArgument)
	case msg.Direction != "" && msg.Direction != connector.DirectionInbound && msg.Direction != connector.DirectionOutbound:
		return fmt.Errorf("%w: a chat message is neither inbound nor outbound", apperrors.ErrInvalidArgument)
	}
	return nil
}

// validScopedKeys holds a message whose keys are derived from the connection's
// scopes and the provider's own ids.
func validScopedKeys(binding connector.ChatBinding, msg connector.ChatMessage) error {
	if err := validScopedIDs(binding, msg.ConversationID, msg.IdempotencyKey); err != nil {
		return err
	}
	if msg.Direction != connector.DirectionInbound && msg.Direction != connector.DirectionOutbound {
		return fmt.Errorf("%w: a chat message is neither inbound nor outbound", apperrors.ErrInvalidArgument)
	}
	return nil
}

// validScopedIDs holds the part of a scoped key a change to a message shares
// with the message: the connection's scopes and the provider's own ids.
func validScopedIDs(binding connector.ChatBinding, conversationID, messageID string) error {
	switch {
	case binding.KeyScope == "" || binding.ThreadScope == "":
		return fmt.Errorf("%w: a chat binding names no key scope", apperrors.ErrInvalidArgument)
	case conversationID == "" || messageID == "":
		return fmt.Errorf("%w: a chat message names no conversation or message", apperrors.ErrInvalidArgument)
	case strings.Contains(conversationID, ":") || strings.Contains(messageID, ":"):
		return fmt.Errorf("%w: a chat conversation or message id holds a colon, which would make its key ambiguous", apperrors.ErrInvalidArgument)
	}
	return nil
}

// validChatOwnership refuses a binding whose ownership is neither workspace
// nor member: the zero value would otherwise file a member's chat workspace-wide.
func validChatOwnership(binding connector.ChatBinding) error {
	switch {
	case binding.Ownership != connector.ChatOwnedByWorkspace && binding.Ownership != connector.ChatOwnedByMember:
		return fmt.Errorf("%w: a chat binding is owned by neither the workspace nor a member", apperrors.ErrInvalidArgument)
	case binding.Ownership == connector.ChatOwnedByMember && binding.OwnerUserID.IsZero():
		return fmt.Errorf("%w: a member-owned chat connection names no member to file for", apperrors.ErrInvalidArgument)
	}
	return nil
}

// validPinnedPosture admits only the postures that narrow a reading.
func validPinnedPosture(posture string) error {
	switch posture {
	case "", connector.ChatPostureClassified, connector.ChatPostureHeld:
		return nil
	}
	return fmt.Errorf("%w: a pinned chat posture is classified or held", apperrors.ErrInvalidArgument)
}

func chatAlias(provider string, alias *connector.ChatAlias) *connector.ChannelIdentity {
	if alias == nil {
		return nil
	}
	return &connector.ChannelIdentity{Provider: provider, AccountScope: alias.AccountScope, ChannelUserID: alias.UserID}
}

// chatActingContext puts on the context who a chat connection acts for: the
// member behind a member-owned account, and the seat the account files through.
// Filing a message and changing one both run under it, so the two cannot
// disagree about whose correspondence a message is.
func chatActingContext(ctx context.Context, binding connector.ChatBinding) (context.Context, error) {
	if binding.Ownership == connector.ChatOwnedByMember {
		actor, ok := principal.Actor(ctx)
		if !ok || binding.OwnerUserID.IsZero() {
			return ctx, fmt.Errorf("%w: a member-owned chat connection names no member to file for", apperrors.ErrInvalidArgument)
		}
		actor.UserID = binding.OwnerUserID
		ctx = principal.WithActor(ctx, actor)
	}
	if !binding.SeatID.IsZero() {
		ctx = withChatSeat(ctx, binding.SeatID)
		if binding.Ownership == connector.ChatOwnedByWorkspace {
			// A company account is a team seat, imported as a shared mailbox is.
			ctx = withSharedMailbox(ctx, binding.SeatID)
		}
	}
	return ctx, nil
}

// chatSourceSystem is the natural key's source system: the provider, unless an
// extension unit's records are keyed under the source the unit declared.
func chatSourceSystem(binding connector.ChatBinding) string {
	if binding.SourceSystem == "" {
		return binding.Provider
	}
	return binding.SourceSystem
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

// How a WhatsApp Business message is filed. The number's phone number id scopes
// the message keys and the thread. The thread is capture-only, because staff
// answer in the WhatsApp Business app. The customer's messages and what staff type
// in the app are one thread keyed by the customer.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// CapturedBy is the provenance stamp of every record this connector files.
const CapturedBy = "connector:whatsapp"

// ChatMessage maps one decoded message to what the provider said about it. A
// reaction, an edit and a revoke are no new conversation entry, so they answer
// connector.ErrSkip. ChatChange maps the edit and the revoke. A media item files as its caption, or as a
// placeholder naming its type when it has none. The ingest worker downloads
// the file itself.
func ChatMessage(msg Message) (connector.ChatMessage, error) {
	_, userID := CustomerIdentity(msg)
	if userID == "" {
		return connector.ChatMessage{}, fmt.Errorf("%w: a message names no customer", ErrMalformedItem)
	}
	body := ""
	switch msg.Kind {
	case KindText:
		body = msg.Text
	case KindMedia:
		body = msg.Text
		if body == "" && msg.Media != nil {
			body = "[" + msg.Media.Type + "]"
		}
	case KindPlaceholder:
		body = "[" + placeholderName(msg.Type) + "]"
	default:
		return connector.ChatMessage{}, connector.ErrSkip
	}
	direction := connector.DirectionInbound
	if msg.Direction == Outbound {
		direction = connector.DirectionOutbound
	}
	party := connector.ChatParty{UserID: userID, DisplayName: msg.DisplayName}
	if all := CustomerIdentities(msg); len(all) > 1 {
		party.Alias = &connector.ChatAlias{AccountScope: all[1].Scope, UserID: all[1].UserID}
	}
	chat := connector.ChatMessage{
		ConversationID: userID,
		IdempotencyKey: msg.ID,
		Direction:      direction,
		Counterparty:   party,
		Body:           body,
		OccurredAt:     msg.At,
	}
	// WhatsApp says only that a message was forwarded, never who wrote it.
	if msg.Forwarded && direction == connector.DirectionInbound {
		chat.Forward = &connector.ChatForward{}
	}
	return chat, nil
}

// ChatChange maps an edit or a revoke to a change of the message it names. The
// edit's revision is the time of the edit event, and WhatsApp states no order
// for two edits stamped alike, so RevisionSeq stays zero. Any other kind, and an
// edit that carries no text, answer connector.ErrSkip.
func ChatChange(msg Message) (connector.ChatChange, error) {
	_, userID := CustomerIdentity(msg)
	if userID == "" {
		return connector.ChatChange{}, fmt.Errorf("%w: a message names no customer", ErrMalformedItem)
	}
	change := connector.ChatChange{ConversationID: userID, IdempotencyKey: msg.OriginalID}
	switch {
	case msg.OriginalID == "":
		return connector.ChatChange{}, connector.ErrSkip
	case msg.Kind == KindEdit && msg.Text != "":
		change.Kind, change.Body, change.Revision = connector.ChatEdited, msg.Text, msg.At
	case msg.Kind == KindRevoke:
		change.Kind = connector.ChatDeleted
	default:
		return connector.ChatChange{}, connector.ErrSkip
	}
	return change, nil
}

func placeholderName(typ string) string {
	if typ == "" {
		return "unsupported message"
	}
	return typ
}

// ChatBinding is what the connection says about the message. It names the number,
// whose it is and the seat it files through. It also names the scope of the
// customer's identity, which comes from the message. A customer with a phone is
// global, and one with only a BSUID belongs to the business account.
func ChatBinding(msg Message, link connector.ChatLink) connector.ChatBinding {
	scope, _ := CustomerIdentity(msg)
	threadScope := connector.CaptureOnlyThreadScope + ":" + link.AccountID
	return connector.ChatBinding{
		Provider:     Provider,
		CapturedBy:   CapturedBy,
		KeyScope:     threadScope,
		ThreadScope:  threadScope,
		AccountScope: scope,
		Ownership:    link.Ownership,
		OwnerUserID:  link.OwnerUserID,
		SeatID:       link.SeatID,
	}
}

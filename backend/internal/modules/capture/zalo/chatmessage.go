// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

// How a Zalo OA message is filed. The OA id scopes the message keys and the
// thread. The thread is capture-only, because staff answer in Zalo. A customer's
// messages and what staff type in OA Admin chat are one thread keyed by the
// customer.

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// CapturedBy is the provenance stamp of every record this connector files.
const CapturedBy = "connector:zalo_oa"

// ChatMessage maps one decoded message event to what the provider said about it.
// A message with no text files as a placeholder naming its first
// attachment's type, and the ingest worker downloads the files. A colon in a provider id is replaced, because the sink joins
// scope, conversation and message with colons.
func ChatMessage(ev Event) (connector.ChatMessage, error) {
	if ev.Kind != KindMessage || ev.Message == nil || ev.Message.ID == "" || ev.UserID == "" {
		return connector.ChatMessage{}, fmt.Errorf("%w: a message names no customer or id", ErrMalformedEvent)
	}
	direction := connector.DirectionInbound
	if ev.Message.Direction == Outbound {
		direction = connector.DirectionOutbound
	}
	chat := connector.ChatMessage{
		ConversationID: safeKeyPart(ev.UserID),
		IdempotencyKey: safeKeyPart(ev.Message.ID),
		Direction:      direction,
		Counterparty:   connector.ChatParty{UserID: ev.UserID},
		Body:           body(ev.Message),
		OccurredAt:     ev.OccurredAt,
	}
	if direction == connector.DirectionInbound {
		chat.ReplyToID = ev.Message.ReplyToID
		chat.SharedContact = businessCardOf(ev.Message)
	}
	return chat, nil
}

// businessCardOf is the customer a business card shares, when the card names one.
func businessCardOf(msg *Message) *connector.ChatSharedContact {
	for _, a := range msg.Attachments {
		if a.UserID == "" && a.Phone == "" {
			continue
		}
		card := &connector.ChatSharedContact{UserID: a.UserID}
		if a.Phone != "" {
			card.Phones = []string{a.Phone}
		}
		return card
	}
	return nil
}

func safeKeyPart(id string) string { return strings.ReplaceAll(id, ":", "_") }

func body(msg *Message) string {
	if msg.Text != "" {
		return msg.Text
	}
	if len(msg.Attachments) > 0 && msg.Attachments[0].Type != "" {
		return "[" + msg.Attachments[0].Type + "]"
	}
	if msg.Type == MessageUnsupported || msg.Type == "" {
		return "[unsupported message]"
	}
	return "[" + string(msg.Type) + "]"
}

// ChatBinding is what the connection says about the message: the OA whose it is,
// and the seat it files through. The customer's identity is scoped to the OA.
func ChatBinding(ev Event, link connector.ChatLink) connector.ChatBinding {
	scope := connector.CaptureOnlyThreadScope + ":" + safeKeyPart(link.AccountID)
	return connector.ChatBinding{
		Provider:     Provider,
		CapturedBy:   CapturedBy,
		KeyScope:     scope,
		ThreadScope:  scope,
		AccountScope: ev.OAID,
		Ownership:    link.Ownership,
		OwnerUserID:  link.OwnerUserID,
		SeatID:       link.SeatID,
		Connection:   link.ConnectionID,
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

// How a Telegram message is keyed. Telegram's message ids repeat across chats,
// and a private chat's id is the human's own Telegram id, shared by every bot
// that human talks to. Each connection therefore names the scope its chat and
// message ids are unique in:
//
//   - a bot's own chats: the bot id, key "<bot>:<chat>:<message>", thread
//     "telegram:<bot>:<chat>";
//   - a business account's chats: the account the owner linked, never the
//     connection id, which changes when the owner links the bot again. Key
//     "<bot>:biz:<account>:<chat>:<message>", thread "telegram:biz:<account>:<chat>".
//
// Every part is a decimal Telegram id or the literal "biz". The colon therefore
// splits a key one way only, and a business key (five parts) cannot equal a bot
// key (three parts).

import (
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func botBinding(botID string) connector.ChatBinding {
	return connector.ChatBinding{
		Provider:    Provider,
		CapturedBy:  CapturedByTelegram,
		KeyScope:    botID,
		ThreadScope: botID,
		Ownership:   connector.ChatOwnedByWorkspace,
	}
}

func businessBinding(botID string, link BusinessLink) connector.ChatBinding {
	scope := connector.CaptureOnlyThreadScope + ":" + link.AccountID
	return connector.ChatBinding{
		Provider:    Provider,
		CapturedBy:  CapturedByTelegram,
		KeyScope:    botID + ":" + scope,
		ThreadScope: scope,
		Ownership:   link.Ownership,
		OwnerUserID: link.OwnerUserID,
		SeatID:      link.SeatID,
	}
}

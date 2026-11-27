// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

// Normalize's own table-driven proof (design §6.3): pure, no provider
// handle, no database — every case here builds the raw envelope
// BuildRawEnvelope produces for the worker and asserts on the literal
// strings the brief specifies.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// telegramUpdateFixture is one text message from chat 1001, message id 7,
// sender 555 — the exact numbers the natural-key test asserts on. The chat
// carries its type because a real update does and because nothing outside a
// private chat is captured at all.
const telegramUpdateFixture = `{
	"update_id": 100,
	"message": {
		"message_id": 7,
		"chat": {"id": 1001, "type": "private", "username": "annlee"},
		"from": {"id": 555, "username": "annlee", "first_name": "Ann", "last_name": "Lee"},
		"date": 1690000000,
		"text": "hello"
	}
}`

// parsed is what ParseChatMessage returns for one update.
type parsed struct {
	binding connector.ChatBinding
	msg     connector.ChatMessage
}

// parseUpdate runs ParseChatMessage over one verbatim update as bot 42, handing
// back the error too, for a case whose claim is about the error.
func parseUpdate(t *testing.T, update string, link *BusinessLink) (parsed, error) {
	t.Helper()
	raw, err := BuildRawEnvelope("42", []byte(update))
	if err != nil {
		t.Fatalf("BuildRawEnvelope: %v", err)
	}
	binding, msg, err := ParseChatMessage(raw, link)
	return parsed{binding, msg}, err
}

// normalizeOne parses one update that must map, failing the test on any error.
func normalizeOne(t *testing.T, update string) parsed {
	t.Helper()
	p, err := parseUpdate(t, update, nil)
	if err != nil {
		t.Fatalf("ParseChatMessage: %v", err)
	}
	return p
}

// normalizeFixture is normalizeOne over the canonical private-chat message.
func normalizeFixture(t *testing.T) parsed {
	t.Helper()
	return normalizeOne(t, telegramUpdateFixture)
}

// bodyOf reads the message body.
func bodyOf(_ *testing.T, p parsed) string { return p.msg.Body }

// assertSkipped insists ParseChatMessage declined an update with an ErrSkip,
// which the worker counts as an exclusion rather than a fault.
func assertSkipped(t *testing.T, update string) {
	t.Helper()
	_, err := parseUpdate(t, update, nil)
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("got %v, want an ErrSkip-wrapped error", err)
	}
}

// message_id is unique only within a chat, so the key must be chat-scoped. A key
// omitting the chat would collide two customers' conversations into one
// activity. The key itself is built by
// capture.FileChatMessage; this proves the scopes and ids it is built from.
func TestParseChatMessageNamesTheBotAndChatScope(t *testing.T) {
	p := normalizeFixture(t)
	if p.binding.KeyScope != "42" || p.binding.ThreadScope != "42" {
		t.Errorf("scopes = %q and %q, want the bot id 42 for both", p.binding.KeyScope, p.binding.ThreadScope)
	}
	if p.msg.ConversationID != "1001" || p.msg.IdempotencyKey != "7" {
		t.Errorf("conversation %q, message %q, want 1001 and 7", p.msg.ConversationID, p.msg.IdempotencyKey)
	}
	if p.binding.Provider != Provider || p.binding.CapturedBy != CapturedByTelegram {
		t.Errorf("binding provider %q captured by %q, want %q and %q", p.binding.Provider, p.binding.CapturedBy, Provider, CapturedByTelegram)
	}
	if p.binding.Ownership != connector.ChatOwnedByWorkspace || p.binding.AccountScope != "" {
		t.Errorf("binding = %+v, want workspace-owned with no account scope", p.binding)
	}
}

// A Telegram counterparty has no address at all: the provider's user id is
// its identity.
func TestParseChatMessageNamesTheSenderAsTheCounterparty(t *testing.T) {
	p := normalizeFixture(t)
	want := connector.ChatParty{UserID: "555", DisplayName: "Ann Lee", Username: "annlee"}
	if p.msg.Counterparty != want || p.msg.Direction != connector.DirectionInbound {
		t.Errorf("counterparty %+v direction %q, want %+v inbound", p.msg.Counterparty, p.msg.Direction, want)
	}
}

// A my_chat_member update (the block/unblock signal) carries no message.
// ParseChatMessage skips it rather than erroring.
func TestParseChatMessageSkipsAnUpdateWithNoMessage(t *testing.T) {
	assertSkipped(t, `{"update_id":101,"my_chat_member":{}}`)
}

// Group chats are out of scope (design §1) and `allowed_updates` cannot say so:
// Telegram delivers a group message under the same bare `message` update a
// private one arrives on. Capturing one would mint a Contact per member the
// bot's privacy mode happens to show, file the activity under the group's
// thread, and then route the rep's reply to the sender's PRIVATE chat — a
// message answered somewhere other than where it was read.
func TestParseChatMessageSkipsAGroupChatMessage(t *testing.T) {
	assertSkipped(t, `{
		"update_id": 102,
		"message": {
			"message_id": 8,
			"chat": {"id": -1001234567890, "type": "supergroup", "title": "Acme Support"},
			"from": {"id": 555, "username": "annlee", "first_name": "Ann"},
			"date": 1690000100,
			"text": "/help please"
		}
	}`)
}

// The scope exclusion fails CLOSED: an update whose chat states no type is not
// evidence of a private conversation, and capturing it on the strength of a
// missing field would file a message nobody can be shown to be able to reply to.
func TestParseChatMessageSkipsAChatOfUnstatedType(t *testing.T) {
	assertSkipped(t, `{
		"update_id": 103,
		"message": {"message_id": 9, "chat": {"id": 1001}, "from": {"id": 555}, "date": 1690000100, "text": "hello"}
	}`)
}

// Telegram gives a supergroup or channel a NEGATIVE id, and the type field is
// the payload's own claim about itself. A chat calling itself private while
// carrying a group's id is therefore not a private chat, and admitting it on
// the strength of the label would file a group conversation as a customer's
// and then answer it in that group — publishing the rep's reply to whoever
// owns the id.
func TestParseChatMessageSkipsAChatWhoseIDIsNotAPrivateOne(t *testing.T) {
	assertSkipped(t, `{
		"update_id": 106,
		"message": {
			"message_id": 12,
			"chat": {"id": -1001234567890, "type": "private"},
			"from": {"id": 555, "username": "annlee"},
			"date": 1690000100,
			"text": "mislabelled"
		}
	}`)
}

// The same rule for the sender: an account id is positive, so a negative one
// names no Telegram account. Minted as an identity it would bind a Contact to a
// channel_user_id no human owns and no reply can reach.
func TestParseChatMessageSkipsAMessageFromANonAccountSender(t *testing.T) {
	assertSkipped(t, `{
		"update_id": 107,
		"message": {
			"message_id": 13,
			"chat": {"id": 1001, "type": "private", "username": "annlee"},
			"from": {"id": -555},
			"date": 1690000100,
			"text": "who owns this id?"
		}
	}`)
}

// A sender id of 0 — Telegram's rendering of a message with no `from` — is a
// valid, non-empty key that EVERY anonymous sender shares. Captured, it merges
// distinct humans onto one Contact and one identity row, which then reads as
// reachable at chat id 0. The private-chat gate excludes the shape this
// arrives in today, so the refusal is stated here, where the identity is
// minted, rather than left resting on a gate that exists for another reason.
func TestParseChatMessageSkipsAMessageWithNoSender(t *testing.T) {
	assertSkipped(t, `{
		"update_id": 104,
		"message": {
			"message_id": 10,
			"chat": {"id": 1001, "type": "private", "username": "annlee"},
			"date": 1690000100,
			"text": "who sent this?"
		}
	}`)
}

// Raw must stay empty for Telegram. The poll already stored the update as the
// only-copy evidence row. A message carrying Raw would store the same bytes
// again under another key. The subject would then receive every message twice
// in an Art. 15 export.
func TestParseChatMessageCarriesNoRawBecauseThePollOwnsTheEvidenceCopy(t *testing.T) {
	if raw := normalizeFixture(t).msg.Raw; len(raw) != 0 {
		t.Errorf("Raw = %s, want empty — the raw_capture row the poll wrote is the single evidence copy", raw)
	}
}

// A text message's body is its text — the baseline the media cases below vary.
func TestParseChatMessageCapturesTheMessageTextAsTheBody(t *testing.T) {
	if body := bodyOf(t, normalizeFixture(t)); body != "hello" {
		t.Errorf("Body = %q, want %q", body, "hello")
	}
}

// Telegram puts the words of a media message in `caption`, never in `text`, so
// a connector reading only `text` files a photo-with-a-caption as an activity
// with an empty body: the customer's sentence is on the wire and nowhere on the
// timeline.
func TestParseChatMessageKeepsAMediaCaptionAsTheBody(t *testing.T) {
	p := normalizeOne(t, `{
		"update_id": 104,
		"message": {
			"message_id": 10,
			"chat": {"id": 1001, "type": "private", "username": "annlee"},
			"from": {"id": 555, "username": "annlee", "first_name": "Ann"},
			"date": 1690000100,
			"photo": [{"file_id": "AgACAgQ", "width": 90, "height": 90}],
			"caption": "here is the damaged part"
		}
	}`)
	if body := bodyOf(t, p); body != "here is the damaged part" {
		t.Errorf("Body = %q, want the caption", body)
	}
}

// A wordless media message is captured with a placeholder naming what arrived,
// not skipped and not left blank: the customer did reach out, and both of the
// alternatives leave the rep a timeline that says otherwise while the reply box
// offers to answer it.
func TestParseChatMessageNamesTheMediaKindWhenAMessageHasNoWords(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    string
	}{
		{"photo", `"photo": [{"file_id": "AgACAgQ"}]`, "[photo]"},
		{"sticker", `"sticker": {"file_id": "CAACAgIA", "emoji": "👍"}`, "[sticker]"},
		{"voice", `"voice": {"file_id": "AwACAgQ", "duration": 3}`, "[voice message]"},
		// Telegram sends a GIF as an animation AND a document; the animation is
		// the truer of the two descriptions.
		{"animation", `"animation": {"file_id": "CgACAgQ"}, "document": {"file_id": "BQACAgQ"}`, "[animation]"},
		// A kind this package does not model (a poll here) still says something
		// arrived rather than vanishing.
		{"unmodelled kind", `"poll": {"question": "which day suits?"}`, "[attachment]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := normalizeOne(t, `{
				"update_id": 105,
				"message": {
					"message_id": 11,
					"chat": {"id": 1001, "type": "private", "username": "annlee"},
					"from": {"id": 555, "username": "annlee"},
					"date": 1690000100,
					`+tc.payload+`
				}
			}`)
			if body := bodyOf(t, p); body != tc.want {
				t.Errorf("Body = %q, want %q", body, tc.want)
			}
		})
	}
}

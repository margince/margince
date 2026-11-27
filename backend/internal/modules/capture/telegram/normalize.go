// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

// The pure Telegram → chat message mapping (no provider handle, no
// I/O). ParseChatMessage maps the envelope connector.go's BuildRawEnvelope built
// (the bot id joined with one verbatim Telegram update) onto the binding and the
// message that capture.FileChatMessage files. This is the test-guarded surface a
// table-driven fixture proves without a live bot or a database.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Provider is the source_system / NaturalKey namespace every Telegram record
// carries. It restates capture.ProviderTelegram, because capture already
// imports this package. Keep the two equal.
const Provider = "telegram"

// BusinessProvider is the provider of a Telegram Business link, the row an
// account owner's link of the bot gets in channel_connection. It differs from
// Provider so a reader that polls or sends through "the telegram connection"
// never meets a link, which holds no credential.
const BusinessProvider = "telegram_business"

// KindMessage is the interaction kind every captured channel message files
// under. Restated here rather than imported from the activities module for the
// same reason Provider is: a capture connector must not reach across to a
// sibling module, and the compose-level drift test holds the two spellings
// against each other.
const KindMessage = "message"

// telegramUser is the `from` object of a Telegram message: the sender's
// identity as Telegram reports it.
type telegramUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// chatTypePrivate is the ONLY chat type this connector captures: design §1
// puts group chats out of scope, and the exclusion is load-bearing twice
// over. A bot in a group runs in Telegram's default privacy mode and is shown
// only commands, so a group conversation arrives as fragments of itself. And a
// reply resolves its recipient through the sender's channel identity — which
// for Telegram is the sender's PRIVATE chat — so a message filed under a group
// thread would be answered somewhere else entirely, or refused outright by a
// user who never started the bot. Telegram answers group messages under the
// same bare `message` update this connector subscribes to, so the refusal has to
// happen here.
const chatTypePrivate = "private"

// telegramChat is the `chat` object: the id is half of the chat-scoped natural
// key (design §6.3's load-bearing rule that message_id repeats across chats),
// Type is the scope gate above, and Username is the counterpart's handle —
// present because for a private chat the chat IS that user, which is what lets
// a membership update read an identity straight out of it (membership.go).
type telegramChat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// isPrivate reports whether this chat is the 1:1 bot conversation. A missing
// or unrecognized type is NOT private: the scope exclusion fails closed, so an
// update shape this package cannot place is skipped rather than captured
// against a conversation nobody can reply to.
//
// The id has to agree with the label, because the label is only the payload's
// claim about itself. Telegram numbers a private chat POSITIVE and a
// supergroup or channel negative, so a negative id under a `private` type is a
// group — precisely the chat this connector excludes, and the one a reply must
// never be published into.
//
// Zero is no chat at all, and it matters most on the membership path, which
// reads the customer's account OUT of the chat: a zero id there would be
// written as a reachability state against "telegram:0", an account every such
// update would share. Refusing it here is why that path needs no guard of its
// own.
func (c telegramChat) isPrivate() bool { return c.Type == chatTypePrivate && c.ID > 0 }

// telegramMedia is the attachment set Telegram delivers in place of `text`.
// Every field is decoded for PRESENCE only, never for its schema: naming the
// kind is all a wordless message's body needs, and modelling file ids would be
// the media-download path this feature excludes.
type telegramMedia struct {
	Animation json.RawMessage `json:"animation"`
	Photo     json.RawMessage `json:"photo"`
	Sticker   json.RawMessage `json:"sticker"`
	Voice     json.RawMessage `json:"voice"`
	VideoNote json.RawMessage `json:"video_note"`
	Video     json.RawMessage `json:"video"`
	Audio     json.RawMessage `json:"audio"`
	Document  json.RawMessage `json:"document"`
	Location  json.RawMessage `json:"location"`
	Contact   json.RawMessage `json:"contact"`
}

// telegramMessage is the one update kind this system captures as an
// activity. Date is Telegram's unix-seconds send time; Caption is where
// Telegram puts the words of a message whose payload is media, embedded
// alongside the media set so one struct decodes the whole message.
type telegramMessage struct {
	// BusinessConnectionID is set on a business_message and names the link the
	// message arrived through.
	BusinessConnectionID string       `json:"business_connection_id"`
	MessageID            int64        `json:"message_id"`
	Chat                 telegramChat `json:"chat"`
	From                 telegramUser `json:"from"`
	Date                 int64        `json:"date"`
	// EditDate is when Telegram says the message was last edited. It is zero
	// for a message that was never edited.
	EditDate int64  `json:"edit_date"`
	Text     string `json:"text"`
	Caption  string `json:"caption"`
	// ForwardOrigin is set when the sender forwarded a message written elsewhere.
	ForwardOrigin *telegramForwardOrigin `json:"forward_origin"`
	// ReplyToMessage is the message this one answers, when it answers one.
	ReplyToMessage *struct {
		MessageID int64 `json:"message_id"`
	} `json:"reply_to_message"`
	telegramMedia
}

// telegramForwardOrigin is where a forwarded message came from. Only a `user`
// origin names the author by account. A `hidden_user` origin, a channel and a
// chat carry no user id this connector could file the message under.
type telegramForwardOrigin struct {
	Type       string       `json:"type"`
	Date       int64        `json:"date"`
	SenderUser telegramUser `json:"sender_user"`
}

// chatForward is what Telegram said about the forward, or nil for a message
// written by its sender.
func chatForward(origin *telegramForwardOrigin) *connector.ChatForward {
	if origin == nil {
		return nil
	}
	forward := &connector.ChatForward{}
	if origin.Date > 0 {
		forward.OriginalAt = time.Unix(origin.Date, 0).UTC()
	}
	if origin.Type == "user" && origin.SenderUser.ID > 0 {
		forward.Author = &connector.ChatParty{
			UserID:      fmt.Sprintf("%d", origin.SenderUser.ID),
			DisplayName: telegramDisplayName(origin.SenderUser),
			Username:    origin.SenderUser.Username,
		}
	}
	return forward
}

// telegramUpdate is Telegram's own update envelope. Message is a pointer
// because the other update kind this connector subscribes to (my_chat_member,
// the block/unblock signal) carries none; a nil Message is how Normalize tells
// "not a message" from "a message with no text".
type telegramUpdate struct {
	UpdateID              int64                    `json:"update_id"`
	Message               *telegramMessage         `json:"message"`
	BusinessMessage       *telegramMessage         `json:"business_message"`
	EditedMessage         *telegramMessage         `json:"edited_message"`
	EditedBusinessMessage *telegramMessage         `json:"edited_business_message"`
	DeletedBusiness       *deletedBusinessMessages `json:"deleted_business_messages"`
}

// deletedBusinessMessages is Telegram's report that messages of a business
// chat were deleted, by either side. It carries the ids and no content.
type deletedBusinessMessages struct {
	BusinessConnectionID string       `json:"business_connection_id"`
	Chat                 telegramChat `json:"chat"`
	MessageIDs           []int64      `json:"message_ids"`
}

// ingestEnvelope is the raw record Normalize actually maps: the connected
// bot's numeric id alongside Telegram's own update JSON, verbatim. Update
// stays a json.RawMessage rather than a decoded telegramUpdate so an update
// kind this package does not parse into a typed field (my_chat_member's own
// payload, an edited_message) survives untouched for a later Normalize
// extension to read — decoding and re-encoding here would silently drop it.
type ingestEnvelope struct {
	BotID  string          `json:"bot_id"`
	Update json.RawMessage `json:"update"`
}

// BusinessLink is what the capture side knows about the approved Telegram
// Business link a business_message arrived through. The poller resolved it from
// the message's connection id before anything was stored.
type BusinessLink = connector.ChatLink

// ParseChatMessage maps one raw envelope (connector.go's BuildRawEnvelope) to the
// chat message it carries and the binding it files under. link is nil for a
// message to the bot itself and set for one that arrived through a business
// link. The worker classifies a my_chat_member update before it gets here.
// A shape this function does not map (an edited_message, or a message kind that
// does not match link) answers ErrSkip.
func ParseChatMessage(raw connector.RawRecord, link *BusinessLink) (connector.ChatBinding, connector.ChatMessage, error) {
	update, binding, err := decodeChatUpdate(raw, link)
	if err != nil {
		return connector.ChatBinding{}, connector.ChatMessage{}, err
	}
	msg := update.Message
	if link != nil {
		msg = update.BusinessMessage
	}
	chat, err := chatMessageOf(msg, update.UpdateID, link)
	if err != nil {
		return connector.ChatBinding{}, connector.ChatMessage{}, err
	}
	return binding, chat, nil
}

// ParseChatChanges maps one raw envelope that reports an edit or a deletion
// to the changes capture.ChangeChatMessage applies and the binding they apply
// under. link is nil for an update to the bot itself and set for one that
// arrived through a business link. An edit restates the whole message, so it
// can file a message whose original has not arrived. A deletion carries the
// ids of every message it removes. Any other shape answers ErrSkip.
func ParseChatChanges(raw connector.RawRecord, link *BusinessLink) (connector.ChatBinding, []connector.ChatChange, error) {
	update, binding, err := decodeChatUpdate(raw, link)
	if err != nil {
		return connector.ChatBinding{}, nil, err
	}
	if link != nil && update.DeletedBusiness != nil {
		changes, err := deletionChanges(update.DeletedBusiness, update.UpdateID)
		return binding, changes, err
	}
	msg := update.EditedMessage
	if link != nil {
		msg = update.EditedBusinessMessage
	}
	chat, err := chatMessageOf(msg, update.UpdateID, link)
	if err != nil {
		return connector.ChatBinding{}, nil, err
	}
	if msg.EditDate <= 0 {
		return connector.ChatBinding{}, nil, fmt.Errorf("telegram: update %d is an edit that names no edit time: %w", update.UpdateID, connector.ErrSkip)
	}
	edited := time.Unix(msg.EditDate, 0).UTC()
	chat.Revision, chat.RevisionSeq = edited, update.UpdateID
	return binding, []connector.ChatChange{{
		Kind: connector.ChatEdited, ConversationID: chat.ConversationID, IdempotencyKey: chat.IdempotencyKey,
		Body: chat.Body, Revision: edited, RevisionSeq: update.UpdateID, Restated: &chat,
	}}, nil
}

// deletionChanges is one deletion per message id a deleted_business_messages
// update names. The chat is the customer's, whoever typed the message.
func deletionChanges(deleted *deletedBusinessMessages, updateID int64) ([]connector.ChatChange, error) {
	if !deleted.Chat.isPrivate() {
		return nil, fmt.Errorf("telegram: update %d deletes messages of a %q chat, not a private one: %w",
			updateID, deleted.Chat.Type, connector.ErrSkip)
	}
	changes := make([]connector.ChatChange, 0, len(deleted.MessageIDs))
	for _, id := range deleted.MessageIDs {
		changes = append(changes, connector.ChatChange{
			Kind: connector.ChatDeleted, ConversationID: fmt.Sprintf("%d", deleted.Chat.ID), IdempotencyKey: fmt.Sprintf("%d", id),
		})
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("telegram: update %d deletes no message: %w", updateID, connector.ErrSkip)
	}
	return changes, nil
}

// decodeChatUpdate decodes the envelope and picks the binding its link names.
func decodeChatUpdate(raw connector.RawRecord, link *BusinessLink) (telegramUpdate, connector.ChatBinding, error) {
	var env ingestEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return telegramUpdate{}, connector.ChatBinding{}, fmt.Errorf("telegram: decoding the ingest envelope: %w", err)
	}
	if env.BotID == "" {
		return telegramUpdate{}, connector.ChatBinding{}, errors.New("telegram: normalize envelope carries no bot id")
	}
	var update telegramUpdate
	if err := json.Unmarshal(env.Update, &update); err != nil {
		return telegramUpdate{}, connector.ChatBinding{}, fmt.Errorf("telegram: decoding the update: %w", err)
	}
	if strings.Contains(env.BotID, ":") || (link != nil && (link.AccountID == "" || strings.Contains(link.AccountID, ":"))) {
		return telegramUpdate{}, connector.ChatBinding{}, errors.New("telegram: a bot or account id is empty or holds a colon, so the message key would be ambiguous")
	}
	binding := botBinding(env.BotID)
	if link != nil {
		binding = businessBinding(env.BotID, *link)
	}
	return update, binding, nil
}

// chatMessageOf maps one Telegram message to the chat message it states. A
// missing message, one outside a private chat and one with no addressable
// sender answer ErrSkip.
func chatMessageOf(msg *telegramMessage, updateID int64, link *BusinessLink) (connector.ChatMessage, error) {
	if msg == nil {
		return connector.ChatMessage{}, fmt.Errorf("telegram: update %d carries no message of the kind its link names: %w",
			updateID, connector.ErrSkip)
	}
	if !msg.Chat.isPrivate() {
		return connector.ChatMessage{}, fmt.Errorf("telegram: update %d is in a %q chat, not a private one: %w",
			updateID, msg.Chat.Type, connector.ErrSkip)
	}
	// A Telegram account id is positive. A message with no `from` (an anonymous
	// group admin posts under `sender_chat`) decodes to sender id 0. Every such
	// sender would share that key and collapse onto one Contact. A negative id is
	// a chat, not a user. The chat-scope gate above does not decide whether the
	// sender is addressable.
	if msg.From.ID <= 0 {
		return connector.ChatMessage{}, fmt.Errorf("telegram: update %d carries no addressable sender: %w",
			updateID, connector.ErrSkip)
	}
	chat := connector.ChatMessage{
		ConversationID: fmt.Sprintf("%d", msg.Chat.ID),
		IdempotencyKey: fmt.Sprintf("%d", msg.MessageID),
		Direction:      connector.DirectionInbound,
		Counterparty: connector.ChatParty{
			UserID:      fmt.Sprintf("%d", msg.From.ID),
			DisplayName: telegramDisplayName(msg.From),
			Username:    msg.From.Username,
		},
		Body:       messageBody(msg),
		OccurredAt: time.Unix(msg.Date, 0).UTC(),
		Forward:    chatForward(msg.ForwardOrigin),
	}
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.MessageID > 0 {
		chat.ReplyToID = fmt.Sprintf("%d", msg.ReplyToMessage.MessageID)
	}
	chat.SharedContact = sharedContactOf(msg.Contact)
	if link != nil {
		// Through a business link the other end of the private chat is the chat
		// itself, whoever typed. The customer typed, or the owner did in their
		// own Telegram app.
		chat.Counterparty.UserID = chat.ConversationID
		if fmt.Sprintf("%d", msg.From.ID) == link.AccountID {
			chat.Direction = connector.DirectionOutbound
			chat.Forward = nil
			chat.Counterparty.DisplayName = chatDisplayName(msg.Chat)
			chat.Counterparty.Username = msg.Chat.Username
		}
	}
	return chat, nil
}

// sharedContactOf reads the card of a contact message. A card for someone who
// is not on Telegram carries a phone number and no account id.
func sharedContactOf(raw json.RawMessage) *connector.ChatSharedContact {
	if len(raw) == 0 {
		return nil
	}
	var card struct {
		PhoneNumber string `json:"phone_number"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		UserID      int64  `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &card); err != nil {
		return nil
	}
	out := &connector.ChatSharedContact{DisplayName: strings.TrimSpace(card.FirstName + " " + card.LastName)}
	if card.PhoneNumber != "" {
		out.Phones = []string{card.PhoneNumber}
	}
	if card.UserID > 0 {
		out.UserID = fmt.Sprintf("%d", card.UserID)
	}
	return out
}

// messageBody is what the timeline shows for one message. Telegram carries a
// media message's words in `caption`, never in `text`, so reading `text` alone
// files a photo-with-a-caption as an activity with an empty body — the rep sees
// the Contact and a blank line where the customer's sentence was.
//
// A message with no words at all reads as a bracketed placeholder naming what
// arrived, deliberately NOT ErrSkip: the customer did reach out, and skipping
// leaves a timeline that says nothing arrived while the reply box offers to
// answer it — the same silent gap an empty body is. The words are all this
// connector can show (fetching the media itself is out of scope). The
// verbatim update is not carried here: the raw_capture row the poll wrote is its
// single owner.
func messageBody(msg *telegramMessage) string {
	if msg.Text != "" {
		return msg.Text
	}
	if msg.Caption != "" {
		return msg.Caption
	}
	return "[" + msg.mediaKind() + "]"
}

// mediaKind names the attachment a wordless message carries. Animation is
// tested first because Telegram sends a GIF as an animation AND a document,
// and the animation is the truer description of the two.
func (m telegramMedia) mediaKind() string {
	switch {
	case m.Animation != nil:
		return "animation"
	case m.Photo != nil:
		return "photo"
	case m.Sticker != nil:
		return "sticker"
	case m.Voice != nil:
		return "voice message"
	case m.VideoNote != nil:
		return "video note"
	case m.Video != nil:
		return "video"
	case m.Audio != nil:
		return "audio"
	case m.Document != nil:
		return "document"
	case m.Location != nil:
		return "location"
	case m.Contact != nil:
		return "contact"
	}
	// Telegram keeps adding message kinds (a poll, a venue, a shared story). One
	// this package cannot name is still a customer reaching out, so it reads as
	// an unnamed attachment rather than as an empty activity.
	return "attachment"
}

// chatDisplayName is telegramDisplayName for the private chat's own name fields.
func chatDisplayName(c telegramChat) string {
	return telegramDisplayName(telegramUser{FirstName: c.FirstName, LastName: c.LastName, Username: c.Username})
}

// telegramDisplayName renders the sender's name from Telegram's separate
// first/last name fields, falling back to the @username Telegram guarantees
// every account has. Untrusted text, exactly like a mail header's display
// name — a human typed it, and nothing here sanitizes it.
func telegramDisplayName(u telegramUser) string {
	name := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if name != "" {
		return name
	}
	return u.Username
}

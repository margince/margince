// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProviderDispact is the one chat a channel can be linked in today.
const ProviderDispact = "dispact"

// Chat is what this package asks of a team chat, as its bot. dispact.Client is
// the implementation; a test passes a client over a fake server.
type Chat interface {
	Me(ctx context.Context) (dispact.Bot, error)
	Channel(ctx context.Context, slug string) (dispact.Channel, error)
	Channels(ctx context.Context) ([]dispact.Channel, error)
	CreateChannel(ctx context.Context, externalKey, name string) (dispact.Channel, error)
	AddMembers(ctx context.Context, slug string, emails []string) (dispact.MemberResult, error)
	Post(ctx context.Context, slug, text, clientMsgID, threadTS string) (dispact.Posted, error)
	Update(ctx context.Context, slug, ts, text string) error
	History(ctx context.Context, slug string, after int64, limit int) (dispact.HistoryPage, error)
}

// Session is the live bot of this installation and a client acting as it.
type Session struct {
	ConnectionID ids.UUID
	TeamID       string
	Address      string
	Chat         Chat
}

// Chats opens a session as the installation's live bot. It answers
// ErrBotNotConnected when no bot is connected.
type Chats interface {
	Open(ctx context.Context) (Session, error)
}

var (
	// ErrBotNotConnected means no chat bot is connected, so nothing can be linked.
	ErrBotNotConnected = errors.New("chatlinks: no chat bot is connected")
	// ErrChannelNotFound means the named channel does not exist in the chat.
	ErrChannelNotFound = errors.New("chatlinks: the channel was not found")
	// ErrChannelNotPrivate means only a private channel may carry a record.
	ErrChannelNotPrivate = errors.New("chatlinks: the channel is public")
	// ErrBotNotMember means the bot must be in the channel before it is linked.
	ErrBotNotMember = errors.New("chatlinks: the bot is not in the channel")
	// ErrChannelAlreadyLinked means the channel carries another record.
	ErrChannelAlreadyLinked = errors.New("chatlinks: the channel is linked to another record")
	// ErrRecordAlreadyLinked means the record has a channel already.
	ErrRecordAlreadyLinked = errors.New("chatlinks: the record already has a channel")
	// ErrLinkNotFound means the record has no channel.
	ErrLinkNotFound = errors.New("chatlinks: the record has no channel")
	// ErrMessageNotFound means the named message is not in the record's channel.
	ErrMessageNotFound = errors.New("chatlinks: the message was not found in the channel")
	// ErrThreadTooLong means the thread is longer than one filing reads, so it is
	// not filed in part.
	ErrThreadTooLong = errors.New("chatlinks: the thread is too long to file")
	// ErrNoSummaryModel means no AI model is bound to summarise chat talk.
	ErrNoSummaryModel = errors.New("chatlinks: no AI model summarises chat talk")
	// ErrSummaryFailed means the model could not summarise the talk; nothing was filed.
	ErrSummaryFailed = errors.New("chatlinks: the talk could not be summarised")
	// ErrChatUnreachable means the chat could not be reached; nothing was written.
	ErrChatUnreachable = errors.New("chatlinks: the chat could not be reached")
)

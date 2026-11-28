// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/teamchat"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Native provider identifiers stay consistent across personal consent and channel links.
const (
	ProviderDispact = teamchat.Dispact
	ProviderSlack   = teamchat.Slack
	ProviderTeams   = teamchat.Teams
)

// IsChatProvider recognizes the native provider identifiers used by channel links.
func IsChatProvider(provider string) bool {
	return teamchat.Supported(provider)
}

// Chat is the channel API required by the link engine.
type Chat interface {
	Me(ctx context.Context) (chatwire.Bot, error)
	Channel(ctx context.Context, slug string) (chatwire.Channel, error)
	Channels(ctx context.Context) ([]chatwire.Channel, error)
	CreateChannel(ctx context.Context, externalKey, name string) (chatwire.Channel, error)
	AddMembers(ctx context.Context, slug string, emails []string) (chatwire.MemberResult, error)
	Post(ctx context.Context, slug, text, clientMsgID, threadTS string) (chatwire.Posted, error)
	Update(ctx context.Context, slug, ts, text string) error
	History(ctx context.Context, slug string, after int64, limit int) (chatwire.HistoryPage, error)
}

// Session is the live bot of this installation and a client acting as it.
type Session struct {
	Provider     string
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

// Provider failures preserve consent, membership, source and delivery distinctions for repair.
var (
	ErrMessengerDeliveryNeedsReview = errors.New("chatlinks: review the unresolved delivery before another send")
	ErrAudienceNeedsReview          = chatwire.ErrAudienceNeedsReview
	ErrMembersConsentRequired       = chatwire.ErrMembersConsentRequired
	ErrChannelIdentityNeedsReview   = errors.New("chatlinks: confirm the immutable channel identity before syncing")
	// ErrBotNotConnected means no chat bot is connected, so nothing can be linked.
	ErrBotNotConnected = errors.New("chatlinks: no chat bot is connected")
	// ErrChannelNotFound means the named channel does not exist in the chat.
	ErrChannelNotFound = errors.New("chatlinks: the channel was not found")
	// ErrChannelNameRefused means the chat will not make a channel under the
	// record's name, even in the form derived from the record.
	ErrChannelNameRefused = errors.New("chatlinks: the chat refused the channel name")
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
	ErrThreadTooLong = chatwire.ErrThreadTooLong
	// ErrNoSummaryModel means no AI model is bound to summarise chat talk.
	ErrNoSummaryModel = errors.New("chatlinks: no AI model summarises chat talk")
	// ErrSummaryFailed means the model could not summarise the talk; nothing was filed.
	ErrSummaryFailed = errors.New("chatlinks: the talk could not be summarised")
	// ErrChatUnreachable means the chat could not be reached; nothing was written.
	ErrChatUnreachable = errors.New("chatlinks: the chat could not be reached")
)

// ProviderChats resolves the workspace bot independently for each native provider.
type ProviderChats interface {
	OpenProvider(context.Context, string) (Session, error)
}

// OpenProvider opens the selected installation instead of guessing another provider.
func OpenProvider(ctx context.Context, chats Chats, provider string) (Session, error) {
	if provider == "" {
		provider = ProviderDispact
	}
	if !IsChatProvider(provider) {
		return Session{}, ErrBotNotConnected
	}
	if native, ok := chats.(ProviderChats); ok {
		return native.OpenProvider(ctx, provider)
	}
	if provider != ProviderDispact {
		return Session{}, ErrBotNotConnected
	}
	return chats.Open(ctx)
}

// LinkedChannel checks the stored immutable identity against current provider state.
func LinkedChannel(ctx context.Context, chat Chat, link Link) (chatwire.Channel, error) {
	_, canonical := chat.(chatwire.RootReader)
	if canonical && link.Provider == ProviderDispact && !link.NativeIdentity {
		return chatwire.Channel{}, ErrChannelIdentityNeedsReview
	}
	channel, err := LiveChannel(ctx, chat, link.ChannelSlug)
	if err != nil {
		return chatwire.Channel{}, err
	}
	if canonical && link.Provider == ProviderDispact && channel.NativeID != "" && channel.NativeID != link.ChannelSlug {
		return chatwire.Channel{}, ErrChannelIdentityNeedsReview
	}
	return channel, nil
}

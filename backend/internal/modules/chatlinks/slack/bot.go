// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// Bot performs channel synchronization through an installed application's credentials.
type Bot struct {
	client  *Client
	history historyPacing
}

// NewBot binds channel operations to the installed application's identity.
func NewBot(httpClient messaging.Doer, token, workspace, member string) *Bot {
	client := New(httpClient, token, workspace, member)
	client.bot = true
	return &Bot{client: client}
}

// Me resolves the native installation identity behind the credential.
func (b *Bot) Me(ctx context.Context) (chatwire.Bot, error) {
	raw, err := b.client.call(ctx, http.MethodGet, "auth.test", nil)
	if err != nil {
		return chatwire.Bot{}, botFailure(err)
	}
	var proof struct {
		User string `json:"user_id"`
		Bot  string `json:"bot_id"`
		Team string `json:"team_id"`
		Name string `json:"team"`
	}
	if json.Unmarshal(raw, &proof) != nil || proof.Bot == "" || proof.User == "" || proof.Team == "" || proof.Team != b.client.workspace || proof.User != b.client.user {
		return chatwire.Bot{}, chatwire.ErrTokenRejected
	}
	return chatwire.Bot{ID: proof.User, TeamID: proof.Team, Name: proof.Name}, nil
}

// Channel retains native channel identity independently of its mutable display name.
func (b *Bot) Channel(ctx context.Context, id string) (chatwire.Channel, error) {
	raw, err := b.client.call(ctx, http.MethodGet, "conversations.info?"+url.Values{channelKey: {id}}.Encode(), nil)
	if err != nil {
		return chatwire.Channel{}, botFailure(err)
	}
	var response struct {
		Channel channel `json:"channel"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Channel.ID != id {
		return chatwire.Channel{}, chatwire.ErrUnreachable
	}
	ch := response.Channel
	if ch.IM || ch.MPIM {
		return chatwire.Channel{}, chatwire.ErrRequestRejected
	}
	posting := ch.Private
	return chatwire.Channel{PostingAllowed: &posting, Slug: ch.ID, Name: ch.Name, IsPrivate: ch.Private, IsArchived: ch.Archived, BotIsMember: ch.Member}, nil
}

// Channels lists native rooms available to the current credential.
func (b *Bot) Channels(ctx context.Context) ([]chatwire.Channel, error) {
	out := []chatwire.Channel{}
	kinds := "private_channel,public_channel"
	cursor := ""
	seen := map[string]bool{}
	for range 100 {
		raw, err := b.client.call(ctx, http.MethodGet, "conversations.list?"+url.Values{"types": {kinds}, "exclude_archived": {"true"}, limitKey: {"100"}, cursorKey: {cursor}}.Encode(), nil)
		if errors.Is(err, messaging.ErrConsentRequired) && kinds != "private_channel" {
			kinds = "private_channel"
			cursor = ""
			out = []chatwire.Channel{}
			clear(seen)
			continue
		}
		if err != nil {
			return nil, botFailure(err)
		}
		var page struct {
			Channels []channel `json:"channels"`
			Metadata struct {
				Next string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return nil, chatwire.ErrUnreachable
		}
		for _, ch := range page.Channels {
			if ch.Member && !ch.Archived && !ch.IM && !ch.MPIM {
				posting := ch.Private
				out = append(out, chatwire.Channel{Slug: ch.ID, Name: ch.Name, IsPrivate: ch.Private, BotIsMember: true, PostingAllowed: &posting})
			}
		}
		cursor = page.Metadata.Next
		if cursor == "" {
			return out, nil
		}
		if seen[cursor] {
			return nil, chatwire.ErrUnreachable
		}
		seen[cursor] = true
	}
	return nil, chatwire.ErrUnreachable
}

// CreateChannel creates an internal destination when the provider permits it.
func (b *Bot) CreateChannel(ctx context.Context, key, name string) (chatwire.Channel, error) {
	digest := sha256.Sum256([]byte(key))
	var slug strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			slug.WriteRune(r)
		} else {
			slug.WriteByte('-')
		}
		if slug.Len() >= 60 {
			break
		}
	}
	body, err := json.Marshal(struct {
		Name    string `json:"name"`
		Private bool   `json:"is_private"`
	}{Name: strings.Trim(slug.String(), "-") + "-" + hex.EncodeToString(digest[:6]), Private: true})
	if err != nil {
		return chatwire.Channel{}, chatwire.ErrRequestRejected
	}
	raw, err := b.client.call(ctx, http.MethodPost, "conversations.create", body)
	if err != nil {
		return chatwire.Channel{}, botFailure(err)
	}
	var response struct {
		Channel channel `json:"channel"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Channel.ID == "" {
		return chatwire.Channel{}, messaging.ErrDeliveryUnknown
	}
	return b.Channel(ctx, response.Channel.ID)
}

// AddMembers reports native membership changes separately from unresolved addresses.
func (b *Bot) AddMembers(ctx context.Context, id string, emails []string) (chatwire.MemberResult, error) {
	out := chatwire.MemberResult{}
	for _, email := range emails {
		member, err := b.client.ResolvePerson(ctx, email)
		if errors.Is(err, messaging.ErrUnavailable) {
			out.NotInChat = append(out.NotInChat, email)
			continue
		}
		if err != nil {
			return out, botFailure(err)
		}
		if !member.Active || member.External {
			out.NotInChat = append(out.NotInChat, email)
			continue
		}
		body, err := json.Marshal(struct {
			Channel string `json:"channel"`
			Users   string `json:"users"`
		}{id, member.ID})
		if err != nil {
			return out, chatwire.ErrRequestRejected
		}
		if _, err = b.client.call(ctx, http.MethodPost, "conversations.invite", body); err != nil {
			return out, botFailure(err)
		}
		out.Added = append(out.Added, email)
	}
	return out, nil
}

// Audience resolves effective readers for comparison with the record team.
func (b *Bot) Audience(ctx context.Context, id string) (messaging.Conversation, error) {
	ch, err := b.Channel(ctx, id)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if ch.IsArchived || !ch.BotIsMember {
		return messaging.Conversation{}, chatwire.ErrNotMember
	}
	if !ch.IsPrivate {
		return messaging.Conversation{}, chatwire.ErrAudienceNeedsReview
	}
	audience, err := b.client.Conversation(ctx, id)
	if err != nil {
		return messaging.Conversation{}, botFailure(err)
	}
	if !audience.AudienceKnown || audience.External {
		return messaging.Conversation{}, chatwire.ErrAudienceNeedsReview
	}
	for _, member := range audience.Members {
		if !member.Active || member.Bot && member.ID != b.client.user {
			return messaging.Conversation{}, chatwire.ErrAudienceNeedsReview
		}
	}
	return audience, nil
}

// Post returns the provider receipt needed for later shared-message updates.
func (b *Bot) Post(ctx context.Context, id, text, key, root string) (chatwire.Posted, error) {
	audience, err := b.Audience(ctx, id)
	if err != nil {
		return chatwire.Posted{}, err
	}
	expected, err := messaging.AudienceKey(audience)
	if err != nil {
		return chatwire.Posted{}, botFailure(err)
	}
	return b.PostReviewed(ctx, id, text, key, root, expected)
}

// PostReviewed checks the expected audience before a shared post.
func (b *Bot) PostReviewed(ctx context.Context, id, text, key, root, expected string) (chatwire.Posted, error) {
	receipt, err := b.client.Send(ctx, messaging.Send{Conversation: id, Text: text, RequestID: key, Root: root, ExpectedAudience: expected})
	return chatwire.Posted{ID: receipt.ID}, botFailure(err)
}

// Update replaces a shared provider message using its native receipt.
func (b *Bot) Update(ctx context.Context, id, message, text string) error {
	audience, err := b.Audience(ctx, id)
	if err != nil {
		return err
	}
	expected, err := messaging.AudienceKey(audience)
	if err != nil {
		return err
	}
	return b.UpdateReviewed(ctx, id, message, text, expected)
}

// UpdateReviewed checks the reviewed audience before replacing a shared message.
func (b *Bot) UpdateReviewed(ctx context.Context, id, message, text, expected string) error {
	audience, err := b.Audience(ctx, id)
	if err != nil {
		return err
	}
	key, err := messaging.AudienceKey(audience)
	if err != nil || expected == "" || key != expected {
		return messaging.ErrInvalid
	}
	body, err := json.Marshal(struct {
		Channel     string `json:"channel"`
		TS          string `json:"ts"`
		Text        string `json:"text"`
		Markdown    bool   `json:"mrkdwn"`
		Parse       string `json:"parse"`
		UnfurlLinks bool   `json:"unfurl_links"`
		UnfurlMedia bool   `json:"unfurl_media"`
	}{Channel: id, TS: message, Text: text, Parse: "none"})
	if err != nil {
		return chatwire.ErrRequestRejected
	}
	_, err = b.client.call(ctx, http.MethodPost, "chat.update", body)
	return botFailure(err)
}

func botFailure(err error) error {
	if errors.Is(err, messaging.ErrConsentRequired) {
		return errors.Join(chatwire.ErrHistoryProfileNeedsReview, err)
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, messaging.ErrForbidden) {
		return errors.Join(chatwire.ErrNotMember, err)
	}
	if refusal, ok := errors.AsType[*apiRefusal](err); ok {
		if refusal.code == errorNotInChannel {
			return errors.Join(chatwire.ErrNotMember, err)
		}
		if refusal.code == errorChannelNotFound {
			return errors.Join(chatwire.ErrChannelGone, err)
		}
	}
	if errors.Is(err, messaging.ErrUnauthorized) {
		return errors.Join(chatwire.ErrTokenRejected, err)
	}
	return err
}

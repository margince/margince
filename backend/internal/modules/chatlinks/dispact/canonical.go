// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	channelKind        = "channel"
	cursorKey          = "cursor"
	limitKey           = "limit"
	privateChannelKind = "private"
)

// CanonicalClient uses immutable channel identities and canonical source reads.
type CanonicalClient struct{ *Client }

// CanonicalAvailable requires the provider capabilities needed to revise complete source recaps.
func CanonicalAvailable(bot Bot) bool {
	for _, capability := range []string{"immutable_channel_ids", "canonical_threads"} {
		if !slices.Contains(bot.Capabilities, capability) {
			return false
		}
	}
	return true
}

// NewCanonical adds canonical source operations to the existing Dispact transport.
func NewCanonical(client *Client) *CanonicalClient { return &CanonicalClient{Client: client} }

// Channel retains native channel identity independently of its mutable display name.
func (c *CanonicalClient) Channel(ctx context.Context, channel string) (Channel, error) {
	room, err := c.Client.Channel(ctx, immutableChannelRef(channel))
	if err != nil {
		return Channel{}, err
	}
	if room.NativeID == "" {
		return Channel{}, messaging.ErrUnsupported
	}
	room.Slug = room.NativeID
	return room, nil
}

// Channels lists native rooms available to the current credential.
func (c *CanonicalClient) Channels(ctx context.Context) ([]Channel, error) {
	rooms, err := c.Client.Channels(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rooms {
		if rooms[i].NativeID == "" {
			return nil, messaging.ErrUnsupported
		}
		rooms[i].Slug = rooms[i].NativeID
	}
	return rooms, nil
}

// CreateChannel creates an internal destination when the provider permits it.
func (c *CanonicalClient) CreateChannel(ctx context.Context, key, name string) (Channel, error) {
	room, err := c.Client.CreateChannel(ctx, key, name)
	if err != nil {
		return Channel{}, err
	}
	if room.NativeID == "" {
		return Channel{}, messaging.ErrUnsupported
	}
	room.Slug = room.NativeID
	return room, nil
}

// Audience reads and verifies the complete private channel roster.
func (c *CanonicalClient) Audience(ctx context.Context, channel string) (messaging.Conversation, error) {
	room, err := c.Channel(ctx, channel)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if room.IsArchived || !room.BotIsMember {
		return messaging.Conversation{}, chatwire.ErrNotMember
	}
	bot, err := c.Me(ctx)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if !room.IsPrivate {
		return messaging.Conversation{}, chatwire.ErrAudienceNeedsReview
	}
	if !slices.Contains(bot.Capabilities, "channel_members") {
		return messaging.Conversation{}, errors.Join(chatwire.ErrAudienceNeedsReview, chatwire.ErrMembersConsentRequired)
	}
	out := messaging.Conversation{ID: room.NativeID, Name: room.Name, Kind: channelKind, SupportsThreads: true}
	cursor, revision := "", ""
	seen := map[string]bool{}
	for range 10 {
		var page canonicalMemberPage
		page, err = c.canonicalMembersPage(ctx, room.NativeID, cursor, revision)
		if err != nil {
			return messaging.Conversation{}, err
		}
		revision = page.Revision
		out.Members, err = appendCanonicalMembers(out.Members, page, bot.ID)
		if err != nil {
			return messaging.Conversation{}, err
		}
		if page.Metadata.Next == "" {
			return completeCanonicalAudience(out, page, bot.ID)
		}
		if page.Complete || !page.More || seen[page.Metadata.Next] {
			return messaging.Conversation{}, chatwire.ErrUnreachable
		}
		seen[page.Metadata.Next] = true
		cursor = page.Metadata.Next
	}
	return messaging.Conversation{}, chatwire.ErrUnreachable
}

// Post returns the provider receipt needed for later shared-message updates.
func (c *CanonicalClient) Post(ctx context.Context, id, text, key, root string) (chatwire.Posted, error) {
	audience, err := c.Audience(ctx, id)
	if err != nil {
		return chatwire.Posted{}, err
	}
	expected, err := messaging.AudienceKey(audience)
	if err != nil {
		return chatwire.Posted{}, err
	}
	return c.PostReviewed(ctx, id, text, key, root, expected)
}

// PostReviewed checks the expected audience before a shared post.
func (c *CanonicalClient) PostReviewed(ctx context.Context, id, text, key, root, expected string) (chatwire.Posted, error) {
	if root != "" {
		page, err := c.Thread(ctx, id, root, "")
		if err != nil {
			return chatwire.Posted{}, err
		}
		if page.Deleted {
			return chatwire.Posted{}, chatwire.ErrChannelGone
		}
		writeRef := ""
		for _, message := range page.Messages {
			if message.NativeID == root && !message.Deleted {
				writeRef = message.WriteRef
				break
			}
		}
		if writeRef == "" {
			return chatwire.Posted{}, chatwire.ErrUnreachable
		}
		root = writeRef
	}
	if err := c.reviewedAudience(ctx, id, expected); err != nil {
		return chatwire.Posted{}, err
	}
	return c.Client.Post(ctx, immutableChannelRef(id), text, key, root)
}

// UpdateReviewed checks the reviewed audience before replacing a shared message.
func (c *CanonicalClient) UpdateReviewed(ctx context.Context, id, message, text, expected string) error {
	if err := c.reviewedAudience(ctx, id, expected); err != nil {
		return err
	}
	return c.Update(ctx, immutableChannelRef(id), message, text)
}

func (c *CanonicalClient) reviewedAudience(ctx context.Context, id, expected string) error {
	audience, err := c.Audience(ctx, id)
	if err != nil {
		return err
	}
	key, err := messaging.AudienceKey(audience)
	if err != nil || expected == "" || key != expected {
		return messaging.ErrInvalid
	}
	return nil
}

func immutableChannelRef(value string) string {
	if _, err := ids.Parse(value); err == nil {
		return "id:" + value
	}
	return value
}

// AddMembers reports native membership changes separately from unresolved addresses.
func (c *CanonicalClient) AddMembers(ctx context.Context, channel string, emails []string) (MemberResult, error) {
	return c.Client.AddMembers(ctx, immutableChannelRef(channel), emails)
}

// History serves the legacy sequence interface where the provider supports it.
func (c *CanonicalClient) History(ctx context.Context, channel string, after int64, limit int) (HistoryPage, error) {
	return c.Client.History(ctx, immutableChannelRef(channel), after, limit)
}

type canonicalMemberPage struct {
	Channel  string `json:"channel_id"`
	Revision string `json:"revision"`
	Members  []struct {
		ID       string `json:"id"`
		Name     string `json:"display_name"`
		Email    string `json:"email"`
		Active   bool   `json:"is_active"`
		Guest    bool   `json:"is_guest"`
		Bot      bool   `json:"is_bot"`
		Imported bool   `json:"is_import_placeholder"`
	} `json:"members"`
	Complete bool `json:"complete"`
	More     bool `json:"has_more"`
	Metadata struct {
		Next string `json:"next_cursor"`
	} `json:"response_metadata"`
}

func appendCanonicalMembers(members []messaging.Member, page canonicalMemberPage, bot string) ([]messaging.Member, error) {
	for _, member := range page.Members {
		ownBot := member.ID == bot && member.Bot
		members = append(members, messaging.Member{Bot: member.Bot, ID: member.ID, Name: member.Name, Email: member.Email, Active: member.Active, External: member.Guest || member.Imported})
		if !member.Active || member.Guest || member.Imported || member.Bot && !ownBot {
			return nil, chatwire.ErrAudienceNeedsReview
		}
	}
	return members, nil
}

func (c *CanonicalClient) canonicalMembersPage(ctx context.Context, channel, cursor, revision string) (canonicalMemberPage, error) {
	var page canonicalMemberPage
	path := "/api/bot/channels/" + url.PathEscape(immutableChannelRef(channel)) + "/members?" + url.Values{limitKey: {"100"}, cursorKey: {cursor}}.Encode()
	if err := c.call(ctx, http.MethodGet, path, nil, &page); err != nil {
		var refusal *CodeError
		if errors.As(err, &refusal) && refusal.Code == "missing_scope" {
			return page, errors.Join(chatwire.ErrAudienceNeedsReview, chatwire.ErrMembersConsentRequired)
		}
		return page, err
	}
	if page.Channel != channel || page.Revision == "" || revision != "" && revision != page.Revision {
		return page, chatwire.ErrUnreachable
	}
	return page, nil
}

func completeCanonicalAudience(out messaging.Conversation, page canonicalMemberPage, bot string) (messaging.Conversation, error) {
	if !page.Complete || page.More {
		return messaging.Conversation{}, chatwire.ErrUnreachable
	}
	out.AudienceKnown = true
	if !messaging.HasActiveMember(out.Members, bot) {
		return messaging.Conversation{}, chatwire.ErrNotMember
	}
	if _, err := messaging.AudienceKey(out); err != nil {
		return messaging.Conversation{}, err
	}
	return out, nil
}

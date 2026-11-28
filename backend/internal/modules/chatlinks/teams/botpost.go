// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// ConversationReference retains the Bot Framework address needed for a channel thread.
type ConversationReference struct {
	Conversation string
	Activity     string
	Channel      string
	Root         string
	App          string
	Tenant       string
}

// ThreadReference resolves Bot Framework delivery addressing for a native thread.
type ThreadReference func(context.Context, string, string) (ConversationReference, error)

// WithThreadReferences resolves delivery addressing from verified native thread evidence.
func (b *Bot) WithThreadReferences(resolve ThreadReference) *Bot {
	b.references = resolve
	return b
}

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type botActivity struct {
	Type        string         `json:"type"`
	Text        string         `json:"text"`
	TextFormat  string         `json:"textFormat"`
	ReplyTo     string         `json:"replyToId,omitempty"`
	ChannelData botChannelData `json:"channelData"`
}

type botAccount struct {
	ID string `json:"id"`
}

type botChannelData struct {
	Channel botAccount `json:"channel"`
	Team    botAccount `json:"team"`
	Tenant  botAccount `json:"tenant"`
}

// Post returns the provider receipt needed for later shared-message updates.
func (b *Bot) Post(ctx context.Context, id, text, key, root string) (chatwire.Posted, error) {
	audience, err := b.Audience(ctx, id)
	if err != nil {
		return chatwire.Posted{}, err
	}
	expected, err := messaging.AudienceKey(audience)
	if err != nil {
		return chatwire.Posted{}, err
	}
	return b.PostReviewed(ctx, id, text, key, root, expected)
}

// PostReviewed checks the expected audience before a shared post.
func (b *Bot) PostReviewed(ctx context.Context, id, text, key, root, expected string) (chatwire.Posted, error) {
	if strings.TrimSpace(text) == "" || len(text) > 90000 || key == "" {
		return chatwire.Posted{}, messaging.ErrInvalid
	}
	if err := b.authorizePost(ctx, id, expected); err != nil {
		return chatwire.Posted{}, err
	}
	_, channel, _ := strings.Cut(id, "/")
	activity := botActivity{Type: messageKind, Text: text, TextFormat: "plain", ChannelData: botChannelData{Channel: botAccount{ID: channel}, Team: botAccount{ID: b.nativeTeam}, Tenant: botAccount{ID: b.graph.tenant}}}
	if root != "" {
		return b.reply(ctx, id, root, activity)
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	body, err := json.Marshal(struct {
		IsGroup     bool           `json:"isGroup"`
		Bot         botAccount     `json:"bot"`
		ChannelData botChannelData `json:"channelData"`
		Activity    botActivity    `json:"activity"`
	}{IsGroup: true, Bot: botAccount{ID: b.member}, ChannelData: activity.ChannelData, Activity: activity})
	if err != nil {
		return chatwire.Posted{}, messaging.ErrInvalid
	}
	raw, err := b.connector.Call(ctx, http.MethodPost, "/v3/conversations", body)
	if err != nil {
		return chatwire.Posted{}, teamsBotFailure(err)
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var response struct {
		ID       string `json:"id"`
		Activity string `json:"activityId"`
	}
	if json.Unmarshal(raw, &response) != nil || response.ID == "" || response.Activity == "" {
		return chatwire.Posted{}, messaging.ErrDeliveryUnknown
	}
	return b.posted(ConversationReference{Conversation: response.ID, Activity: response.Activity, Root: response.Activity, Channel: id, App: b.app, Tenant: b.graph.tenant})
}

func (b *Bot) authorizePost(ctx context.Context, id, expected string) error {
	channel, err := b.Channel(ctx, id)
	if err != nil {
		return err
	}
	if channel.PostingAllowed == nil || !*channel.PostingAllowed {
		return messaging.ErrUnsupported
	}
	if b.member == "" || b.nativeTeam == "" || b.connector.Token == "" || b.connector.Base == "" {
		return messaging.ErrUnauthorized
	}
	audience, err := b.Audience(ctx, id)
	if err != nil {
		return err
	}
	key, err := messaging.AudienceKey(audience)
	if err != nil || expected == "" || expected != key {
		return messaging.ErrInvalid
	}
	return nil
}

func (b *Bot) reply(ctx context.Context, id, root string, activity botActivity) (chatwire.Posted, error) {
	ref, err := b.reference(ctx, id, root)
	if err != nil {
		return chatwire.Posted{}, err
	}
	activity.ReplyTo = ref.Root
	body, err := json.Marshal(activity)
	if err != nil {
		return chatwire.Posted{}, messaging.ErrInvalid
	}
	path := "/v3/conversations/" + url.PathEscape(ref.Conversation) + "/activities/" + url.PathEscape(ref.Root)
	raw, err := b.connector.Call(ctx, http.MethodPost, path, body)
	if err != nil {
		return chatwire.Posted{}, teamsBotFailure(err)
	}
	var response struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &response) != nil || response.ID == "" {
		return chatwire.Posted{}, messaging.ErrDeliveryUnknown
	}
	ref.Activity = response.ID
	return b.posted(ref)
}

func (b *Bot) reference(ctx context.Context, id, root string) (ConversationReference, error) {
	var ref ConversationReference
	if strings.HasPrefix(root, "teams:") {
		ref, err := b.decodeReference(id, root)
		return ref, err
	}
	if b.references == nil {
		return ref, messaging.ErrUnavailable
	}
	ref, err := b.references(ctx, id, root)
	if err != nil {
		return ref, err
	}
	if !b.ownsReference(id, ref) || ref.Root != root {
		return ConversationReference{}, messaging.ErrInvalid
	}
	return ref, nil
}

func (b *Bot) ownsReference(id string, ref ConversationReference) bool {
	return ref.Channel == id && ref.App == b.app && ref.Tenant == b.graph.tenant && ref.Conversation != "" && ref.Activity != "" && ref.Root != ""
}

func (b *Bot) posted(ref ConversationReference) (chatwire.Posted, error) {
	raw, err := json.Marshal(ref)
	if err != nil {
		return chatwire.Posted{}, messaging.ErrDeliveryUnknown
	}
	return chatwire.Posted{ID: "teams:" + base64.RawURLEncoding.EncodeToString(raw)}, nil
}

func (b *Bot) decodeReference(id, encoded string) (ConversationReference, error) {
	var ref ConversationReference
	if !strings.HasPrefix(encoded, "teams:") || len(encoded) > 8192 {
		return ref, messaging.ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "teams:"))
	if err != nil || json.Unmarshal(raw, &ref) != nil || !b.ownsReference(id, ref) {
		return ConversationReference{}, messaging.ErrInvalid
	}
	return ref, nil
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
	if strings.TrimSpace(text) == "" || len(text) > 90000 {
		return messaging.ErrInvalid
	}
	if err := b.authorizePost(ctx, id, expected); err != nil {
		return err
	}
	ref, err := b.decodeReference(id, message)
	if err != nil {
		return err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	body, err := json.Marshal(struct {
		Type       string `json:"type"`
		Text       string `json:"text"`
		TextFormat string `json:"textFormat"`
	}{Type: "message", Text: text, TextFormat: "plain"})
	if err != nil {
		return messaging.ErrInvalid
	}
	_, err = b.connector.Call(ctx, http.MethodPut, "/v3/conversations/"+url.PathEscape(ref.Conversation)+"/activities/"+url.PathEscape(ref.Activity), body)
	return teamsBotFailure(err)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dispact is the client of the Dispact bot API: the calls a deal or
// project channel needs, made as the installation's bot.
package dispact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// TokenPrefix starts every Dispact app token.
const TokenPrefix = "gwapp_"

// HistoryPageLimit is the most messages one history page may ask for.
const HistoryPageLimit = 99

// responseLimit caps a body this client reads, so a misbehaving server cannot
// hold a worker's memory.
const responseLimit = 4 << 20

// HTTPDoer sends one request; a test passes an httptest client.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client talks to one Dispact install as one bot.
type Client struct {
	http    HTTPDoer
	address string
	token   string
}

// New builds a client for the install at address, authenticated by token.
func New(doer HTTPDoer, address, token string) *Client {
	return &Client{http: doer, address: strings.TrimRight(address, "/"), token: token}
}

// Bot is the bot the token belongs to.
type Bot struct {
	ID     string `json:"bot_id"`
	Name   string `json:"team"`
	TeamID string `json:"team_id"`
}

// Channel is one channel as the bot sees it.
type Channel struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	IsPrivate   bool   `json:"is_private"`
	IsArchived  bool   `json:"is_archived"`
	BotIsMember bool   `json:"bot_is_member"`
}

// Message is one channel message from the history read. TS is Dispact's
// encoding of Seq; ThreadTS is the TS of the thread's first message.
type Message struct {
	TS         string
	Seq        int64
	ThreadTS   string
	ReplyCount int
	UserName   string
	UserEmail  string
	Text       string
	PostedAt   time.Time
	EditedAt   time.Time
	Subtype    string
	IsBot      bool
}

// HistoryPage is one page of a channel's history, oldest first.
type HistoryPage struct {
	Messages   []Message
	NextCursor *int64
}

// Posted is the chat's answer to a post. ID is the message's TS, the handle
// an edit names it by.
type Posted struct {
	ID  string
	Seq int64
}

// MemberResult says what happened to each email a channel was opened to.
type MemberResult struct {
	Added, AlreadyMember []string
	// NotInChat have no active chat account: unknown, a guest, or deactivated.
	NotInChat []string
	// NotAdded have an account the chat would not add: outside the app's
	// audience, a scope the bot lacks, or a failed add.
	NotAdded []string
}

// Me reads the bot the token belongs to and its team.
func (c *Client) Me(ctx context.Context) (Bot, error) {
	var bot Bot
	return bot, c.call(ctx, http.MethodGet, "/api/bot/me", nil, &bot)
}

// Channel reads one channel by slug.
func (c *Client) Channel(ctx context.Context, slug string) (Channel, error) {
	var out struct {
		Channel Channel `json:"channel"`
	}
	err := c.call(ctx, http.MethodGet, "/api/bot/channels/"+url.PathEscape(slug), nil, &out)
	return out.Channel, err
}

// CreateChannel makes a private channel under an external key. Dispact answers
// a repeated key with the channel made the first time, so a retried create
// never makes two channels.
func (c *Client) CreateChannel(ctx context.Context, externalKey, name string) (Channel, error) {
	body := map[string]any{"name": name, "is_private": true, "external_key": externalKey}
	var out struct {
		Slug        string `json:"channel"`
		IsArchived  bool   `json:"is_archived"`
		BotIsMember bool   `json:"bot_is_member"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/bot/channels", body, &out); err != nil {
		return Channel{}, err
	}
	return c.Channel(ctx, out.Slug)
}

// Channels lists the channels the bot can see: every public channel, and each
// private channel it is a member of. Dispact names a channel here by its slug
// and leaves archived channels out.
func (c *Client) Channels(ctx context.Context) ([]Channel, error) {
	var out struct {
		Channels []struct {
			Name       string `json:"name"`
			IsPrivate  bool   `json:"is_private"`
			IsArchived bool   `json:"is_archived"`
		} `json:"channels"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/bot/channels", nil, &out); err != nil {
		return nil, err
	}
	channels := make([]Channel, 0, len(out.Channels))
	for _, ch := range out.Channels {
		channels = append(channels, Channel{
			Slug: ch.Name, Name: ch.Name, IsPrivate: ch.IsPrivate, IsArchived: ch.IsArchived,
			// A private channel is listed only to its members.
			BotIsMember: ch.IsPrivate,
		})
	}
	return channels, nil
}

// ChannelByKey finds the channel created under externalKey.
func (c *Client) ChannelByKey(ctx context.Context, externalKey string) (Channel, error) {
	var out struct {
		Channel Channel `json:"channel"`
	}
	err := c.call(ctx, http.MethodGet, "/api/bot/channel-keys/"+url.PathEscape(externalKey), nil, &out)
	return out.Channel, err
}

// AddMembers opens a channel to the members with these emails, at most 100.
func (c *Client) AddMembers(ctx context.Context, slug string, emails []string) (MemberResult, error) {
	var out struct {
		Added         []string `json:"added"`
		AlreadyMember []string `json:"already_member"`
		Failed        []struct {
			Email  string `json:"email"`
			Reason string `json:"reason"`
		} `json:"failed"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/bot/channels/"+url.PathEscape(slug)+"/members",
		map[string]any{"emails": emails}, &out); err != nil {
		return MemberResult{}, err
	}
	result := MemberResult{Added: out.Added, AlreadyMember: out.AlreadyMember}
	for _, f := range out.Failed {
		if f.Reason == "user_not_found" || f.Reason == "user_inactive" {
			result.NotInChat = append(result.NotInChat, f.Email)
		} else {
			result.NotAdded = append(result.NotAdded, f.Email)
		}
	}
	return result, nil
}

// Post sends one message. clientMsgID makes the post idempotent, so a retried
// flush cannot post twice; threadTS replies in a thread when not empty.
func (c *Client) Post(ctx context.Context, slug, text, clientMsgID, threadTS string) (Posted, error) {
	body := map[string]any{"channel": slug, "text": text, "client_msg_id": clientMsgID}
	if threadTS != "" {
		body["thread_ts"] = threadTS
	}
	var out struct {
		TS string `json:"ts"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/bot/messages", body, &out); err != nil {
		return Posted{}, err
	}
	seq, err := seqOf(out.TS)
	if err != nil {
		return Posted{}, fmt.Errorf("dispact: posting: %w: %w", ErrUnreachable, err)
	}
	return Posted{ID: out.TS, Seq: seq}, nil
}

// Update rewrites a message the bot posted in the channel slug; ts is the
// message's Posted.ID.
func (c *Client) Update(ctx context.Context, slug, ts, text string) error {
	return c.call(ctx, http.MethodPost, "/api/bot/messages/update",
		map[string]any{"channel": slug, "ts": ts, "text": text}, nil)
}

// History reads the messages after the cursor seq, oldest first. Dispact
// holds back messages younger than 30 seconds, so a page never ends mid-burst.
func (c *Client) History(ctx context.Context, slug string, after int64, limit int) (HistoryPage, error) {
	if limit <= 0 || limit > HistoryPageLimit {
		limit = HistoryPageLimit
	}
	path := fmt.Sprintf("/api/bot/channels/%s/messages?after=%d&limit=%d", url.PathEscape(slug), after, limit)
	var wire historyWire
	if err := c.call(ctx, http.MethodGet, path, nil, &wire); err != nil {
		return HistoryPage{}, err
	}
	page, err := wire.page()
	if err != nil {
		return HistoryPage{}, fmt.Errorf("dispact: reading history: %w: %w", ErrUnreachable, err)
	}
	return page, nil
}

// call sends one request and decodes the answer into out when out is not nil.
// Every failure is one of this package's typed errors.
//
//craft:ignore naked-any body and out are the JSON shapes each caller owns
func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("dispact: %s %s: %w: %w", method, routeOf(path), ErrUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }() //craft:ignore swallowed-errors a close error after a full read changes nothing the caller can act on
	raw, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit))
	if err != nil {
		return fmt.Errorf("dispact: reading %s: %w: %w", routeOf(path), ErrUnreachable, err)
	}
	if err := failureOf(resp, routeOf(path)); err != nil {
		return err
	}
	if err := refusalOf(raw, routeOf(path)); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("dispact: decoding %s: %w: %w", routeOf(path), ErrUnreachable, err)
	}
	return nil
}

//craft:ignore naked-any body is the JSON shape the caller owns
func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("dispact: encoding %s: %w", routeOf(path), err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.address+path, reader)
	if err != nil {
		return nil, fmt.Errorf("dispact: building %s: %w", routeOf(path), err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", outbound.ChatHeader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// failureOf maps a non-2xx answer to a typed error. The body stays out of the
// error, so a channel's content cannot reach a log through it.
func failureOf(resp *http.Response, route string) error {
	switch status := resp.StatusCode; {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized:
		return fmt.Errorf("dispact: %s: %w", route, ErrTokenRejected)
	case status == http.StatusForbidden:
		return &StatusError{Route: route, Status: status, wrapped: ErrNotMember}
	case status == http.StatusNotFound || status == http.StatusGone:
		return &StatusError{Route: route, Status: status, wrapped: ErrChannelGone}
	case status == http.StatusTooManyRequests:
		return &connector.RateLimitedError{RetryAfter: retryafter.Of(resp), Reason: route, Status: status}
	case status >= 500:
		return &StatusError{Route: route, Status: status, wrapped: ErrUnreachable}
	default:
		return &StatusError{Route: route, Status: status, wrapped: ErrRequestRejected}
	}
}

// refusalOf reads the envelope Dispact puts on every bot API answer. Dispact
// refuses with HTTP 200 and {"ok": false, "error": code}, as Slack does, so
// the status alone never shows a refusal. An answer without "ok": true is a
// fault, so a drifted shape fails instead of decoding as empty.
func refusalOf(raw []byte, route string) error {
	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("dispact: decoding %s: %w: %w", route, ErrUnreachable, err)
	}
	if envelope.OK {
		return nil
	}
	return &CodeError{Route: route, Code: envelope.Error, wrapped: refusalKind(envelope.Error)}
}

// refusalKind maps a Dispact error code to this package's typed errors.
func refusalKind(code string) error {
	switch code {
	case "invalid_auth", "not_authed", "token_revoked", "missing_scope":
		return ErrTokenRejected
	case "not_in_channel":
		return ErrNotMember
	case "channel_not_found", "message_not_found", "is_archived":
		return ErrChannelGone
	case "internal_error", "channel_create_failed", "":
		return ErrUnreachable
	default:
		return ErrRequestRejected
	}
}

// routeOf drops the query from a path, so an error names the route and never
// an email or a cursor.
func routeOf(path string) string {
	route, _, _ := strings.Cut(path, "?")
	return route
}

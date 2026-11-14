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
	ID     string `json:"id"`
	Name   string `json:"name"`
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

// Message is one channel message from the history read.
type Message struct {
	ID         string    `json:"id"`
	Seq        int64     `json:"seq"`
	ThreadTS   string    `json:"thread_ts"`
	ReplyCount int       `json:"reply_count"`
	UserName   string    `json:"user_name"`
	UserEmail  string    `json:"user_email"`
	Text       string    `json:"text"`
	PostedAt   time.Time `json:"posted_at"`
	EditedAt   time.Time `json:"edited_at"`
	Subtype    string    `json:"subtype"`
	IsBot      bool      `json:"is_bot"`
}

// HistoryPage is one page of a channel's history, oldest first.
type HistoryPage struct {
	Messages   []Message `json:"messages"`
	NextCursor *int64    `json:"next_cursor"`
}

// Posted is the chat's answer to a post.
type Posted struct {
	ID  string `json:"id"`
	Seq int64  `json:"seq"`
}

// MemberResult says which of the emails a channel was opened to.
type MemberResult struct {
	Added    []string `json:"added"`
	NotFound []string `json:"not_found"`
}

// Me reads the bot the token belongs to and its team.
func (c *Client) Me(ctx context.Context) (Bot, error) {
	var bot Bot
	return bot, c.call(ctx, http.MethodGet, "/api/bot/me", nil, &bot)
}

// Channel reads one channel by slug.
func (c *Client) Channel(ctx context.Context, slug string) (Channel, error) {
	var ch Channel
	return ch, c.call(ctx, http.MethodGet, "/api/bot/channels/"+url.PathEscape(slug), nil, &ch)
}

// CreateChannel makes a private channel under an external key. A second create
// with the same key answers 409. The first channel is then read back by its
// key, so a retried create never makes two channels.
func (c *Client) CreateChannel(ctx context.Context, externalKey, name string) (Channel, error) {
	body := map[string]any{"name": name, "is_private": true, "external_key": externalKey}
	var ch Channel
	err := c.call(ctx, http.MethodPost, "/api/bot/channels", body, &ch)
	if err == nil {
		return ch, nil
	}
	if status, ok := statusOf(err); !ok || status != http.StatusConflict {
		return Channel{}, err
	}
	return c.ChannelByKey(ctx, externalKey)
}

// ChannelByKey finds the channel created under externalKey.
func (c *Client) ChannelByKey(ctx context.Context, externalKey string) (Channel, error) {
	var ch Channel
	return ch, c.call(ctx, http.MethodGet, "/api/bot/channels?external_key="+url.QueryEscape(externalKey), nil, &ch)
}

// AddMembers opens a channel to the members with these emails, at most 100.
func (c *Client) AddMembers(ctx context.Context, slug string, emails []string) (MemberResult, error) {
	var out MemberResult
	err := c.call(ctx, http.MethodPost, "/api/bot/channels/"+url.PathEscape(slug)+"/members",
		map[string]any{"emails": emails}, &out)
	return out, err
}

// Post sends one message. clientMsgID makes the post idempotent, so a retried
// flush cannot post twice; threadTS replies in a thread when not empty.
func (c *Client) Post(ctx context.Context, slug, text, clientMsgID, threadTS string) (Posted, error) {
	body := map[string]any{"text": text, "client_msg_id": clientMsgID}
	if threadTS != "" {
		body["thread_ts"] = threadTS
	}
	var out Posted
	return out, c.call(ctx, http.MethodPost, "/api/bot/channels/"+url.PathEscape(slug)+"/messages", body, &out)
}

// Update rewrites a message the bot posted.
func (c *Client) Update(ctx context.Context, messageID, text string) error {
	return c.call(ctx, http.MethodPatch, "/api/bot/messages/"+url.PathEscape(messageID),
		map[string]any{"text": text}, nil)
}

// History reads the messages after the cursor seq, oldest first. Dispact
// holds back messages younger than 30 seconds, so a page never ends mid-burst.
func (c *Client) History(ctx context.Context, slug string, after int64, limit int) (HistoryPage, error) {
	if limit <= 0 || limit > HistoryPageLimit {
		limit = HistoryPageLimit
	}
	path := fmt.Sprintf("/api/bot/channels/%s/messages?after=%d&limit=%d", url.PathEscape(slug), after, limit)
	var page HistoryPage
	return page, c.call(ctx, http.MethodGet, path, nil, &page)
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
	if out == nil || len(raw) == 0 {
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

// routeOf drops the query from a path, so an error names the route and never
// an email or a cursor.
func routeOf(path string) string {
	route, _, _ := strings.Cut(path, "?")
	return route
}

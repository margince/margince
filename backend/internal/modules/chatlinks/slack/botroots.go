// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

type botMessage struct {
	ID      string `json:"ts"`
	Root    string `json:"thread_ts"`
	Text    string `json:"text"`
	User    string `json:"user"`
	Bot     string `json:"bot_id"`
	Subtype string `json:"subtype"`
	Replies int    `json:"reply_count"`
	Latest  string `json:"latest_reply"`
	Edited  *struct {
		TS string `json:"ts"`
	} `json:"edited"`
	Files []struct {
		Name string `json:"name"`
		Type string `json:"mimetype"`
	} `json:"files"`
}

type botMessagePage struct {
	Messages []botMessage `json:"messages"`
	More     bool         `json:"has_more"`
	Metadata struct {
		Next string `json:"next_cursor"`
	} `json:"response_metadata"`
}

func (b *Bot) readMessages(ctx context.Context, method string, fields url.Values) (botMessagePage, error) {
	if err := b.waitHistory(ctx, method); err != nil {
		return botMessagePage{}, err
	}
	if method == "conversations.history" {
		fields.Set("limit", strconv.Itoa(b.history.profile.HistoryPageSize))
	} else {
		fields.Set("limit", strconv.Itoa(b.history.profile.RepliesPageSize))
	}
	reader := b.client
	if method == repliesMethod {
		if b.threadFailure != nil {
			return botMessagePage{}, b.threadFailure
		}
		if b.threadReader == nil {
			return botMessagePage{}, chatwire.ErrHistoryProfileNeedsReview
		}
		reader = b.threadReader
	}
	raw, err := reader.call(ctx, http.MethodGet, method+"?"+fields.Encode(), nil)
	if err != nil {
		if method == "conversations.replies" {
			return botMessagePage{}, b.pauseHistory(ctx, method, threadReadFailure(err))
		}
		return botMessagePage{}, b.pauseHistory(ctx, method, botFailure(err))
	}

	var page botMessagePage
	if json.Unmarshal(raw, &page) != nil || page.More && page.Metadata.Next == "" {
		return botMessagePage{}, chatwire.ErrUnreachable
	}
	return page, nil
}

// Roots discovers canonical roots using provider continuation state.
func (b *Bot) Roots(ctx context.Context, id string, since time.Time, cursor string) (chatwire.RootPage, error) {
	if err := b.readableChannel(ctx, id); err != nil {
		return chatwire.RootPage{}, err
	}
	fields := url.Values{channelKey: {id}, "cursor": {cursor}, limitKey: {"200"}}
	if !since.IsZero() {
		fields.Set("oldest", fmt.Sprintf("%d.%06d", since.Unix(), since.Nanosecond()/1000))
	}
	page, err := b.readMessages(ctx, "conversations.history", fields)
	if err != nil {
		return chatwire.RootPage{}, err
	}
	out := chatwire.RootPage{Roots: []chatwire.RootRef{}, Next: page.Metadata.Next}
	for _, row := range page.Messages {
		if row.Root != "" && row.Root != row.ID {
			continue
		}
		at, err := timestamp(row.ID)
		if err != nil {
			return chatwire.RootPage{}, chatwire.ErrUnreachable
		}
		raw, err := json.Marshal(row)
		if err != nil {
			return chatwire.RootPage{}, chatwire.ErrUnreachable
		}
		digest := sha256.Sum256(raw)
		out.Roots = append(out.Roots, chatwire.RootRef{ID: row.ID, Revision: hex.EncodeToString(digest[:]), UpdatedAt: at})
	}
	return out, nil
}

// Thread reads canonical thread state with explicit continuation and deletion evidence.
func (b *Bot) Thread(ctx context.Context, id, root, cursor string) (chatwire.ThreadPage, error) {
	if err := b.readableThreadChannel(ctx, id); err != nil {
		return chatwire.ThreadPage{}, err
	}
	if err := b.readableChannel(ctx, id); err != nil {
		return chatwire.ThreadPage{}, err
	}
	if cursor != "" {
		return chatwire.ThreadPage{}, chatwire.ErrRequestRejected
	}
	authors := map[string]messaging.Member{}
	return chatwire.StableThread(ctx, func(ctx context.Context) (chatwire.ThreadPage, error) {
		return b.threadSnapshot(ctx, id, root, authors)
	})
}

func (b *Bot) threadSnapshot(ctx context.Context, id, root string, authors map[string]messaging.Member) (chatwire.ThreadPage, error) {
	out := chatwire.ThreadPage{Messages: []chatwire.Message{}}
	fields := url.Values{channelKey: {id}, "ts": {root}, "limit": {"200"}}
	seen := map[string]bool{}
	runes := 0
	for range 100 {
		page, err := b.readMessages(ctx, "conversations.replies", fields)
		if err != nil {
			return chatwire.ThreadPage{}, err
		}
		if fields.Get("cursor") == "" && !startsWithRoot(page.Messages, root) {
			return chatwire.ThreadPage{}, chatwire.ErrUnreachable
		}
		for _, row := range page.Messages {
			if row.ID != root && row.Root != root {
				return chatwire.ThreadPage{}, chatwire.ErrUnreachable
			}
			message, err := b.message(ctx, row, authors)
			if err != nil {
				return chatwire.ThreadPage{}, err
			}
			runes += utf8.RuneCountInString(message.Text)
			out.Messages = append(out.Messages, message)
			if len(out.Messages) > 1000 || runes > 120000 {
				return chatwire.ThreadPage{}, chatwire.ErrThreadTooLong
			}
		}
		if page.Metadata.Next == "" {
			return out, nil
		}
		if seen[page.Metadata.Next] {
			return chatwire.ThreadPage{}, chatwire.ErrUnreachable
		}
		seen[page.Metadata.Next] = true
		fields.Set("cursor", page.Metadata.Next)
	}
	return chatwire.ThreadPage{}, chatwire.ErrThreadTooLong
}

func (b *Bot) message(ctx context.Context, row botMessage, authors map[string]messaging.Member) (chatwire.Message, error) {
	at, err := timestamp(row.ID)
	if err != nil {
		return chatwire.Message{}, chatwire.ErrUnreachable
	}
	out := chatwire.Message{TS: row.ID, ThreadTS: row.Root, Text: row.Text, PostedAt: at, Subtype: row.Subtype, IsBot: row.Bot != "", ReplyCount: row.Replies}
	if row.Edited != nil {
		out.EditedAt, err = timestamp(row.Edited.TS)
		if err != nil {
			return chatwire.Message{}, chatwire.ErrUnreachable
		}
	}
	if row.User != "" && !out.IsBot {
		author, err := b.sourceAuthor(ctx, row.User, authors)
		if err != nil {
			return chatwire.Message{}, botFailure(err)
		}
		out.UserName, out.UserEmail = author.Name, author.Email
	}
	for _, file := range row.Files {
		out.Files = append(out.Files, chatwire.File{Name: file.Name, MimeType: file.Type})
	}
	return out, nil
}

// History serves the legacy sequence interface where the provider supports it.
func (b *Bot) History(ctx context.Context, id string, after int64, limit int) (chatwire.HistoryPage, error) {
	if err := b.readableChannel(ctx, id); err != nil {
		return chatwire.HistoryPage{}, err
	}
	if limit < 1 || limit > 99 {
		limit = 99
	}
	fields := url.Values{"channel": {id}, "limit": {"100"}, "oldest": {fmt.Sprintf("%d.%06d", after/1000000, after%1000000)}}
	out := chatwire.HistoryPage{Messages: []chatwire.Message{}}
	for range 100 {
		page, err := b.readMessages(ctx, "conversations.history", fields)
		if err != nil {
			return chatwire.HistoryPage{}, err
		}
		for _, row := range page.Messages {
			message, err := b.message(ctx, row, nil)
			if err != nil {
				return chatwire.HistoryPage{}, err
			}
			message.Seq = message.PostedAt.UnixMicro()
			out.Messages = append(out.Messages, message)
		}
		if page.Metadata.Next == "" {
			slices.SortFunc(out.Messages, func(a, b chatwire.Message) int { return a.PostedAt.Compare(b.PostedAt) })
			if len(out.Messages) > limit {
				out.Messages = out.Messages[:limit]
				next := out.Messages[len(out.Messages)-1].Seq
				out.NextCursor = &next
			}
			return out, nil
		}
		if fields.Get("cursor") == page.Metadata.Next {
			return chatwire.HistoryPage{}, chatwire.ErrUnreachable
		}
		fields.Set("cursor", page.Metadata.Next)
	}
	return chatwire.HistoryPage{}, chatwire.ErrUnreachable
}

func (b *Bot) readableChannel(ctx context.Context, id string) error {
	room, err := b.Channel(ctx, id)
	if err != nil {
		return err
	}
	if room.IsArchived || !room.BotIsMember {
		return chatwire.ErrNotMember
	}
	return nil
}

func (b *Bot) readableThreadChannel(ctx context.Context, id string) error {
	if b.threadFailure != nil {
		return b.threadFailure
	}
	if b.threadReader == nil {
		return chatwire.ErrHistoryProfileNeedsReview
	}
	raw, err := b.threadReader.call(ctx, http.MethodGet, "conversations.info?"+url.Values{"channel": {id}}.Encode(), nil)
	if err != nil {
		return threadReadFailure(err)
	}
	var response struct {
		Channel channel `json:"channel"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Channel.ID != id {
		return chatwire.ErrUnreachable
	}
	room := response.Channel
	if !room.Member || room.Archived || room.IM || room.MPIM {
		return chatwire.ErrHistoryProfileNeedsReview
	}
	return nil
}

func threadReadFailure(err error) error {
	var refusal *apiRefusal
	if errors.Is(err, messaging.ErrUnauthorized) || errors.Is(err, messaging.ErrNotFound) || errors.As(err, &refusal) && (refusal.code == errorChannelNotFound || refusal.code == "not_in_channel") {
		return errors.Join(chatwire.ErrHistoryProfileNeedsReview, err)
	}
	return err
}

func startsWithRoot(messages []botMessage, root string) bool {
	return len(messages) > 0 && messages[0].ID == root && (messages[0].Root == "" || messages[0].Root == root)
}

func (b *Bot) sourceAuthor(ctx context.Context, id string, authors map[string]messaging.Member) (messaging.Member, error) {
	author, known := authors[id]
	if known {
		return author, nil
	}
	author, err := b.client.member(ctx, id)
	if err != nil {
		return messaging.Member{}, err
	}
	if authors != nil {
		authors[id] = author
	}
	return author, nil
}

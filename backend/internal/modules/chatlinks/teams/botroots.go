// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// Roots discovers canonical roots using provider continuation state.
func (b *Bot) Roots(ctx context.Context, id string, _ time.Time, cursor string) (chatwire.RootPage, error) {
	if _, err := b.Channel(ctx, id); err != nil {
		return chatwire.RootPage{}, err
	}
	base, ok := channelPath(id)
	if !ok {
		return chatwire.RootPage{}, messaging.ErrInvalid
	}
	path, err := graphPagePath(base+"/messages", cursor)
	if err != nil {
		return chatwire.RootPage{}, err
	}
	page, err := b.graphMessagePage(ctx, path)
	if err != nil {
		return chatwire.RootPage{}, err
	}
	out := chatwire.RootPage{Roots: []chatwire.RootRef{}, Next: page.Next}
	for _, row := range page.Messages {
		if row.ID == "" || row.Root != "" {
			return chatwire.RootPage{}, chatwire.ErrUnreachable
		}
		revision, err := messageRevision(row)
		if err != nil {
			return chatwire.RootPage{}, err
		}
		out.Roots = append(out.Roots, chatwire.RootRef{ID: row.ID, Revision: revision, UpdatedAt: row.Modified, Deleted: row.Deleted != nil})
	}
	return out, nil
}

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type graphMessages struct {
	Messages []message `json:"value"`
	Next     string    `json:"@odata.nextLink"`
}

func (b *Bot) graphMessagePage(ctx context.Context, path string) (graphMessages, error) {
	raw, err := b.graph.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return graphMessages{}, teamsBotFailure(err)
	}
	var page graphMessages
	if json.Unmarshal(raw, &page) != nil {
		return graphMessages{}, chatwire.ErrUnreachable
	}
	return page, nil
}

func graphPagePath(base, cursor string) (string, error) {
	if cursor == "" {
		return base + "?$top=50", nil
	}
	path, err := nextPath(cursor)
	if err != nil || !strings.HasPrefix(path, base+"?") {
		return "", messaging.ErrInvalid
	}
	return path, nil
}

func messageRevision(value message) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", chatwire.ErrUnreachable
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// Thread reads canonical thread state with explicit continuation and deletion evidence.
func (b *Bot) Thread(ctx context.Context, id, root, cursor string) (chatwire.ThreadPage, error) {
	if cursor != "" || root == "" {
		return chatwire.ThreadPage{}, messaging.ErrInvalid
	}
	if _, err := b.Channel(ctx, id); err != nil {
		return chatwire.ThreadPage{}, err
	}
	audience, err := b.graph.Conversation(ctx, id)
	if err != nil {
		return chatwire.ThreadPage{}, teamsBotFailure(err)
	}
	authors := map[string]messaging.Member{}
	for _, member := range audience.Members {
		authors[member.ID] = member
	}
	return chatwire.StableThread(ctx, func(ctx context.Context) (chatwire.ThreadPage, error) {
		return b.threadSnapshot(ctx, id, root, authors)
	})
}

func (b *Bot) threadSnapshot(ctx context.Context, id, root string, authors map[string]messaging.Member) (chatwire.ThreadPage, error) {
	first, _, err := b.completeThread(ctx, id, root)
	if err != nil {
		return chatwire.ThreadPage{}, err
	}
	out := chatwire.ThreadPage{Messages: []chatwire.Message{}}
	for _, row := range first {
		item, err := row.normalized()
		if err != nil {
			return chatwire.ThreadPage{}, err
		}
		author := authors[row.From.User.ID]
		name := row.From.User.Name
		if author.Name != "" {
			name = author.Name
		}
		out.Deleted = out.Deleted || row.ID == root && item.Deleted
		out.Messages = append(out.Messages, chatwire.Message{NativeID: item.ID, ChannelID: id, RootID: root, TS: item.ID, ThreadTS: root, Text: item.Text, PostedAt: item.At, EditedAt: row.Modified, Deleted: item.Deleted, IsBot: row.From.Application.ID != "", UserName: name, UserEmail: author.Email})
	}
	return out, nil
}

func (b *Bot) completeThread(ctx context.Context, id, root string) ([]message, string, error) {
	base, ok := channelPath(id)
	if !ok {
		return nil, "", messaging.ErrInvalid
	}
	base += "/messages/" + url.PathEscape(root)
	raw, err := b.graph.http.Call(ctx, http.MethodGet, base, nil)
	if err != nil {
		return nil, "", teamsBotFailure(err)
	}
	var parent message
	if json.Unmarshal(raw, &parent) != nil || parent.ID != root || parent.Root != "" {
		return nil, "", chatwire.ErrUnreachable
	}
	if parent.Deleted != nil {
		revision, err := messageRevision(parent)
		return []message{parent}, revision, err
	}
	return b.completeReplies(ctx, base, root, parent)
}

func (b *Bot) completeReplies(ctx context.Context, base, root string, parent message) ([]message, string, error) {
	rows := []message{parent}
	seen := map[string]bool{root: true}
	path := base + "/replies?$top=50"
	runes := utf8.RuneCountInString(parent.Body.Text)
	for range 100 {
		page, err := b.graphMessagePage(ctx, path)
		if err != nil {
			return nil, "", err
		}
		for _, row := range page.Messages {
			if row.ID == "" || row.Root != root || seen[row.ID] {
				return nil, "", chatwire.ErrUnreachable
			}
			seen[row.ID] = true
			runes += utf8.RuneCountInString(row.Body.Text)
			rows = append(rows, row)
			if len(rows) > 1000 || runes > 120000 {
				return nil, "", chatwire.ErrThreadTooLong
			}
		}
		if page.Next == "" {
			slices.SortFunc(rows, func(a, b message) int { return strings.Compare(a.ID, b.ID) })
			encoded, err := json.Marshal(rows)
			if err != nil {
				return nil, "", chatwire.ErrUnreachable
			}
			digest := sha256.Sum256(encoded)
			return rows, hex.EncodeToString(digest[:]), nil
		}
		next, err := graphPagePath(base+"/replies", page.Next)
		if err != nil || next == path {
			return nil, "", chatwire.ErrUnreachable
		}
		path = next
	}
	return nil, "", chatwire.ErrThreadTooLong
}

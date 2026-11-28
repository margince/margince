// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
)

// Roots discovers canonical roots using provider continuation state.
func (c *CanonicalClient) Roots(ctx context.Context, channel string, _ time.Time, cursor string) (chatwire.RootPage, error) {
	room, err := c.Channel(ctx, channel)
	if err != nil {
		return chatwire.RootPage{}, err
	}
	after := int64(0)
	if cursor != "" {
		after, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || after < 0 {
			return chatwire.RootPage{}, chatwire.ErrRequestRejected
		}
	}
	page, err := c.History(ctx, room.NativeID, after, HistoryPageLimit)
	if err != nil {
		return chatwire.RootPage{}, err
	}
	out := chatwire.RootPage{Roots: []chatwire.RootRef{}}
	roots := map[string]chatwire.RootRef{}
	for _, message := range page.Messages {
		if message.ChannelID != room.NativeID || message.NativeID == "" || message.RootID == "" {
			return chatwire.RootPage{}, chatwire.ErrUnreachable
		}
		digest := sha256.Sum256([]byte(message.NativeID + "\x00" + message.EditedAt.Format(time.RFC3339Nano) + "\x00" + message.Text))
		root := chatwire.RootRef{ID: message.RootID, Revision: hex.EncodeToString(digest[:]), UpdatedAt: message.PostedAt, Deleted: message.Deleted && message.NativeID == message.RootID}
		if _, exists := roots[root.ID]; !exists {
			out.Roots = append(out.Roots, root)
		}
		roots[root.ID] = root
	}
	for i := range out.Roots {
		out.Roots[i] = roots[out.Roots[i].ID]
	}
	out.Checkpoint, out.Next, err = canonicalRootCheckpoint(page, after)
	return out, err
}

// Thread reads canonical thread state with explicit continuation and deletion evidence.
func (c *CanonicalClient) Thread(ctx context.Context, channel, root, cursor string) (chatwire.ThreadPage, error) {
	room, err := c.Channel(ctx, channel)
	if err != nil {
		return chatwire.ThreadPage{}, err
	}
	var page struct {
		historyWire
		Channel  string `json:"channel_id"`
		Root     string `json:"root_message_id"`
		Revision string `json:"revision"`
		Complete bool   `json:"complete"`
		More     bool   `json:"has_more"`
		Deleted  bool   `json:"deleted"`
	}
	path := "/api/bot/channels/" + url.PathEscape(immutableChannelRef(room.NativeID)) + "/threads/" + url.PathEscape(root) + "?" + url.Values{limitKey: {canonicalPageLimit}, cursorKey: {cursor}}.Encode()
	if err := c.call(ctx, http.MethodGet, path, nil, &page); err != nil {
		return chatwire.ThreadPage{}, err
	}
	if page.Channel != room.NativeID || page.Root != root || page.Revision == "" {
		return chatwire.ThreadPage{}, chatwire.ErrUnreachable
	}
	next := page.ResponseMetadata.NextCursor
	if page.Complete != (next == "") || page.More != (next != "") || next != "" && next == cursor {
		return chatwire.ThreadPage{}, chatwire.ErrUnreachable
	}
	// Canonical cursors are opaque; the legacy decoder must not interpret them as sequences.
	page.ResponseMetadata.NextCursor = ""
	decoded, err := page.page()
	if err != nil {
		return chatwire.ThreadPage{}, err
	}
	out := chatwire.ThreadPage{Messages: decoded.Messages, Next: next, Deleted: page.Deleted, Revision: page.Revision}
	out.Messages, err = canonicalThreadMessages(out.Messages, room.NativeID, root)
	return out, err
}

func canonicalRootCheckpoint(page HistoryPage, after int64) (string, string, error) {
	var checkpoint, next string
	if page.HasMore == nil || *page.HasMore && (page.NextCursor == nil || *page.NextCursor <= after || len(page.Messages) == 0) {
		return "", "", chatwire.ErrUnreachable
	}
	checkpoint = strconv.FormatInt(after, 10)
	if page.NextCursor != nil {
		if *page.NextCursor < after {
			return "", "", chatwire.ErrUnreachable
		}
		checkpoint = strconv.FormatInt(*page.NextCursor, 10)
	}
	if *page.HasMore {
		next = checkpoint
	}
	return checkpoint, next, nil
}

func canonicalThreadMessages(messages []chatwire.Message, channel, root string) ([]chatwire.Message, error) {
	for i := range messages {
		message := &messages[i]
		if message.ChannelID != channel || message.RootID != root || message.NativeID == "" {
			return nil, chatwire.ErrUnreachable
		}
		message.WriteRef = message.TS
		message.TS, message.ThreadTS = message.NativeID, message.RootID
		if message.Deleted {
			message.Text = ""
		}
	}
	return messages, nil
}

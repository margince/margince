// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// Source resolves a native source only under current participant access.
func (c *Participant) Source(ctx context.Context, conversation, root string) (string, error) {
	room, err := c.channel(ctx, conversation)
	if err != nil {
		return "", err
	}
	page, err := c.Messages(ctx, room.ID, root, "")
	if err != nil {
		return "", err
	}
	if root == "" || !messaging.ContainsLiveRoot(page, root) {
		return "", messaging.ErrUnavailable
	}
	return strings.TrimSuffix(c.http.Base, "/api") + "/channels/" + url.PathEscape(immutableChannelRef(room.ID)) + "/messages/" + url.PathEscape(root), nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// Source resolves a native source only under current participant access.
func (c *Client) Source(ctx context.Context, conversation, root string) (string, error) {
	page, err := c.Messages(ctx, conversation, root, "")
	if err != nil {
		return "", err
	}
	if root == "" || !messaging.ContainsLiveRoot(page, root) {
		return "", messaging.ErrUnavailable
	}
	_, channel, ok := strings.Cut(conversation, "/")
	if !ok {
		return "", messaging.ErrUnsupported
	}
	for _, message := range page.Messages {
		if message.ID != root {
			continue
		}
		return teamsSourceURL(message.SourceURL, channel, root)
	}
	return "", messaging.ErrUnavailable
}

func teamsSourceURL(address, channel, root string) (string, error) {
	link, err := url.Parse(address)
	if err != nil || link.Scheme != httpsScheme || link.Host != "teams.microsoft.com" || link.User != nil {
		return "", messaging.ErrUnavailable
	}
	path := strings.Split(strings.Trim(link.Path, "/"), "/")
	if len(path) != 4 || path[0] != "l" || path[1] != messageKind || path[2] != channel || path[3] != root {
		return "", messaging.ErrUnavailable
	}
	return link.String(), nil
}

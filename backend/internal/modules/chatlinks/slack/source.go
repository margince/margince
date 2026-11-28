// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"net/http"
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
	raw, err := c.call(ctx, http.MethodGet, "chat.getPermalink?"+url.Values{channelKey: {conversation}, "message_ts": {root}}.Encode(), nil)
	if err != nil {
		return "", err
	}
	var response struct {
		URL string `json:"permalink"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return "", messaging.ErrUnavailable
	}
	link, err := url.Parse(response.URL)
	if err != nil || link.Scheme != "https" || link.User != nil || !strings.HasSuffix(link.Host, ".slack.com") || link.Path != "/archives/"+conversation+"/p"+strings.ReplaceAll(root, ".", "") {
		return "", messaging.ErrUnavailable
	}
	return link.String(), nil
}

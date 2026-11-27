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

// ResolvePerson resolves an address to the provider identity used for recipient verification.
func (c *Client) ResolvePerson(ctx context.Context, email string) (messaging.Member, error) {
	raw, err := c.call(ctx, http.MethodGet, "users.lookupByEmail?"+url.Values{"email": {email}}.Encode(), nil)
	if err != nil {
		return messaging.Member{}, err
	}
	var response struct {
		User user `json:"user"`
	}
	if json.Unmarshal(raw, &response) != nil || response.User.ID == "" || !strings.EqualFold(response.User.Profile.Email, email) {
		return messaging.Member{}, messaging.ErrUnavailable
	}
	return c.person(response.User), nil
}

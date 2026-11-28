// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

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
	escaped := strings.ReplaceAll(email, "'", "''")
	path := "/users?" + url.Values{"$select": {"id,displayName,mail,userPrincipalName,userType,accountEnabled"}, "$filter": {"mail eq '" + escaped + "' or userPrincipalName eq '" + escaped + "'"}, "$top": {"2"}}.Encode()
	raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return messaging.Member{}, err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var response struct {
		People []person `json:"value"`
		Next   string   `json:"@odata.nextLink"`
	}
	if json.Unmarshal(raw, &response) != nil || len(response.People) != 1 || response.Next != "" {
		return messaging.Member{}, messaging.ErrUnavailable
	}
	found := response.People[0]
	if found.ID == "" || (!strings.EqualFold(found.Email, email) && !strings.EqualFold(found.Principal, email)) {
		return messaging.Member{}, messaging.ErrUnavailable
	}
	return found.member(), nil
}

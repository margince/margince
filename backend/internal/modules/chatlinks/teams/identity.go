// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// MessengerIdentity reads the granting user from Graph; the directory is pinned
// by the tenant-specific authorization endpoint, never by browser input.
func MessengerIdentity(ctx context.Context, httpClient messaging.Doer, token, tenant string) (messaging.Identity, error) {
	client := New(httpClient, token, tenant, "")
	raw, err := client.http.Call(ctx, http.MethodGet, "/me?$select=id,displayName,mail,userPrincipalName,userType,accountEnabled", nil)
	if err != nil {
		return messaging.Identity{}, err
	}
	var me person
	if json.Unmarshal(raw, &me) != nil || me.ID == "" {
		return messaging.Identity{}, messaging.ErrUnauthorized
	}
	client.user = me.ID
	return client.Identity(ctx)
}

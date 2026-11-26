// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
)

// authorized runs op under the mailbox's access token. A kept token can be
// revoked before its stated lifetime ends. So a 401 redeems once more, and op
// runs again on the new token rather than parking a healthy connection.
func authorized[T any](
	ctx context.Context, c *Connector, st graphconn.AuthState, op func(oauthflow.TokenRefresh) (T, error),
) (T, error) {
	refreshed, err := c.oauth.Refresh(ctx, st.RefreshToken, st.Granted)
	if err != nil {
		var none T
		return none, err
	}
	out, err := op(refreshed)
	if !oauthflow.Unauthorized(err) {
		return out, err
	}
	if refreshed, err = c.oauth.Refresh(oauthflow.Renewing(ctx), st.RefreshToken, st.Granted); err != nil {
		var none T
		return none, err
	}
	return op(refreshed)
}

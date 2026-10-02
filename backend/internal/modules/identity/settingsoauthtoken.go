// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How long the access token an MCP connector's OAuth handshake mints lives.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// minOAuthAccessTokenTTLMinutes is the shortest lifetime an admin may choose.
// Connector norms are minutes plus refresh, and the refresh machinery makes a
// short token cheap; below five minutes a connector spends its life rotating.
const minOAuthAccessTokenTTLMinutes = 5

// OAuthAccessTokenTTLMinutes is the lifetime of the passport the OAuth
// handshake mints, for the code exchange and every refresh rotation alike: a
// short-lived token that an hour-old rotation re-issues for 30 days is not
// short-lived.
//
// Thirty days by default, the passport's own default, so an installation that
// never chose sees what every connector saw before this was a setting. The
// ceiling is the passport's. A change applies to the next token minted; the
// tokens already issued keep the expiry they were issued with.
//
// MachineryApplied because the mint has no session to gate against: the token
// endpoint is reached by the connector, not by a signed-in member, and the
// lifetime must bind whoever completed the consent.
var OAuthAccessTokenTTLMinutes = settings.Define[int](
	"installation.oauth_access_token_ttl_minutes",
	installationSettingsObject,
	"update",
	int(defaultPassportTTL/time.Minute),
	func(minutes int) error {
		if highest := int(maxPassportTTL / time.Minute); minutes < minOAuthAccessTokenTTLMinutes || minutes > highest {
			return fmt.Errorf("an access token lives %d..%d minutes, not %d", minOAuthAccessTokenTTLMinutes, highest, minutes)
		}
		return nil
	},
).MachineryApplied() // both OAuth mints apply it to the passport they issue

// oauthAccessTokenTTL reads the lifetime inside the mint's own transaction, so
// the token and the value it was minted under come from one snapshot.
func oauthAccessTokenTTL(ctx context.Context, tx pgx.Tx) (time.Duration, error) {
	minutes, err := settings.ApplyTx(ctx, tx, OAuthAccessTokenTTLMinutes)
	if err != nil {
		return 0, fmt.Errorf("identity: reading the OAuth access-token lifetime: %w", err)
	}
	return time.Duration(minutes) * time.Minute, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How long the access token an MCP connector's OAuth handshake mints lives.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
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
var OAuthAccessTokenTTLMinutes = config.DefineStored(config.Spec[int]{
	Key:     "installation.oauth_access_token_ttl_minutes",
	Env:     "MARGINCE_INSTALLATION_OAUTH_ACCESS_TOKEN_TTL_MINUTES",
	Type:    config.Int,
	Default: config.Some(int(defaultPassportTTL / time.Minute)),
	Min:     config.Some(minOAuthAccessTokenTTLMinutes),
	Max:     config.Some(int(maxPassportTTL / time.Minute)),
	Doc:     "Minutes an access token from the OAuth handshake of a connector lives, for the first token and every refresh.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// errOAuthMintUnwired refuses a mint on a service composed without the
// configuration, which would otherwise ignore a lifetime a boot source sets.
var errOAuthMintUnwired = errors.New("identity: this service mints no OAuth token, because its configuration is not wired")

// WithConfig wires the configuration the OAuth mint reads its token lifetime
// through.
func (s *Service) WithConfig(c *config.Config) *Service {
	s.config = c
	return s
}

// oauthAccessTokenTTL reads the lifetime inside the mint's own transaction, so
// the token and the value it was minted under come from one snapshot.
func (s *Service) oauthAccessTokenTTL(ctx context.Context, tx pgx.Tx) (time.Duration, error) {
	if s.config == nil {
		return 0, errOAuthMintUnwired
	}
	minutes, err := config.StoredApplyTx(ctx, tx, s.config, OAuthAccessTokenTTLMinutes)
	if err != nil {
		return 0, fmt.Errorf("identity: reading the OAuth access-token lifetime: %w", err)
	}
	return time.Duration(minutes) * time.Minute, nil
}

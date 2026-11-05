// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The grant the core keeps for a unit's OAuth app, and the access token a unit
// reads from it. A refresh needs the client secret, which only the core may
// unseal. The grant is one workspace secret under a key the unit's own port
// refuses (extsecrets.CoreHeld). Nothing here logs a token or puts one in an error.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// oauthTokenSkew is how long before its stated expiry an access token counts
// as expired. A token handed out then still lives when it reaches the vendor.
const oauthTokenSkew = time.Minute

// codeInvalidGrant is RFC 6749 §5.2's refusal of a refresh token that was
// revoked or has expired at the vendor.
const codeInvalidGrant = "invalid_grant"

// errExtOAuthAppMissing is a unit app whose client id and secret no admin has
// stored, so no flow can run and no token can be refreshed.
var errExtOAuthAppMissing = errors.New("no client id and secret are stored for this OAuth app — an admin adds them under Settings → Sign-in and apps")

// storedOAuthGrant is the sealed grant. ExpiresAt is zero when the vendor did
// not state the access token's lifetime; the token is then refreshed on use.
type storedOAuthGrant struct {
	RefreshToken string    `json:"refresh_token"`
	AccessToken  string    `json:"access_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
}

func (g storedOAuthGrant) fresh(now time.Time) bool {
	return g.AccessToken != "" && now.Add(oauthTokenSkew).Before(g.ExpiresAt)
}

// extOAuthGrants is one unit's grants in the calling workspace.
type extOAuthGrants struct {
	secrets extension.Secrets
	// credentials resolves the installation's stored client id and unsealed
	// secret for a unit app.
	credentials func(ctx context.Context, provider string) (capture.ConnectorApp, bool, error)
	now         func() time.Time
}

// newExtOAuthGrants opens one unit's grants on the extension runtime binding.
// The consent callback and the Runtime both use it.
func newExtOAuthGrants(unit string, deps extensionRuntimeBinding) extOAuthGrants {
	store := capture.NewConnectorAppStore(NewSettingsStore(deps.pool), deps.vault, slog.Default()).
		WithUnitApps(composedUnitApp)
	return extOAuthGrants{
		secrets: extsecrets.CoreHeld(unit, deps.pool, deps.vault),
		credentials: func(ctx context.Context, provider string) (capture.ConnectorApp, bool, error) {
			return appCredentialsFrom(store, capture.AppProvider(provider))(ctx)
		},
		now: time.Now,
	}
}

func grantKey(provider string) string { return "oauth/" + provider }

func (g extOAuthGrants) load(ctx context.Context, provider string) (storedOAuthGrant, error) {
	raw, err := g.secrets.Get(ctx, grantKey(provider))
	if errors.Is(err, extension.ErrSecretNotFound) {
		return storedOAuthGrant{}, extension.ErrOAuthNotConnected
	}
	if err != nil {
		return storedOAuthGrant{}, err
	}
	var grant storedOAuthGrant
	if err := json.Unmarshal(raw, &grant); err != nil || grant.RefreshToken == "" {
		return storedOAuthGrant{}, fmt.Errorf("compose: the stored %s grant is unreadable; an admin must connect it again", provider)
	}
	return grant, nil
}

func (g extOAuthGrants) save(ctx context.Context, provider string, grant storedOAuthGrant) error {
	//nolint:gosec // G117: these bytes go only to the sealed secret store, never to a log or a response.
	raw, err := json.Marshal(grant)
	if err != nil {
		return fmt.Errorf("compose: encoding the %s grant: %w", provider, err)
	}
	return g.secrets.Put(ctx, grantKey(provider), raw)
}

// forget removes the grant. One already gone is the state asked for.
func (g extOAuthGrants) forget(ctx context.Context, provider string) error {
	if err := g.secrets.Delete(ctx, grantKey(provider)); err != nil && !errors.Is(err, extension.ErrSecretNotFound) {
		return err
	}
	return nil
}

// client is the vendor's token client, under the installation's stored app.
func (g extOAuthGrants) client(ctx context.Context, app extension.OAuthApp) (*oauthflow.Client, error) {
	creds, ok, err := g.credentials(ctx, app.Provider)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errExtOAuthAppMissing
	}
	return oauthflow.New(oauthflow.Config{
		Provider:     app.Provider,
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecretRef,
		Scopes:       app.Scopes,
		AuthURL:      app.AuthURL,
		TokenURL:     app.TokenURL,
		AuthParams:   app.AuthParams,
		AuthRejected: connector.ErrAuthRejected,
		Unreachable:  connector.ErrUnreachable,
	}), nil
}

// exchange redeems an authorization code and keeps the grant it yields.
func (g extOAuthGrants) exchange(ctx context.Context, app extension.OAuthApp, code, redirectURI string) error {
	client, err := g.client(ctx, app)
	if err != nil {
		return err
	}
	tok, err := client.Exchange(ctx, code, redirectURI)
	if err != nil {
		return err
	}
	return g.save(ctx, app.Provider, storedOAuthGrant{
		RefreshToken: tok.RefreshToken, AccessToken: tok.AccessToken, ExpiresAt: g.expiry(tok.ExpiresIn),
	})
}

func (g extOAuthGrants) expiry(lifetime time.Duration) time.Time {
	if lifetime <= 0 {
		return time.Time{}
	}
	return g.now().Add(lifetime)
}

// accessToken answers the stored access token while it is fresh, and refreshes
// it otherwise. A refresh token the vendor refuses is forgotten, so the next
// call reads as not connected rather than asking the vendor again.
func (g extOAuthGrants) accessToken(ctx context.Context, app extension.OAuthApp) (string, error) {
	grant, err := g.load(ctx, app.Provider)
	if err != nil {
		return "", err
	}
	if grant.fresh(g.now()) {
		return grant.AccessToken, nil
	}
	client, err := g.client(ctx, app)
	if err != nil {
		return "", err
	}
	refreshed, err := client.Refresh(ctx, grant.RefreshToken, nil)
	if connector.ProviderReason(err) == codeInvalidGrant {
		if err := g.forget(ctx, app.Provider); err != nil {
			return "", err
		}
		return "", extension.ErrOAuthRevoked
	}
	if err != nil {
		return "", fmt.Errorf("compose: refreshing the %s grant: %w", app.Provider, err)
	}
	grant.AccessToken, grant.ExpiresAt = refreshed.AccessToken, g.expiry(refreshed.ExpiresIn)
	if refreshed.Rotated != "" {
		grant.RefreshToken = refreshed.Rotated
	}
	if err := g.save(ctx, app.Provider, grant); err != nil {
		return "", err
	}
	return grant.AccessToken, nil
}

// OAuthAccessToken answers a live access token for one of this unit's apps.
// The declaration is checked first, so another unit's app reads as none
// whatever this role has wired.
func (r *callRuntime) OAuthAccessToken(ctx context.Context, provider extension.OAuthProvider) (string, error) {
	app, ok := declaredOAuthApp(r.unit, string(provider))
	if !ok {
		return "", fmt.Errorf("%w: %q", extension.ErrNoSuchOAuthApp, provider)
	}
	ctx, err := r.scoped(ctx)
	if err != nil {
		return "", err
	}
	return newExtOAuthGrants(r.unit, r.deps).accessToken(ctx, app)
}

// OAuthDisconnect forgets this workspace's grant for one of this unit's apps.
func (r *callRuntime) OAuthDisconnect(ctx context.Context, provider extension.OAuthProvider) error {
	if _, ok := declaredOAuthApp(r.unit, string(provider)); !ok {
		return fmt.Errorf("%w: %q", extension.ErrNoSuchOAuthApp, provider)
	}
	ctx, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	return newExtOAuthGrants(r.unit, r.deps).forget(ctx, string(provider))
}

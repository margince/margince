// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oauthflow

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/margince/margince/backend/internal/platform/googlejwt"
)

// MintedToken is an access token from a grant that returns no refresh token.
// The holder mints again once the lifetime has run out.
type MintedToken struct {
	AccessToken string
	// ExpiresIn is zero when the provider did not say.
	ExpiresIn time.Duration
}

// ClientCredentials mints an application token for scope (RFC 6749 §4.4).
// The token speaks for the configured app, not for any one user.
func (c *Client) ClientCredentials(ctx context.Context, scope string) (MintedToken, error) {
	return c.mint(ctx, url.Values{
		paramGrantType:    {"client_credentials"},
		paramClientID:     {c.cfg.ClientID},
		paramClientSecret: {c.cfg.ClientSecret},
		"scope":           {scope},
	})
}

// JWTBearer redeems a signed assertion for an access token (RFC 7523).
func (c *Client) JWTBearer(ctx context.Context, assertion string) (MintedToken, error) {
	return c.mint(ctx, url.Values{
		paramGrantType: {googlejwt.JWTBearerGrant},
		"assertion":    {assertion},
	})
}

func (c *Client) mint(ctx context.Context, form url.Values) (MintedToken, error) {
	tok, err := c.token(ctx, form)
	if err != nil {
		return MintedToken{}, err
	}
	if tok.AccessToken == "" {
		return MintedToken{}, fmt.Errorf("%s: the token endpoint returned no access token: %w", c.cfg.Provider, c.cfg.AuthRejected)
	}
	return MintedToken{AccessToken: tok.AccessToken, ExpiresIn: tok.lifetime()}, nil
}

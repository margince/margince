// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oauthflow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/margince/margince/backend/internal/platform/googlejwt"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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

// JWTBearer redeems a signed assertion for an access token (RFC 7523). The
// exchange itself is googlejwt's; this maps its answers onto the connector
// classes the registry acts on.
func (c *Client) JWTBearer(ctx context.Context, assertion string) (MintedToken, error) {
	tok, err := googlejwt.Exchange(ctx, c.http, c.cfg.TokenURL, assertion)
	if err != nil {
		return MintedToken{}, c.exchangeFailure(err)
	}
	return MintedToken{AccessToken: tok.AccessToken, ExpiresIn: tok.ExpiresIn}, nil
}

// exchangeFailure classes a failed assertion exchange the way token does for
// the other grants. A throttle waits and another 4xx rejects the auth. Anything
// else is the provider being unreachable.
func (c *Client) exchangeFailure(err error) error {
	if refused, ok := errors.AsType[googlejwt.ErrExchangeRefused](err); ok {
		if refused.Status == http.StatusTooManyRequests {
			return &connector.RateLimitedError{RetryAfter: refused.RetryAfter}
		}
		class := c.cfg.Unreachable
		if refused.Permanent() {
			class = c.cfg.AuthRejected
		}
		return &connector.ProviderError{
			Op: tokenOp, Status: refused.Status, Reason: connector.MachineReason(refused.Code), Class: class,
		}
	}
	if errors.Is(err, googlejwt.ErrNoAccessToken) {
		return fmt.Errorf("%s: the token endpoint returned no access token: %w", c.cfg.Provider, c.cfg.AuthRejected)
	}
	return fmt.Errorf("%s: token endpoint: %w", c.cfg.Provider, c.cfg.Unreachable)
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

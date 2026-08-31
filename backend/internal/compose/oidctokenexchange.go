// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The authorization-code -> ID-token exchange for the sign-in login flow, for
// EVERY provider: the request is the OAuth2 one and only the endpoint differs,
// which the caller supplies.
//
// Deliberately separate from capture/oauthflow.Client.Exchange: that method
// requires a refresh token back (offline access) and never parses id_token —
// login requests neither offline access nor a refresh token, and needs exactly
// the field oauthflow never reads. See googlesignin.go and microsoftsignin.go
// for where this is wired in.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
)

const oidcTokenExchangeTimeout = 30 * time.Second

type oidcCodeExchanger struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	HTTPClient   *http.Client
}

type oidcTokenResponse struct {
	IDToken string `json:"id_token"`
	Error   string `json:"error"`
}

// Exchange redeems a PKCE-bound authorization code for the caller's ID token.
// No refresh token is requested and none is read back: the authorization
// request StartOidcSignIn builds asks for no access_type=offline, so Google
// issues no refresh token here either, and this flow never mints one.
func (ex oidcCodeExchanger) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (string, error) {
	client := ex.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: oidcTokenExchangeTimeout}
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"redirect_uri":  {redirectURI},
		"client_id":     {ex.ClientID},
		"client_secret": {ex.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ex.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("oidc token exchange: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", outbound.SignInHeader)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("oidc token exchange: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if closeErr := resp.Body.Close(); closeErr != nil {
		return "", fmt.Errorf("oidc token exchange: close response body: %w", closeErr)
	}
	if readErr != nil {
		return "", fmt.Errorf("oidc token exchange: read response: %w", readErr)
	}
	var parsed oidcTokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("oidc token exchange: parse response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oidc token exchange: status %d: %s", resp.StatusCode, parsed.Error)
	}
	if parsed.IDToken == "" {
		return "", errors.New("oidc token exchange: no id_token in response")
	}
	return parsed.IDToken, nil
}

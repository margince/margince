// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package googlejwt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
)

// ErrExchangeRefused marks a token endpoint that answered the assertion with an
// HTTP error. The status and the OAuth error code are all it keeps: the request
// body is the signed assertion, and Google's own prose is not repeated.
type ErrExchangeRefused struct {
	Status int
	// Code is the RFC 6749 error code, empty when the endpoint sent anything else.
	Code string
	// RetryAfter is the wait a throttling endpoint asked for, zero when none.
	RetryAfter time.Duration
}

func (e ErrExchangeRefused) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("googlejwt: the token endpoint refused the assertion: http %d", e.Status)
	}
	return fmt.Sprintf("googlejwt: the token endpoint refused the assertion: http %d, %s", e.Status, e.Code)
}

// Permanent reports a refusal that retrying cannot fix. It is a 4xx other than
// a throttle, as for a revoked key or a delegation nobody granted.
func (e ErrExchangeRefused) Permanent() bool {
	return e.Status >= 400 && e.Status < 500 && e.Status != http.StatusTooManyRequests
}

// ErrNoAccessToken is a 200 answer that names no access token.
var ErrNoAccessToken = errors.New("googlejwt: the token endpoint returned no access token")

// ErrUnreadableResponse is a 200 answer that could not be read whole.
var ErrUnreadableResponse = errors.New("googlejwt: the token response could not be read")

// Token is an access token and the lifetime the endpoint gave it, zero when it
// gave none.
type Token struct {
	AccessToken string
	ExpiresIn   time.Duration
}

var oauthErrorCode = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// maxTokenResponseBytes bounds the token endpoint's answer: a real one is a
// few hundred bytes.
const maxTokenResponseBytes = 1 << 20

// Exchange posts the signed assertion to tokenURL and returns the access token
// it is exchanged for. A transport error is returned as it came, so a caller
// can tell a timeout from a refusal.
func Exchange(ctx context.Context, client *http.Client, tokenURL, assertion string) (Token, error) {
	form := url.Values{"grant_type": {JWTBearerGrant}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, errors.New("googlejwt: building the token request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("googlejwt: token exchange: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close of a bounded body already read; the status and decode decide the outcome
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		AccessToken string      `json:"access_token"`
		ExpiresIn   json.Number `json:"expires_in"`
		Error       string      `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	decodeErr := json.Unmarshal(raw, &body)
	if resp.StatusCode != http.StatusOK {
		refused := ErrExchangeRefused{Status: resp.StatusCode, RetryAfter: retryafter.Of(resp)}
		if oauthErrorCode.MatchString(body.Error) {
			refused.Code = body.Error
		}
		return Token{}, refused
	}
	if readErr != nil || decodeErr != nil {
		return Token{}, ErrUnreadableResponse
	}
	if body.AccessToken == "" {
		return Token{}, ErrNoAccessToken
	}
	token := Token{AccessToken: body.AccessToken}
	if seconds, err := body.ExpiresIn.Int64(); err == nil && seconds > 0 {
		token.ExpiresIn = time.Duration(seconds) * time.Second
	}
	return token, nil
}

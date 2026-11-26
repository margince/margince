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
)

// ErrExchangeRefused marks a token endpoint that answered the assertion with an
// HTTP error. The status and the OAuth error code are all it keeps: the request
// body is the signed assertion, and Google's own prose is not repeated.
type ErrExchangeRefused struct {
	Status int
	// Code is the RFC 6749 error code, empty when Google sent anything else.
	Code string
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

var oauthErrorCode = regexp.MustCompile(`^[a-z_]{1,64}$`)

// maxTokenResponseBytes bounds the token endpoint's answer: a real one is a
// few hundred bytes.
const maxTokenResponseBytes = 16 << 10

// AccessToken posts the signed assertion to tokenURL and returns the access
// token it is exchanged for. The transport error is returned as it came, so a
// caller can tell a timeout from a refusal.
func AccessToken(ctx context.Context, client *http.Client, tokenURL, assertion string) (string, error) {
	form := url.Values{"grant_type": {JWTBearerGrant}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("googlejwt: building the token request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("googlejwt: token exchange: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close of a bounded body already read; the status and decode decide the outcome
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	decodeErr := json.Unmarshal(raw, &body)
	if resp.StatusCode != http.StatusOK {
		refused := ErrExchangeRefused{Status: resp.StatusCode}
		if oauthErrorCode.MatchString(body.Error) {
			refused.Code = body.Error
		}
		return "", refused
	}
	if readErr != nil || decodeErr != nil || body.AccessToken == "" {
		return "", errors.New("googlejwt: token exchange: the response carried no access token")
	}
	return body.AccessToken, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

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
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/googlejwt"
	"github.com/margince/margince/backend/internal/platform/netguard"
)

// vertexTokenURI is the one token endpoint a service-account assertion is
// ever posted to.
const vertexTokenURI = googlejwt.TokenURI

const (
	vertexScope          = "https://www.googleapis.com/auth/cloud-platform"
	vertexAssertionTTL   = time.Hour
	vertexRefreshHeadway = 5 * time.Minute
	vertexExchangeBudget = 30 * time.Second
)

// vertexProjectID is Google's own project-id rule. The id is spliced into
// every request path, so anything else is refused before it can shape one.
var vertexProjectID = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

// errInvalidServiceAccount marks a key file that cannot be used. Its
// messages name the field at fault and never the value.
var errInvalidServiceAccount = googlejwt.ErrInvalidKey

// vertexServiceAccount is a parsed key whose project id passed the path rule.
type vertexServiceAccount struct {
	googlejwt.ServiceAccount
}

func parseVertexServiceAccount(keyFile string) (vertexServiceAccount, error) {
	account, err := googlejwt.ParseServiceAccount(keyFile)
	if err != nil {
		return vertexServiceAccount{}, fmt.Errorf("ai: gemini_vertex: %w", err)
	}
	if !vertexProjectID.MatchString(account.ProjectID) {
		return vertexServiceAccount{}, fmt.Errorf("ai: gemini_vertex: %w: project_id is missing or not a Google project id", errInvalidServiceAccount)
	}
	return vertexServiceAccount{ServiceAccount: account}, nil
}

// assertion is the Vertex assertion: the service account acts as itself, so
// it names no subject.
func (a vertexServiceAccount) assertion(issuedAt time.Time) (string, error) {
	jwt, err := a.Assertion(googlejwt.Claims{Scope: vertexScope, IssuedAt: issuedAt, TTL: vertexAssertionTTL})
	if err != nil {
		return "", fmt.Errorf("ai: gemini_vertex: %w", err)
	}
	return jwt, nil
}

// vertexTokenSource mints and caches the access token one service account
// authorises Vertex calls with. A burst of callers on an expired token shares
// one exchange rather than spending one each.
type vertexTokenSource struct {
	account vertexServiceAccount
	http    *http.Client
	clock   Clock

	mu                   sync.Mutex
	token                string
	refreshAt, expiresAt time.Time
	inFlight             *tokenExchange
}

func newVertexTokenSource(account vertexServiceAccount, httpc *http.Client, clock Clock) *vertexTokenSource {
	// No redirect is followed: a redirected exchange would carry the
	// assertion to a host this code never named.
	return &vertexTokenSource{account: account, http: netguard.NoRedirect(httpc), clock: clock}
}

type tokenExchange struct {
	done  chan struct{}
	token string
	err   error
}

func (s *vertexTokenSource) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.token != "" && s.clock.Now().Before(s.refreshAt) {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	call := s.inFlight
	if call == nil {
		call = &tokenExchange{done: make(chan struct{})}
		s.inFlight = call
		go s.complete(ctx, call)
	}
	s.mu.Unlock()
	select {
	case <-call.done:
		return call.token, call.err
	case <-ctx.Done():
		return "", fmt.Errorf("ai: gemini_vertex: waiting for an access token: %w", ctx.Err())
	}
}

func (s *vertexTokenSource) complete(caller context.Context, call *tokenExchange) {
	// Detached so one caller giving up does not fail the exchange the others
	// are waiting on.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(caller), vertexExchangeBudget)
	defer cancel()
	issuedAt := s.clock.Now()
	token, lifetime, err := s.exchange(ctx, issuedAt)
	s.mu.Lock()
	switch {
	case err == nil:
		s.token, s.refreshAt, s.expiresAt = token, issuedAt.Add(refreshAfter(lifetime)), issuedAt.Add(lifetime)
	case s.token != "" && s.clock.Now().Before(s.expiresAt):
		// A refresh is early by design, so a failed one leaves a token Google
		// still honours; the next call tries the exchange again.
		token, err = s.token, nil
	}
	s.inFlight = nil
	s.mu.Unlock()
	call.token, call.err = token, err
	close(call.done)
}

// refreshAfter is how long a token is used before it is replaced: the
// headway before expiry, or half the lifetime of one too short to spare it.
func refreshAfter(lifetime time.Duration) time.Duration {
	if lifetime <= vertexRefreshHeadway {
		return lifetime / 2
	}
	return lifetime - vertexRefreshHeadway
}

// exchange posts one assertion for an access token. Its errors keep the HTTP
// status and the OAuth error code and nothing else: the request body is the
// assertion, and a transport error would render the URL it was sent to.
func (s *vertexTokenSource) exchange(ctx context.Context, issuedAt time.Time) (string, time.Duration, error) {
	assertion, err := s.account.assertion(issuedAt)
	if err != nil {
		return "", 0, err
	}
	form := url.Values{"grant_type": {googlejwt.JWTBearerGrant}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, vertexTokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, errors.New("ai: gemini_vertex: building the token request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("ai: gemini_vertex: token exchange: %w", parseFault(err))
	}
	//craft:ignore swallowed-errors best-effort close of a bounded body already read — the status and decode decide the outcome
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	decodeErr := json.Unmarshal(raw, &body)
	if resp.StatusCode != http.StatusOK {
		return "", 0, tokenExchangeRefused(resp.StatusCode, body.Error)
	}
	if readErr != nil || decodeErr != nil || body.AccessToken == "" || body.ExpiresIn <= 0 {
		return "", 0, errors.New("ai: gemini_vertex: token exchange: the response carried no usable access token")
	}
	return body.AccessToken, time.Duration(body.ExpiresIn) * time.Second, nil
}

// oauthErrorCode is the shape of an RFC 6749 error code. Anything else in
// that field is vendor prose and is not repeated.
var oauthErrorCode = regexp.MustCompile(`^[a-z_]{1,64}$`)

// errKeyRefused is Google refusing the key itself — a 4xx on the exchange
// other than a throttle, as for a revoked key or a deleted account — which
// waiting will not fix.
var errKeyRefused = errors.New("the service-account key was refused by Google")

func tokenExchangeRefused(status int, code string) error {
	detail := fmt.Sprintf("http %d", status)
	if oauthErrorCode.MatchString(code) {
		detail += ", " + code
	}
	if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
		return fmt.Errorf("ai: gemini_vertex: token exchange refused: %s: %w", detail, errKeyRefused)
	}
	return fmt.Errorf("ai: gemini_vertex: token exchange refused: %s", detail)
}

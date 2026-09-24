// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// vertexTokenURI is the one token endpoint a service-account assertion is
// ever posted to. A key naming any other is refused: the signed assertion is
// a bearer credential for an hour, and the key file is operator-supplied.
const vertexTokenURI = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: an endpoint address, not a credential

const (
	vertexScope          = "https://www.googleapis.com/auth/cloud-platform"
	vertexAssertionTTL   = time.Hour
	vertexRefreshHeadway = 5 * time.Minute
	vertexJWTBearerGrant = "urn:ietf:params:oauth:grant-type:jwt-bearer" //nolint:gosec // G101: an OAuth grant type name, not a credential
	vertexExchangeBudget = 30 * time.Second
)

// vertexProjectID is Google's own project-id rule. The id is spliced into
// every request path, so anything else is refused before it can shape one.
var vertexProjectID = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

// errInvalidServiceAccount marks a key file that cannot be used. Its
// messages name the field at fault and never the value.
var errInvalidServiceAccount = errors.New("ai: gemini_vertex: invalid service account key")

// vertexServiceAccount is the part of a Google service-account key file this
// adapter uses.
type vertexServiceAccount struct {
	clientEmail string
	projectID   string
	key         *rsa.PrivateKey
}

func parseVertexServiceAccount(keyFile string) (vertexServiceAccount, error) {
	var file struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		PrivateKey  string `json:"private_key"`
		ClientEmail string `json:"client_email"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(keyFile), &file); err != nil {
		return vertexServiceAccount{}, fmt.Errorf("%w: the key file is not a JSON object", errInvalidServiceAccount)
	}
	switch {
	case file.Type != "service_account":
		return vertexServiceAccount{}, fmt.Errorf("%w: type must be service_account", errInvalidServiceAccount)
	case !strings.Contains(file.ClientEmail, "@"):
		return vertexServiceAccount{}, fmt.Errorf("%w: client_email is missing or not an address", errInvalidServiceAccount)
	case !vertexProjectID.MatchString(file.ProjectID):
		return vertexServiceAccount{}, fmt.Errorf("%w: project_id is missing or not a Google project id", errInvalidServiceAccount)
	case file.TokenURI != "" && file.TokenURI != vertexTokenURI:
		return vertexServiceAccount{}, fmt.Errorf("%w: token_uri must be %s", errInvalidServiceAccount, vertexTokenURI)
	}
	key, err := parseRSAPrivateKey(file.PrivateKey)
	if err != nil {
		return vertexServiceAccount{}, err
	}
	return vertexServiceAccount{clientEmail: file.ClientEmail, projectID: file.ProjectID, key: key}, nil
}

// parseRSAPrivateKey reads a PEM key in either encoding Google has issued.
// The parser's own error is dropped: it describes the bytes it was given.
func parseRSAPrivateKey(pemText string) (*rsa.PrivateKey, error) {
	unreadable := fmt.Errorf("%w: private_key is not a PEM-encoded RSA key", errInvalidServiceAccount)
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, unreadable
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, unreadable
		}
		return key, nil
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, unreadable
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, unreadable
		}
		return key, nil
	default:
		return nil, unreadable
	}
}

// assertion is the RS256-signed JWT the token endpoint exchanges for an
// access token.
func (a vertexServiceAccount) assertion(issuedAt time.Time) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(struct {
		Issuer   string `json:"iss"`
		Audience string `json:"aud"`
		Scope    string `json:"scope"`
		IssuedAt int64  `json:"iat"`
		Expires  int64  `json:"exp"`
	}{a.clientEmail, vertexTokenURI, vertexScope, issuedAt.Unix(), issuedAt.Add(vertexAssertionTTL).Unix()})
	if err != nil {
		return "", fmt.Errorf("ai: gemini_vertex: encode assertion claims: %w", err)
	}
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", errors.New("ai: gemini_vertex: signing the token assertion failed")
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
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
	return &vertexTokenSource{account: account, http: noRedirect(httpc), clock: clock}
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
	form := url.Values{"grant_type": {vertexJWTBearerGrant}, "assertion": {assertion}}
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

func tokenExchangeRefused(status int, code string) error {
	if oauthErrorCode.MatchString(code) {
		return fmt.Errorf("ai: gemini_vertex: token exchange refused: http %d, %s", status, code)
	}
	return fmt.Errorf("ai: gemini_vertex: token exchange refused: http %d", status)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/googlejwt"
	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	// sharedScope is the one scope a Workspace admin delegates to the service
	// account for a shared mailbox: reading, nothing else.
	sharedScope        = "https://www.googleapis.com/auth/gmail.readonly"
	sharedAssertionTTL = time.Hour
)

// SharedCredential is the sealed bundle of a shared mailbox: the service
// account's key file and the mailbox it acts for under domain-wide delegation.
type SharedCredential struct {
	ServiceAccountKey string `json:"service_account_key"`
	Mailbox           string `json:"mailbox"`
}

// SealShared encodes the bundle the registry stores encrypted in the vault.
func SealShared(cred SharedCredential) (connector.Auth, error) {
	auth, err := json.Marshal(cred)
	if err != nil {
		return nil, fmt.Errorf("gmail: encoding shared credential: %w", err)
	}
	return auth, nil
}

// ReadShared opens a sealed shared-mailbox bundle.
func ReadShared(auth connector.Auth) (SharedCredential, error) {
	var cred SharedCredential
	if err := json.Unmarshal(auth, &cred); err != nil {
		return SharedCredential{}, fmt.Errorf("gmail: malformed shared credential: %w", err)
	}
	return cred, nil
}

// SharedAuthConfig wires the shared authorizer. Empty fields take Google's
// endpoint, a bounded client and the wall clock; tests set all three.
type SharedAuthConfig struct {
	TokenURL   string
	HTTPClient *http.Client
	Now        func() time.Time
}

// SharedAuthorizer mints the token a shared mailbox is read with: a service
// account acting for the mailbox address. The Gmail client then calls
// users/me unchanged, because the token already is the mailbox's.
type SharedAuthorizer struct {
	tokenURL string
	http     *http.Client
	now      func() time.Time
	tokens   *oauthflow.TokenCache
}

// NewSharedAuthorizer builds the authorizer. One instance serves every shared
// mailbox; the cache is keyed by key file and mailbox, never by a connection.
func NewSharedAuthorizer(cfg SharedAuthConfig) *SharedAuthorizer {
	if cfg.TokenURL == "" {
		cfg.TokenURL = googleEndpoints.token
	}
	client := cfg.HTTPClient
	if client == nil {
		client = boundedHTTPClient()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &SharedAuthorizer{
		tokenURL: cfg.TokenURL, http: netguard.NoRedirect(client), now: now, tokens: oauthflow.NewTokenCache(now),
	}
}

// AccessToken returns a read-only token for the sealed mailbox. A refused
// delegation (unauthorized_client) or an unusable key fails as
// ErrAuthRejected, which parks the connection rather than retrying it.
func (s *SharedAuthorizer) AccessToken(ctx context.Context, auth connector.Auth) (string, error) {
	cred, err := ReadShared(auth)
	if err != nil {
		return "", err
	}
	mailbox, err := oauthflow.MailboxAddress(cred.Mailbox)
	if err != nil {
		return "", fmt.Errorf("gmail: %w: %w", err, ErrAuthRejected)
	}
	account, err := googlejwt.ParseServiceAccount(cred.ServiceAccountKey)
	if err != nil {
		return "", fmt.Errorf("gmail: the shared mailbox key: %w: %w", err, ErrAuthRejected)
	}
	flow := oauthflow.New(oauthflow.Config{
		Provider: "gmail", TokenURL: s.tokenURL,
		AuthRejected: ErrAuthRejected, Unreachable: ErrUnreachable, HTTPClient: s.http,
	})
	key := oauthflow.CacheKey(cred.ServiceAccountKey, mailbox)
	return s.tokens.Token(ctx, key, func(ctx context.Context) (oauthflow.MintedToken, error) {
		assertion, err := account.Assertion(googlejwt.Claims{
			Scope: sharedScope, Subject: mailbox, IssuedAt: s.now(), TTL: sharedAssertionTTL,
		})
		if err != nil {
			return oauthflow.MintedToken{}, fmt.Errorf("gmail: %w", err)
		}
		return flow.JWTBearer(ctx, assertion)
	})
}

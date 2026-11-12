// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sharedScope asks for every application permission an admin consented to
// for the app; the client-credentials grant takes no narrower list.
const sharedScope = "https://graph.microsoft.com/.default"

// SharedCredential is the sealed bundle of a shared mailbox: the customer's
// own Entra app registration, and the mailbox that app reads.
type SharedCredential struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Mailbox      string `json:"mailbox"`
}

// SealShared encodes the bundle the registry stores encrypted in the vault.
func SealShared(cred SharedCredential) (connector.Auth, error) {
	//nolint:gosec // G117: sealing the app secret into the opaque Auth bundle is the intended path; the registry stores it encrypted
	auth, err := json.Marshal(cred)
	if err != nil {
		return nil, fmt.Errorf("graph: encoding shared credential: %w", err)
	}
	return auth, nil
}

// ReadShared opens a sealed shared-mailbox bundle.
func ReadShared(auth connector.Auth) (SharedCredential, error) {
	var cred SharedCredential
	if err := json.Unmarshal(auth, &cred); err != nil {
		return SharedCredential{}, fmt.Errorf("graph: malformed shared credential: %w", err)
	}
	return cred, nil
}

// SharedAuthConfig wires the shared authorizer. Empty fields take Microsoft's
// endpoint, a bounded client and the wall clock; tests set all three.
type SharedAuthConfig struct {
	// TokenURLFormat takes the tenant id as its one %s.
	TokenURLFormat string
	HTTPClient     *http.Client
	Now            func() time.Time
}

// SharedAuthorizer mints the application token a shared mailbox is read
// with. It holds no refresh token: the sealed app secret mints a new token
// whenever the cached one runs out.
type SharedAuthorizer struct {
	tokenURLFormat string
	http           *http.Client
	tokens         *oauthflow.TokenCache
}

// NewSharedAuthorizer builds the authorizer. One instance serves every shared
// mailbox; the cache is keyed by the credential, never by a connection.
func NewSharedAuthorizer(cfg SharedAuthConfig) *SharedAuthorizer {
	if cfg.TokenURLFormat == "" {
		cfg.TokenURLFormat = msTokenURLFormat
	}
	client := cfg.HTTPClient
	if client == nil {
		client = boundedHTTPClient()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &SharedAuthorizer{tokenURLFormat: cfg.TokenURLFormat, http: netguard.NoRedirect(client), tokens: oauthflow.NewTokenCache(now)}
}

// AccessToken returns an application token for the sealed credential. A
// refused app (a wrong secret, no admin consent) fails as ErrAuthRejected,
// which parks the connection rather than retrying it.
func (s *SharedAuthorizer) AccessToken(ctx context.Context, auth connector.Auth) (string, error) {
	cred, err := ReadShared(auth)
	if err != nil {
		return "", err
	}
	if err := cred.usable(); err != nil {
		return "", err
	}
	flow := oauthflow.New(oauthflow.Config{
		Provider:     connectorName,
		ClientID:     cred.ClientID,
		ClientSecret: cred.ClientSecret,
		TokenURL:     fmt.Sprintf(s.tokenURLFormat, url.PathEscape(cred.TenantID)),
		AuthRejected: ErrAuthRejected,
		Unreachable:  ErrUnreachable,
		HTTPClient:   s.http,
	})
	key := oauthflow.CacheKey(cred.TenantID, cred.ClientID, cred.ClientSecret)
	return s.tokens.Token(ctx, key, func(ctx context.Context) (oauthflow.MintedToken, error) {
		return flow.ClientCredentials(ctx, sharedScope)
	})
}

// tenantID is a directory id (a GUID) or a verified domain name. Nothing else
// may shape the token URL, and the multi-tenant aliases have no dot.
var tenantID = regexp.MustCompile(
	`^(?i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|` +
		`(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?)$`)

// usable refuses a bundle before it reaches Microsoft. A multi-tenant alias
// is refused because an app token is only ever issued by one tenant.
func (c SharedCredential) usable() error {
	if len(c.TenantID) > 253 || !tenantID.MatchString(c.TenantID) {
		return fmt.Errorf("graph: a shared mailbox needs its own tenant id: %w", ErrAuthRejected)
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return fmt.Errorf("graph: a shared mailbox needs a client id and secret: %w", ErrAuthRejected)
	}
	return nil
}

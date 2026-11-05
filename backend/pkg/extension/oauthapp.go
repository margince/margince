// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The OAuth surface. A unit that reads a vendor's API for the company declares
// the vendor's app, and the core runs the consent flow and keeps the tokens.
// A refresh needs the client secret, which an admin seals on Settings and only
// the core holds. So a unit asks Runtime.OAuthAccessToken before each call.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// OAuthApp is one vendor whose authorization-code flow the core runs for a
// unit. Admin-only: the core starts a flow for a human holding the
// integrations create grant, and the grant it yields is the workspace's.
type OAuthApp struct {
	// Provider names the vendor in `/v1/ext-oauth/<provider>/…` and keys the
	// stored client credentials. It is unique across the composed set and may
	// not shadow a core vendor. Changing it strands the stored app and grants.
	Provider string

	// Title is what Settings calls the app, one short line an admin
	// recognises ("Books ledger").
	Title string

	// AuthURL is the vendor's authorization endpoint, TokenURL its token
	// endpoint. Both absolute https URLs.
	AuthURL  string
	TokenURL string

	// Scopes are the scopes the consent asks for, sent space-separated.
	Scopes []string

	// AuthParams are extra query parameters for the authorization URL, for a
	// vendor that needs one to issue a refresh token. The parameters the core
	// sets itself (client_id, redirect_uri, response_type, scope, state) are
	// refused: each one would send the code elsewhere or break the flow.
	AuthParams map[string]string

	// OnGrant is optional. It runs once the core has stored the grant, with the
	// callback's extra parameters, as the admin who consented. A unit keeps what
	// it needs (a vendor's company id) in its own Secrets. An error fails the
	// connect: the core forgets the grant and the admin lands on an error.
	OnGrant OAuthGrantFunc
}

// OAuthProvider names one of the calling unit's own OAuth apps on the Runtime.
// A distinct type, as JobName is. It resolves against the declarations of the
// unit the Runtime was minted for, so it cannot name another unit's app.
type OAuthProvider string

// OAuthGrantFunc is what a unit does once an admin has consented.
type OAuthGrantFunc func(ctx context.Context, rt Runtime, grant OAuthGrant) error

// OAuthGrant is what a completed consent tells a unit. It carries no token:
// a unit reads one through Runtime.OAuthAccessToken when it calls the vendor.
type OAuthGrant struct {
	// Params are the callback's query parameters other than code, state and
	// error. Some vendors return what the grant is FOR there, such as the
	// company the admin picked on the consent screen.
	Params map[string]string
}

var (
	// ErrOAuthNotConnected answers a token request for an app no admin has
	// connected in this workspace, or one disconnected since.
	ErrOAuthNotConnected = errors.New("extension: no admin has connected this OAuth app in this workspace")

	// ErrOAuthRevoked answers a token request the vendor refused to refresh:
	// the admin revoked access, or the grant expired at the vendor. The core
	// has forgotten the grant, so the remedy is to connect again.
	ErrOAuthRevoked = errors.New("extension: the vendor refused this OAuth grant; an admin must connect it again")

	// ErrNoSuchOAuthApp refuses a provider the calling unit does not declare.
	// One that belongs to another unit reads the same, so a unit cannot learn
	// its neighbours' apps.
	ErrNoSuchOAuthApp = errors.New("extension: this unit declares no OAuth app for that provider")
)

// Validate holds what one declaration can decide. A provider claimed twice
// across the composed set, or one that shadows a core vendor, is refused at
// boot, where the whole set is known.
func (a OAuthApp) Validate() error {
	if !providerGrammar.MatchString(a.Provider) {
		return fmt.Errorf("%w: OAuth app provider %q must start with a letter and contain only lower-case letters, digits and underscores",
			ErrInvalid, a.Provider)
	}
	if len(a.Provider) > maxProviderLength {
		return fmt.Errorf("%w: OAuth app provider %q is longer than %d characters",
			ErrInvalid, a.Provider, maxProviderLength)
	}
	if strings.TrimSpace(a.Title) == "" {
		return fmt.Errorf("%w: OAuth app %q has no Title, and Settings lists the app by it", ErrInvalid, a.Provider)
	}
	for _, reserved := range coreAuthParams {
		if _, ok := a.AuthParams[reserved]; ok {
			return fmt.Errorf("%w: OAuth app %q sets AuthParams[%q], which the core sets itself", ErrInvalid, a.Provider, reserved)
		}
	}
	if err := requireHTTPS(a.AuthURL); err != nil {
		return fmt.Errorf("%w: OAuth app %q AuthURL: %w", ErrInvalid, a.Provider, err)
	}
	if err := requireHTTPS(a.TokenURL); err != nil {
		return fmt.Errorf("%w: OAuth app %q TokenURL: %w", ErrInvalid, a.Provider, err)
	}
	return nil
}

// coreAuthParams are the authorization parameters the core's flow owns.
var coreAuthParams = []string{"client_id", "redirect_uri", "response_type", "scope", "state"}

// requireHTTPS refuses anything but an absolute https URL. The token endpoint
// receives the client secret and the authorization code, so plain http would
// send both in the clear.
func requireHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%q is not an absolute https URL", raw)
	}
	return nil
}

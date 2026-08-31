// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package graphconn is the Microsoft plumbing shared by the Microsoft 365
// connectors: the credential bundle they seal, the connect payload the
// transport hands them, and the scope vocabulary they freeze at grant time.
//
// It is the Microsoft twin of capture/googleconn, and it exists for the same
// reason that one does. Two connectors now authorize against the same identity
// platform with the same handshake and differ only in which permission they
// ask for — the mailbox and the calendar. What they seal, and how they read it
// back, is one answer to one question, and two copies of it would drift on the
// field that matters: `Granted` is what MICROSOFT says it granted, in
// Microsoft's own vocabulary, and it is what a refresh must ask for. A copy
// that fell behind would ask an older grant for a permission it never
// consented to, which Microsoft refuses at the refresh — stopping a connection
// syncing rather than merely narrowing it.
//
// What is NOT here: the endpoints, the scopes, and the provider name. Those are
// each connector's own, and passing the name in is what keeps an error saying
// which connector produced it.
package graphconn

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// AuthState is the persisted credential bundle (the opaque connector.Auth).
//
// The refresh token is the durable secret; the short-lived access token is
// re-minted from it on every use and never stored.
type AuthState struct {
	RefreshToken string `json:"refresh_token"`
	Owner        string `json:"owner_email"`
	// Scopes is this system's INTERNAL permission vocabulary (the connector's
	// declared principal scopes), frozen at grant time.
	Scopes []string `json:"scopes"`
	// Granted is what MICROSOFT says it granted, in Microsoft's own vocabulary.
	// A separate field because the two vocabularies mean different things and
	// must never overwrite one another; empty for a bundle sealed before the
	// grant was recorded.
	Granted []string `json:"granted_scopes,omitempty"`
}

// Seal encodes the bundle the registry stores encrypted in the vault.
func Seal(provider string, st AuthState) (connector.Auth, error) {
	//nolint:gosec // G117: sealing the connector's own refresh token into the opaque Auth bundle IS the intended path — the registry stores it encrypted in the vault, never logged or returned
	auth, err := json.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("%s: encoding auth state: %w", provider, err)
	}
	return auth, nil
}

// Read opens a sealed bundle. One reader, so every entry point reports a
// malformed bundle the same way rather than several spellings of one failure.
func Read(provider string, auth connector.Auth) (AuthState, error) {
	var st AuthState
	if err := json.Unmarshal(auth, &st); err != nil {
		return AuthState{}, fmt.Errorf("%s: malformed auth bundle: %w", provider, err)
	}
	return st, nil
}

// authPayload is the connect request the transport hands to Authenticate: the
// OAuth authorization code and the redirect URI it was issued against.
type authPayload struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

// AuthRequestFrom packages an OAuth callback's code into the opaque connector
// AuthRequest the callback handler passes to Authenticate.
func AuthRequestFrom(provider, code, redirectURI string) (connector.AuthRequest, error) {
	payload, err := json.Marshal(authPayload{Code: code, RedirectURI: redirectURI})
	if err != nil {
		return connector.AuthRequest{}, fmt.Errorf("%s: encoding auth payload: %w", provider, err)
	}
	return connector.AuthRequest{Payload: payload}, nil
}

// ReadAuthRequest opens that payload again on the connector's side.
func ReadAuthRequest(provider string, req connector.AuthRequest) (code, redirectURI string, err error) {
	var p authPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return "", "", fmt.Errorf("%s: malformed auth payload: %w", provider, err)
	}
	return p.Code, p.RedirectURI, nil
}

// ScopeStrings renders this system's own principal scopes for the bundle.
func ScopeStrings(scopes []principal.Scope) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, string(s))
	}
	return out
}

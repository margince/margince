// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// oauthGrantTypesSupported is what discovery advertises and what a
// registration may ask for. refresh_token is in it because a client that
// cannot see it advertised never presents one: it asks for offline_access,
// stores the token it gets, and never renews with it.
var oauthGrantTypesSupported = []string{oauthGrantAuthorizationCode, oauthRefreshToken}

// dcrSubstitutableAuthMethods are the token_endpoint_auth_method values a
// registration may ask for; each is registered as none. A key- or TLS-bound
// method is refused, since a client that asks for one cannot work without it.
var dcrSubstitutableAuthMethods = []string{"", oauthAuthMethodNone, oauthAuthMethodSecretBasic, oauthAuthMethodSecretPost}

// dcrRequest is the part of an RFC 7591 registration document this server
// acts on.
type dcrRequest struct {
	RedirectURIs            []string
	ClientName              string
	TokenEndpointAuthMethod string
	GrantTypes              []string
	ResponseTypes           []string
}

// dcrRefusal is a refusal RFC 7591 §3.2.2 names: the code is the error
// member, the message its description. A concrete type rather than an error,
// so the endpoint cannot answer anything but an RFC 7591 document.
type dcrRefusal struct {
	code, description string
}

// dcrLoggedReasonBytes bounds the logged reason: a description can quote a
// redirect URI the caller sent, and the body may be 1 MiB.
const dcrLoggedReasonBytes = 200

func (r *dcrRefusal) logged() string {
	if len(r.description) <= dcrLoggedReasonBytes {
		return r.description
	}
	return strings.ToValidUTF8(r.description[:dcrLoggedReasonBytes], "") + "…"
}

// parseDCR ignores every member it does not read: a client registering with
// several servers sends the union of what they all understand.
func parseDCR(members map[string]json.RawMessage) (dcrRequest, *dcrRefusal) {
	var req dcrRequest
	var err *dcrRefusal
	if req.RedirectURIs, err = dcrMember[[]string](members, "redirect_uris"); err != nil {
		return dcrRequest{}, err
	}
	if req.ClientName, err = dcrMember[string](members, "client_name"); err != nil {
		return dcrRequest{}, err
	}
	if req.TokenEndpointAuthMethod, err = dcrMember[string](members, "token_endpoint_auth_method"); err != nil {
		return dcrRequest{}, err
	}
	if req.GrantTypes, err = dcrMember[[]string](members, "grant_types"); err != nil {
		return dcrRequest{}, err
	}
	if req.ResponseTypes, err = dcrMember[[]string](members, "response_types"); err != nil {
		return dcrRequest{}, err
	}
	return req, req.validate()
}

func (req dcrRequest) validate() *dcrRefusal {
	// Public clients only: PKCE is the proof of possession, and only an admin
	// mints a secret. A client asking for a secret method is registered as
	// public instead, which §3.2.1 allows, and the echo of none tells it so.
	// The Gemini app is one client that asks for client_secret_basic.
	if !slices.Contains(dcrSubstitutableAuthMethods, req.TokenEndpointAuthMethod) {
		return &dcrRefusal{
			oauthErrInvalidClientMetadata,
			fmt.Sprintf("token_endpoint_auth_method: %q is not served here; register with %q",
				req.TokenEndpointAuthMethod, oauthAuthMethodNone),
		}
	}
	if req.ClientName == "" || len(req.RedirectURIs) == 0 {
		return &dcrRefusal{oauthErrInvalidClientMetadata, "client_name and redirect_uris are required"}
	}
	for _, raw := range req.RedirectURIs {
		if !validRedirectURI(raw) {
			return &dcrRefusal{
				oauthErrInvalidRedirectURI,
				fmt.Sprintf("%q: redirect uris must be https, or http on localhost", raw),
			}
		}
	}
	// An empty list names nothing, so it reads as the member omitted; the echo
	// tells the client what it holds either way.
	if len(req.GrantTypes) > 0 && !slices.Contains(req.GrantTypes, oauthGrantAuthorizationCode) {
		return &dcrRefusal{
			oauthErrInvalidClientMetadata,
			"grant_types: every connection begins with authorization_code, so the list must include it",
		}
	}
	for _, grant := range req.GrantTypes {
		if !slices.Contains(oauthGrantTypesSupported, grant) {
			return &dcrRefusal{
				oauthErrInvalidClientMetadata,
				fmt.Sprintf("grant_types: %q is not issued here; supported: %v", grant, oauthGrantTypesSupported),
			}
		}
	}
	for _, responseType := range req.ResponseTypes {
		if responseType != oauthResponseTypeCode {
			return &dcrRefusal{
				oauthErrInvalidClientMetadata,
				fmt.Sprintf("response_types: %q is not served here; only %q is", responseType, oauthResponseTypeCode),
			}
		}
	}
	return nil
}

// dcrRegistration is the metadata the 201 echoes (§3.2.1).
type dcrRegistration struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// registered echoes what this server does for every public client, which
// §3.2.1 lets it substitute for what was asked: nothing stores a client's
// grant list, and whether a refresh token is issued follows offline_access
// at consent, not registration.
func (req dcrRequest) registered(clientID string) dcrRegistration {
	return dcrRegistration{
		ClientID: clientID, ClientName: req.ClientName, RedirectURIs: req.RedirectURIs,
		TokenEndpointAuthMethod: oauthAuthMethodNone,
		GrantTypes:              oauthGrantTypesSupported,
		ResponseTypes:           []string{oauthResponseTypeCode},
	}
}

// dcrMember decodes one member by its exact name; absent and null both read
// as the zero value, which §2 makes the same thing as not sending it.
func dcrMember[T any](members map[string]json.RawMessage, name string) (T, *dcrRefusal) {
	var value T
	raw, present := members[name]
	if !present || string(raw) == "null" {
		return value, nil
	}
	if json.Unmarshal(raw, &value) != nil {
		return value, &dcrRefusal{
			oauthErrInvalidClientMetadata,
			fmt.Sprintf("%s: the value has the wrong type for this member", name),
		}
	}
	return value, nil
}

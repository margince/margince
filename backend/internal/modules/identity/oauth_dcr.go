// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
	"fmt"
	"slices"
)

// oauthGrantTypesSupported is what discovery advertises and what a
// registration may ask for. refresh_token is in it because a client that
// cannot see it advertised never presents one: it asks for offline_access,
// stores the token it gets, and never renews with it.
var oauthGrantTypesSupported = []string{oauthGrantAuthorizationCode, oauthRefreshToken}

// dcrRequest is the part of an RFC 7591 registration document this server
// acts on.
type dcrRequest struct {
	RedirectURIs            []string
	ClientName              string
	TokenEndpointAuthMethod string
	GrantTypes              []string
	ResponseTypes           []string
}

// dcrMetadataError is a refusal RFC 7591 §3.2.2 names: the code is the
// error member, the message its description.
type dcrMetadataError struct {
	code, description string
}

func (e *dcrMetadataError) Error() string { return e.description }

// parseDCR reads the members this server acts on, by their exact names, and
// ignores every other one — §2 obliges it to, and a client registering with
// several servers sends the union of what they all understand. A member it
// does read is held to what this server can honour: accepting a grant it
// never issues would tell the client a promise it will find broken later.
func parseDCR(members map[string]json.RawMessage) (dcrRequest, error) {
	var req dcrRequest
	var err error
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

func (req dcrRequest) validate() error {
	// Public clients only: PKCE is the proof of possession. A client
	// asking for a secret-based method is asking to be privileged —
	// refused, and there is no column to store a secret in anyway.
	if req.TokenEndpointAuthMethod != "" && req.TokenEndpointAuthMethod != "none" {
		return &dcrMetadataError{"invalid_client_metadata",
			"only public clients register here (token_endpoint_auth_method must be none)"}
	}
	if req.ClientName == "" || len(req.RedirectURIs) == 0 {
		return &dcrMetadataError{"invalid_client_metadata", "client_name and redirect_uris are required"}
	}
	for _, raw := range req.RedirectURIs {
		if !validRedirectURI(raw) {
			return &dcrMetadataError{"invalid_redirect_uri",
				fmt.Sprintf("%q: redirect uris must be https, or http on localhost", raw)}
		}
	}
	for _, grant := range req.GrantTypes {
		if !slices.Contains(oauthGrantTypesSupported, grant) {
			return &dcrMetadataError{"invalid_client_metadata",
				fmt.Sprintf("grant_types: %q is not issued here; supported: %v", grant, oauthGrantTypesSupported)}
		}
	}
	for _, responseType := range req.ResponseTypes {
		if responseType != oauthResponseTypeCode {
			return &dcrMetadataError{"invalid_client_metadata",
				fmt.Sprintf("response_types: %q is not served here; only %q is", responseType, oauthResponseTypeCode)}
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

// registered answers an omitted grant or response type list with the RFC's
// default rather than nothing, so the client reads what it actually holds.
func (req dcrRequest) registered(clientID string) dcrRegistration {
	echo := dcrRegistration{
		ClientID: clientID, ClientName: req.ClientName, RedirectURIs: req.RedirectURIs,
		TokenEndpointAuthMethod: "none",
		GrantTypes:              req.GrantTypes, ResponseTypes: req.ResponseTypes,
	}
	if len(echo.GrantTypes) == 0 {
		echo.GrantTypes = []string{oauthGrantAuthorizationCode}
	}
	if len(echo.ResponseTypes) == 0 {
		echo.ResponseTypes = []string{oauthResponseTypeCode}
	}
	return echo
}

// dcrMember decodes one member by its exact name; absent and null both read
// as the zero value, which §2 makes the same thing as not sending it.
func dcrMember[T any](members map[string]json.RawMessage, name string) (T, error) {
	var value T
	raw, present := members[name]
	if !present || string(raw) == "null" {
		return value, nil
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, &dcrMetadataError{"invalid_client_metadata",
			fmt.Sprintf("%s: the value has the wrong type for this member", name)}
	}
	return value, nil
}

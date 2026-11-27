// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oidctoken

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Audience is the aud claim. RFC 7519 lets a token send it as one string or
// as a list; Dispact sends a list of one.
type Audience []string

// UnmarshalJSON reads either form.
func (a *Audience) UnmarshalJSON(raw []byte) error {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		*a = Audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return fmt.Errorf("aud is neither a string nor a list of strings: %w", err)
	}
	*a = many
	return nil
}

// Is reports a token meant for aud alone. A token naming several audiences is
// refused: no check here expects one, and accepting one would trust a token
// minted for another party as well.
func (a Audience) Is(aud string) bool { return len(a) == 1 && a[0] == aud }

func (a Audience) String() string { return strings.Join(a, " ") }

// Claims are the ID token fields a sign-in or push check reads.
type Claims struct {
	Nbf               int64    `json:"nbf"`
	ServiceURL        string   `json:"serviceurl"`
	Iss               string   `json:"iss"`
	Aud               Audience `json:"aud"`
	Sub               string   `json:"sub"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	Exp               int64    `json:"exp"`
	Iat               int64    `json:"iat"`
	Tid               string   `json:"tid"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
	Hd                string   `json:"hd"`
	GivenName         string   `json:"given_name"`
	Picture           string   `json:"picture"`
	Jti               string   `json:"jti"`
	WorkspaceID       string   `json:"workspace_id"`
	DisplayName       string   `json:"display_name"`
}

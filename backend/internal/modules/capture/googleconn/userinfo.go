// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package googleconn

import (
	"context"
	"net/http"
)

// ProfileScope lets a consent read the account's public profile, which names its picture.
const ProfileScope = "https://www.googleapis.com/auth/userinfo.profile"

// userinfoBase is Google's OpenID userinfo root; overridable via NewUserinfo for tests.
const userinfoBase = "https://www.googleapis.com/oauth2/v3"

// Userinfo reads the connected account's profile.
type Userinfo struct {
	client *http.Client
	base   string
}

// NewUserinfo builds the profile reader over client and base, Google's when empty.
func NewUserinfo(client *http.Client, base string) Userinfo {
	if client == nil {
		client = BoundedClient()
	}
	if base == "" {
		base = userinfoBase
	}
	return Userinfo{client: client, base: base}
}

// Picture answers the account's picture URL, "" when it has none.
func (u Userinfo) Picture(ctx context.Context, accessToken string) (string, error) {
	var out struct {
		Picture string `json:"picture"`
	}
	if _, err := Get(ctx, u.client, u.base, accessToken, "/userinfo", nil, &out); err != nil {
		return "", err
	}
	return out.Picture, nil
}

// Email answers the account's address. It needs the userinfo.email scope.
func (u Userinfo) Email(ctx context.Context, accessToken string) (string, error) {
	var out struct {
		Email string `json:"email"`
	}
	if _, err := Get(ctx, u.client, u.base, accessToken, "/userinfo", nil, &out); err != nil {
		return "", err
	}
	return out.Email, nil
}

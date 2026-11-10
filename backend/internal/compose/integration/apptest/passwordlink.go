// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// SetPasswordByLink sets an invited user's password the way they do. The admin
// on e issues a set-password link, and following it also makes the seat active.
func SetPasswordByLink(t *testing.T, e *AppEnv, userID, password string) {
	t.Helper()
	var link struct {
		SetPasswordURL string `json:"set_password_url"`
	}
	if status := e.Call(t, "POST", "/v1/users/"+userID+"/password-link", nil, nil, &link); status != http.StatusCreated {
		t.Fatalf("issuing a set-password link for %s → %d", userID, status)
	}
	body := map[string]string{"token": PasswordLinkToken(t, link.SetPasswordURL), "new_password": password}
	if status := e.Call(t, "POST", "/v1/auth/reset-password", body, nil, nil); status != http.StatusNoContent {
		t.Fatalf("following the set-password link for %s → %d", userID, status)
	}
}

// PasswordLinkToken reads the token out of the link's fragment, which is
// shaped `/reset-password?token=…` because the SPA routes on the fragment.
func PasswordLinkToken(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("set-password link %q: %v", link, err)
	}
	_, rawQuery, found := strings.Cut(u.Fragment, "?")
	if !found {
		t.Fatalf("set-password link %q carries no query in its fragment", link)
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("set-password link %q: %v", link, err)
	}
	token := query.Get("token")
	if token == "" {
		t.Fatalf("set-password link %q carries no token", link)
	}
	return token
}

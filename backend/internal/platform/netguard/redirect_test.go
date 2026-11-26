// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package netguard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestARedirectComesBackAsTheResponseAndIsNotFollowed(t *testing.T) {
	followed := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed = true }))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)

	shared := origin.Client()
	resp, err := NoRedirect(shared).Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusTemporaryRedirect || followed {
		t.Errorf("status %d, followed %v; want the 307 back and no request to the target", resp.StatusCode, followed)
	}
	if shared.CheckRedirect != nil {
		t.Error("NoRedirect changed the caller's own client")
	}
}

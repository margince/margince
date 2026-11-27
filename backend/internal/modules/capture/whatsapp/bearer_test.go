// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The access token goes to the Graph API and to Meta's lookaside host. A media
// CDN gets none, and no redirect hop carries it on.
func TestTheTokenNeverReachesAMediaCDNOrARedirectHop(t *testing.T) {
	var cdnAuth, hopAuth string
	var cdnSeen, hopSeen bool
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnSeen, cdnAuth = true, r.Header.Get("Authorization")
		if _, err := w.Write([]byte("x")); err != nil {
			t.Error(err)
		}
	}))
	defer cdn.Close()
	graph := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hopSeen, hopAuth = true, r.Header.Get("Authorization")
		http.Redirect(w, r, cdn.URL+"/after-redirect", http.StatusFound)
	}))
	defer graph.Close()
	c := NewClient(graph.Client(), graph.URL, "", WithMediaHosts("127.0.0.1"), WithoutDialGuardForTest())

	if _, err := c.DownloadURL(context.Background(), testToken, cdn.URL+"/m", 10); err != nil {
		t.Fatal(err)
	}
	if !cdnSeen || cdnAuth != "" {
		t.Errorf("the CDN saw Authorization %q (seen %v), want none", cdnAuth, cdnSeen)
	}

	cdnSeen, cdnAuth = false, ""
	if _, err := c.DownloadURL(context.Background(), testToken, graph.URL+"/start", 10); err != nil {
		t.Fatal(err)
	}
	if !hopSeen || hopAuth == "" {
		t.Errorf("the Graph host saw Authorization %q, want the token", hopAuth)
	}
	if !cdnSeen || cdnAuth != "" {
		t.Errorf("the redirected request saw Authorization %q (seen %v), want none", cdnAuth, cdnSeen)
	}
}

func TestOnlyTheGraphHostAndMetasLookasideHostTakeTheToken(t *testing.T) {
	c := NewClient(nil, "https://graph.facebook.com", "")
	for raw, want := range map[string]bool{
		"https://graph.facebook.com/v27.0/1": true,
		"https://lookaside.fbsbx.com/x":      true,
		"https://mmg.whatsapp.net/x":         false,
		"https://other.fbsbx.com/x":          false,
		"https://lookaside.fbsbx.com.evil/x": false,
	} {
		if got := c.bearerHost(raw); got != want {
			t.Errorf("bearerHost(%q) = %v, want %v", raw, got, want)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

var errLeftTheHost = errors.New("the hop left the allowed host")

func redirectingServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(http.StatusNotFound)
		case "/picture":
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAGuardedFetchRefusesARedirectItsGuardRefuses(t *testing.T) {
	server := redirectingServer(t)
	ctx := WithRedirectGuard(context.Background(), func(target *url.URL) error {
		if target.Path == "/elsewhere" {
			return errLeftTheHost
		}
		return nil
	})
	if _, _, err := newFetcher(server.Client().Transport).FetchAsset(ctx, server.URL+"/picture"); !errors.Is(err, errLeftTheHost) {
		t.Errorf("a redirect the guard refuses = %v, want the guard's refusal", err)
	}
}

func TestAnUnguardedFetchFollowsTheSameRedirect(t *testing.T) {
	server := redirectingServer(t)
	if _, _, err := newFetcher(server.Client().Transport).FetchAsset(context.Background(), server.URL+"/picture"); err != nil {
		t.Errorf("an unguarded fetch refused a redirect: %v", err)
	}
}

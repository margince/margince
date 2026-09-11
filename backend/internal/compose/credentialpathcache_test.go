// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every credential-bearing public path answers with Cache-Control: no-store,
// and the header is set before anything downstream can decide otherwise.
//
// The bug this pins is an ordering one, and it shipped: the two public token
// edges each set the header themselves, but the session middleware wraps them
// and answers first when the installation is not bootstrapped. That answer left
// with no Cache-Control while the per-edge lines made the surface look covered,
// and no test could see it because none drove a layer above the edges.
//
// So these cases drive the wrapper with a stand-in for everything below it,
// including a stand-in that answers WITHOUT calling through — the shape of the
// middleware that used to leak. The companion census in compose/integration
// proves the same header survives the real handlers over live tokens.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
)

// answersWithout stands in for a middleware that answers on its own instead of
// calling through — the session middleware's not-bootstrapped branch. If the
// header were set below this, its answer would carry none.
func answersWithout(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
}

func TestEveryCredentialPathAnswersUncacheable(t *testing.T) {
	prefixes := capabilitypath.CredentialPrefixes()
	if len(prefixes) == 0 {
		t.Fatal("capabilitypath names no credential-bearing prefix, so this test has no subject " +
			"and would pass however many uncacheable answers leaked")
	}

	// Driven for EVERY prefix the tree calls credential-bearing, not for a
	// chosen few: adding one to capabilitypath brings it under this assertion
	// automatically, which is the point of reading the list rather than
	// restating it.
	for _, prefix := range prefixes {
		for _, tc := range []struct {
			name   string
			path   string
			status int
		}{
			{"a live token's answer", prefix + "sometoken", http.StatusOK},
			{"an answer from above the edges", prefix + "sometoken", http.StatusServiceUnavailable},
			{"a refusal", prefix + "sometoken", http.StatusNotFound},
			{"the bare prefix", prefix, http.StatusNotFound},
			{"a deeper path", prefix + "sometoken/unsubscribe", http.StatusOK},
		} {
			rec := httptest.NewRecorder()
			noStoreOnCredentialPaths(answersWithout(tc.status)).
				ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))

			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("%s: %s → %d carried Cache-Control %q, want %q — this path carries a "+
					"credential in a segment and a shared cache holding its answer is a leak",
					tc.name, capabilitypath.Redact(tc.path), tc.status, got, "no-store")
			}
		}
	}
}

// ServeMux answers a path needing canonicalization with its own 307 BEFORE any
// registered handler runs, and that redirect echoes the cleaned path — the
// credential segment included — into a Location header. Mounted under the /v1
// pattern rather than around the mux, this middleware never saw those answers,
// and no case here would have said so.
func TestTheMuxsOwnRedirectIsUncacheable(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/v1/", answersWithout(http.StatusOK))
	handler := noStoreOnCredentialPaths(mux)

	for _, path := range []string{
		"/v1/public/confirm//sometoken",
		"/v1/public/confirm/a/../sometoken",
		"/v1/public/preferences//sometoken",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))

		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("%s → %d, want a 307 — this case no longer drives the mux's own redirect, "+
				"so it proves nothing about it", capabilitypath.Redact(path), rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: the mux's redirect carried Cache-Control %q, want %q — its Location "+
				"header echoes the cleaned path, credential segment and all",
				capabilitypath.Redact(path), got, "no-store")
		}
	}
}

// The wiring itself, not a hand-built copy of it.
//
// Every other case in this file constructs noStoreOnCredentialPaths directly,
// which assumes the very placement it claims to check: nest the wrapper back
// UNDER the session middleware and they all still pass, while the
// not-bootstrapped answer leaks again exactly as it did before. So this reads
// the composed handler and asserts the order of the two names in the source
// that wires them.
//
// A source assertion rather than a driven request because the alternative needs
// an installation that is NOT bootstrapped, and every harness in the tree
// bootstraps one — the state that leaked is the state no test environment
// produces, which is why it survived.
func TestTheHeaderIsWiredAboveTheSessionMiddleware(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("reading the wiring: %v", err)
	}
	wiring := string(raw)

	wrapper := strings.Index(wiring, "noStoreOnCredentialPaths(mux)")
	if wrapper < 0 {
		t.Fatal("server.go no longer wraps the mux in noStoreOnCredentialPaths. If the header " +
			"moved, it must still sit above every layer that can answer on a credential path — " +
			"the session middleware and the mux's own canonicalization redirect included")
	}

	// The mux holds the session middleware, so wrapping the mux is what puts
	// the header above it. Wrapping anything the mux mounts does not.
	if strings.Contains(wiring, "authH.Middleware(noStoreOnCredentialPaths") {
		t.Error("noStoreOnCredentialPaths is nested UNDER the session middleware, which answers " +
			"a not-bootstrapped installation before reaching it — the leak this middleware exists " +
			"to close")
	}
}

// A path that carries no credential must not be stamped. A no-store on an
// authenticated API answer would be harmless, but on a cacheable one it is a
// regression nobody would attribute to this middleware.
func TestAPathWithNoCredentialIsLeftAlone(t *testing.T) {
	for _, path := range []string{
		"/v1/contacts",
		"/v1/public/rooms/peek",
		"/v1/public",
		"/healthz",
	} {
		rec := httptest.NewRecorder()
		noStoreOnCredentialPaths(answersWithout(http.StatusOK)).
			ServeHTTP(rec, httptest.NewRequest("GET", path, nil))

		if got := rec.Header().Get("Cache-Control"); got != "" {
			t.Errorf("%s was stamped Cache-Control %q, but it carries no credential segment",
				path, got)
		}
	}
}

// The header must be set BEFORE the handler starts writing, and a recorder will
// not show that on its own: httptest.ResponseRecorder happily accepts a header
// written after WriteHeader, where a real connection has already flushed it. So
// this drives a real server over a real socket, which is the only place the
// ordering is observable — moving the Set below next.ServeHTTP passes every
// recorder-based case in this file and fails here.
func TestTheHeaderIsSetBeforeTheBodyStarts(t *testing.T) {
	srv := httptest.NewServer(noStoreOnCredentialPaths(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			if _, err := w.Write([]byte(`{"title":"not found"}`)); err != nil {
				t.Errorf("writing the stand-in body: %v", err)
			}
		})))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/v1/public/confirm/sometoken")
	if err != nil {
		t.Fatalf("driving the credential path: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the body: %v", err)
		}
	}()

	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q over a real connection, want no-store — the header was set "+
			"after the response began, where it no longer reaches the client", got)
	}
}

// The header must reach the response even when the handler writes its own,
// which is the case for every answer the API actually gives.
func TestTheHeaderSurvivesAHandlerThatWritesItsOwn(t *testing.T) {
	rec := httptest.NewRecorder()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		if _, err := w.Write([]byte(`{"title":"not found"}`)); err != nil {
			t.Errorf("writing the stand-in body: %v", err)
		}
	})
	noStoreOnCredentialPaths(inner).
		ServeHTTP(rec, httptest.NewRequest("GET", "/v1/public/confirm/sometoken", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("the wrapper displaced the handler's own header: Content-Type = %q", got)
	}
}

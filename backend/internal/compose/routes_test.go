// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGateMetricsUnconfiguredServesOpenly pins the default, which is the
// whole point of the one-knob design: a deployment that configures nothing
// gets a scrapeable endpoint. An annotation-discovered Prometheus reads a
// target's address and metrics path off the Kubernetes API and has nowhere to
// carry a credential, so a default that refused it left the ordinary
// deployment with no working configuration at all.
func TestGateMetricsUnconfiguredServesOpenly(t *testing.T) {
	called := false
	h := gateMetrics("", func(http.ResponseWriter, *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("the wrapped handler did not run with no token configured")
	}
}

// TestGateMetricsUnconfiguredIgnoresAPresentedCredential: an open endpoint
// refuses nobody. A scraper carrying a bearer for some other target must not
// be answered 401 by the one that asks for none.
func TestGateMetricsUnconfiguredIgnoresAPresentedCredential(t *testing.T) {
	called := false
	h := gateMetrics("", func(http.ResponseWriter, *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer some-other-targets-secret")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("the wrapped handler did not run for a credential the open endpoint does not read")
	}
}

// TestGateMetricsConfiguringATokenClosesIt is the pair of the two above, and
// the invariant the single knob rests on: the token is the ONE thing that
// turns the open default into a closed endpoint. Nothing else has to be set,
// and nothing else can undo it.
func TestGateMetricsConfiguringATokenClosesIt(t *testing.T) {
	called := false
	h := gateMetrics("s3cr3t", func(http.ResponseWriter, *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Fatal("configuring a token left the exposition open")
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="metrics"` {
		t.Errorf("WWW-Authenticate = %q, want the Bearer challenge naming the realm", got)
	}
}

// TestGateMetricsRejectsMissingOrWrongBearer covers both ways an
// unauthenticated or mistaken caller can reach a configured endpoint.
func TestGateMetricsRejectsMissingOrWrongBearer(t *testing.T) {
	cases := []struct {
		name string
		auth string
	}{
		{"no header", ""},
		{"wrong token", "Bearer not-the-secret"},
		{"missing Bearer prefix", "s3cr3t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := gateMetrics("s3cr3t", func(http.ResponseWriter, *http.Request) { called = true })

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if called {
				t.Fatal("the wrapped handler ran without a matching bearer token")
			}
		})
	}
}

// TestGateMetricsAcceptsMatchingBearer is the positive control: the
// gate must not refuse the credential it was configured to accept.
func TestGateMetricsAcceptsMatchingBearer(t *testing.T) {
	called := false
	h := gateMetrics("s3cr3t", func(http.ResponseWriter, *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("the wrapped handler did not run with a matching bearer token")
	}
}

// TestGateMetricsAcceptsCaseInsensitiveScheme pins RFC 7235 §2.1:
// the auth-scheme token is case-insensitive, so "bearer"/"BEARER" must be
// accepted exactly like "Bearer" — only the credential itself is case-
// sensitive.
func TestGateMetricsAcceptsCaseInsensitiveScheme(t *testing.T) {
	for _, scheme := range []string{"bearer", "BEARER", "BeArEr"} {
		t.Run(scheme, func(t *testing.T) {
			called := false
			h := gateMetrics("s3cr3t", func(http.ResponseWriter, *http.Request) { called = true })

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			req.Header.Set("Authorization", scheme+" s3cr3t")
			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if !called {
				t.Fatalf("the wrapped handler did not run for scheme %q", scheme)
			}
		})
	}
}

// TestWithMetricsTokenSetsServerField pins the Option's one job: it must
// not silently miswire onto the wrong field or require a pool.
func TestWithMetricsTokenSetsServerField(t *testing.T) {
	var s Server
	WithMetricsToken("s3cr3t")(&s, nil)
	if s.metricsToken != "s3cr3t" {
		t.Fatalf("metricsToken = %q, want %q", s.metricsToken, "s3cr3t")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGateMetricsUnconfiguredRefusesEveryScrape pins the default: a deployment
// that configures nothing serves the exposition to nobody. The listener is the
// one /v1 is routed to, and the exposition names every route and carries
// workspace-labelled telemetry, so forgetting a setting must not publish it.
//
// The empty bearer is the case that matters most: an empty configured token and
// an empty presented one compare equal in constant time, so a gate that relied
// on the comparison alone would serve every caller who sends nothing.
func TestGateMetricsUnconfiguredRefusesEveryScrape(t *testing.T) {
	for _, auth := range []string{"", "Bearer", "Bearer ", "Bearer some-other-targets-secret"} {
		t.Run(auth, func(t *testing.T) {
			called := false
			h := gateMetrics("", false, func(http.ResponseWriter, *http.Request) { called = true })

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if called {
				t.Fatal("the exposition was served with no token configured and no explicit open posture")
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="metrics"` {
				t.Errorf("WWW-Authenticate = %q, want the Bearer challenge naming the realm", got)
			}
		})
	}
}

// TestGateMetricsOpenedExplicitlyServesAnyScrape is the pair of the default: an
// operator who declared the port contained gets the credential-less scrape an
// annotation-discovered Prometheus needs, and a scraper carrying a bearer for
// some other target is not refused by the endpoint that asks for none.
func TestGateMetricsOpenedExplicitlyServesAnyScrape(t *testing.T) {
	for _, auth := range []string{"", "Bearer some-other-targets-secret"} {
		t.Run(auth, func(t *testing.T) {
			called := false
			h := gateMetrics("", true, func(http.ResponseWriter, *http.Request) { called = true })

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			rec := httptest.NewRecorder()
			h(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if !called {
				t.Fatal("the wrapped handler did not run on an explicitly opened endpoint")
			}
		})
	}
}

// TestGateMetricsConfiguringATokenClosesIt: a configured token refuses a
// scrape that presents none, and says which scheme it wants.
func TestGateMetricsConfiguringATokenClosesIt(t *testing.T) {
	called := false
	h := gateMetrics("s3cr3t", false, func(http.ResponseWriter, *http.Request) { called = true })

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
			h := gateMetrics("s3cr3t", false, func(http.ResponseWriter, *http.Request) { called = true })

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
	h := gateMetrics("s3cr3t", false, func(http.ResponseWriter, *http.Request) { called = true })

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
			h := gateMetrics("s3cr3t", false, func(http.ResponseWriter, *http.Request) { called = true })

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

// TestWithOpenMetricsSetsServerField pins the Option's one job, and that the
// Server it has not touched starts closed.
func TestWithOpenMetricsSetsServerField(t *testing.T) {
	var s Server
	if s.metricsOpen {
		t.Fatal("a zero Server serves /metrics openly")
	}
	WithOpenMetrics()(&s, nil)
	if !s.metricsOpen {
		t.Fatal("WithOpenMetrics did not open /metrics")
	}
}

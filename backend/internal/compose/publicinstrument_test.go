// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/pkg/extension"
)

func edgeUnit(name string) extension.Extension {
	return extension.Extension{Name: extension.Name(name), Instrumentation: &extension.Instrumentation{
		Edge: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			_, _ = io.WriteString(w, name+" "+r.Method+" "+r.URL.Path+" "+string(body))
		}),
	}}
}

func edgeUnits() []extension.Extension {
	return []extension.Extension{edgeUnit("alpha"), edgeUnit("alpha-two")}
}

func serveEdge(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	limiter := ratelimit.New("public-instrument/per-ip/test", ratelimit.FailClosed, 1000, time.Minute)
	return serveEdgeWith(limiter, next, method, path, body)
}

func serveEdgeWith(limiter *ratelimit.Limiter, next http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	publicInstrument(edgeUnits(), limiter)(next).ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestPublicInstrumentHandsTheUnitItsPathWithThePrefixStripped(t *testing.T) {
	rec := serveEdge(t, "POST", "/v1/public/instrument/alpha/tunnel", "env")
	if rec.Code != http.StatusOK || rec.Body.String() != "alpha POST /tunnel env" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestPublicInstrumentPassesOtherPathsOn(t *testing.T) {
	if rec := serveEdge(t, "GET", "/v1/contacts", ""); rec.Code != http.StatusTeapot {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestPublicInstrumentAnswersNotFoundForAUnitWithNoEdge(t *testing.T) {
	if rec := serveEdge(t, "GET", "/v1/public/instrument/beta/config", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestPublicInstrumentRefusesOtherMethods(t *testing.T) {
	rec := serveEdge(t, "DELETE", "/v1/public/instrument/alpha/config", "")
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("code=%d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestPublicInstrumentKeepsAUnitApartFromOneWhoseNameItPrefixes(t *testing.T) {
	rec := serveEdge(t, "GET", "/v1/public/instrument/alpha-two/config", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "alpha-two GET /config " {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestPublicInstrumentAnswersNotFoundForTheBareUnitPath(t *testing.T) {
	if rec := serveEdge(t, "GET", "/v1/public/instrument/alpha", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestPublicInstrumentRefusesAClientPastItsBudget(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	limiter := ratelimit.New("public-instrument/per-ip/budget", ratelimit.FailClosed, 1, time.Minute)
	if rec := serveEdgeWith(limiter, next, "GET", "/v1/public/instrument/alpha/a", ""); rec.Code != http.StatusOK {
		t.Fatalf("first request code = %d", rec.Code)
	}
	if rec := serveEdgeWith(limiter, next, "GET", "/v1/public/instrument/alpha/a", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request code = %d, want 429", rec.Code)
	}
}

func TestPublicInstrumentCapsTheBodyAtOneMebibyte(t *testing.T) {
	rec := serveEdge(t, "POST", "/v1/public/instrument/alpha/tunnel", strings.Repeat("x", instrumentEdgeBodyLimit+1))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("code = %d", rec.Code)
	}
}

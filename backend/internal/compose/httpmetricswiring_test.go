// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// THE WIRING IS THE PART THAT CAN SILENTLY NOT WORK.
//
// httpmetrics_test.go proves the store's arithmetic against a fixed route. What
// it cannot prove is that the route reaching it is chi's TEMPLATE rather than
// the request path — that depends on the generated server applying its
// Middlewares after the match, which is a property of code this repo does not
// hand-write. If that ever stops holding, the metric keeps working and starts
// growing a series per id, which is the failure nobody notices until the
// scrape is expensive.
func TestChiRoutePatternReadsTheTemplateNotThePath(t *testing.T) {
	t.Parallel()
	var seen string
	router := chi.NewRouter()
	// Registered the way the generated server registers: a path parameter in
	// the pattern, so template and path necessarily differ.
	router.Get("/v1/deals/{dealId}", func(_ http.ResponseWriter, r *http.Request) {
		seen = chiRoutePattern(r)
	})
	router.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/v1/deals/9f3cbe20-0000-4000-8000-000000000000", nil))

	if want := "/v1/deals/{dealId}"; seen != want {
		t.Fatalf("chiRoutePattern = %q, want the template %q", seen, want)
	}
	if strings.Contains(seen, "9f3cbe20") {
		t.Fatal("the deal id reached the route label; this grows a series per record")
	}
}

// A request chi matched nothing for must yield "", so HTTPMetrics folds it into
// its single unmatched bucket rather than being handed a path.
func TestChiRoutePatternIsEmptyWhenNothingMatched(t *testing.T) {
	t.Parallel()
	var seen string
	captured := false
	router := chi.NewRouter()
	router.NotFound(func(_ http.ResponseWriter, r *http.Request) {
		seen = chiRoutePattern(r)
		captured = true
	})
	router.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/v1/no/such/route", nil))

	if !captured {
		t.Fatal("the NotFound handler never ran")
	}
	if seen != "" {
		t.Errorf("chiRoutePattern = %q for an unmatched request, want the empty string", seen)
	}
}

// A request carrying NO chi context at all — the shape every non-/v1 edge on
// the operational mux has — must not panic. chiRoutePattern is exported to a
// middleware that could be mounted anywhere.
func TestChiRoutePatternSurvivesARequestWithNoRouteContext(t *testing.T) {
	t.Parallel()
	if got := chiRoutePattern(httptest.NewRequest(http.MethodGet, "/healthz", nil)); got != "" {
		t.Errorf("chiRoutePattern = %q with no chi context, want the empty string", got)
	}
}

// The Server hands ONE store to both the middleware and the exposition.
// contractAPI and operationalMux each take the Server by value, so a
// struct-valued store would give them separate copies: requests counted into
// one, the scrape reading the other, and both looking perfectly healthy.
func TestTheMetricsStoreIsSharedByValueCopiesOfTheServer(t *testing.T) {
	t.Parallel()
	srv := Server{httpMetrics: newHTTPMetricsForTest()}
	copied := srv

	h := copied.httpMetrics.Measure(func(*http.Request) string { return "/v1/x" })(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/x", nil))

	var b strings.Builder
	srv.writeMetricsSections(&b)
	if want := `margince_http_requests_total{route="/v1/x",method="GET",status="200"} 1`; !strings.Contains(b.String(), want) {
		t.Errorf("the original Server's exposition did not see the copy's request\nwant %q\n--- got ---\n%s", want, b.String())
	}
}

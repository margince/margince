// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// routeOf returns a fixed route, standing in for the router's own pattern.
func routeOf(route string) func(*http.Request) string {
	return func(*http.Request) string { return route }
}

func serve(t *testing.T, m *HTTPMetrics, route, method string, status int, h http.HandlerFunc) {
	t.Helper()
	if h == nil {
		h = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }
	}
	mw := m.Measure(routeOf(route))(h)
	mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/v1/anything", nil))
}

func rendered(m *HTTPMetrics) string {
	var b strings.Builder
	m.Write(&b)
	return b.String()
}

// TestRequestsAreCountedByRouteMethodAndStatus is the counter's whole job. The
// three labels are the question an operator asks -- which route, called how,
// answering what -- and a family missing any one of them cannot answer it.
func TestRequestsAreCountedByRouteMethodAndStatus(t *testing.T) {
	m := NewHTTPMetrics()
	serve(t, m, "/v1/companies", http.MethodGet, 200, nil)
	serve(t, m, "/v1/companies", http.MethodGet, 200, nil)
	serve(t, m, "/v1/companies", http.MethodPost, 201, nil)
	serve(t, m, "/v1/companies/{id}", http.MethodGet, 404, nil)

	got := rendered(m)
	for _, want := range []string{
		`margince_http_requests_total{route="/v1/companies",method="GET",status="200"} 2`,
		`margince_http_requests_total{route="/v1/companies",method="POST",status="201"} 1`,
		`margince_http_requests_total{route="/v1/companies/{id}",method="GET",status="404"} 1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\n--- got ---\n%s", want, got)
		}
	}
}

// TestAStatusNeverWrittenIsCountedAs200 pins the net/http default: a handler
// that returns without calling WriteHeader has answered 200, and counting it
// as 0 would invent a status no client ever saw.
func TestAStatusNeverWrittenIsCountedAs200(t *testing.T) {
	m := NewHTTPMetrics()
	serve(t, m, "/v1/ping", http.MethodGet, 0, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			t.Errorf("writing the body: %v", err)
		}
	})
	if want := `margince_http_requests_total{route="/v1/ping",method="GET",status="200"} 1`; !strings.Contains(rendered(m), want) {
		t.Errorf("exposition missing %q\n--- got ---\n%s", want, rendered(m))
	}
}

// TestTheHistogramIsCumulativeAndCarriesInf holds the one part of the text
// format that is easy to get subtly wrong: a Prometheus histogram bucket
// counts everything AT OR BELOW its bound, the +Inf bucket equals _count, and
// _sum carries the observed total. A non-cumulative rendering still parses and
// still draws a chart -- it just answers every quantile wrongly.
func TestTheHistogramIsCumulativeAndCarriesInf(t *testing.T) {
	m := NewHTTPMetrics()
	// Three observations, chosen to fall in three different buckets.
	m.observe("/v1/x", "GET", 200, 2*time.Millisecond)
	m.observe("/v1/x", "GET", 200, 40*time.Millisecond)
	m.observe("/v1/x", "GET", 200, 3*time.Second)

	got := rendered(m)
	// 0.005 holds the 2ms one; 0.05 holds it plus the 40ms one; +Inf holds all.
	for _, want := range []string{
		`margince_http_request_duration_seconds_bucket{route="/v1/x",method="GET",le="0.005"} 1`,
		`margince_http_request_duration_seconds_bucket{route="/v1/x",method="GET",le="0.05"} 2`,
		`margince_http_request_duration_seconds_bucket{route="/v1/x",method="GET",le="+Inf"} 3`,
		`margince_http_request_duration_seconds_count{route="/v1/x",method="GET"} 3`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\n--- got ---\n%s", want, got)
		}
	}
	if !strings.Contains(got, `margince_http_request_duration_seconds_sum{route="/v1/x",method="GET"} 3.042`) {
		t.Errorf("_sum is not the observed total (want 3.042)\n--- got ---\n%s", got)
	}
}

// TestBucketBoundsAscendAndEndAtInf: Prometheus requires ascending bounds, and
// a quantile read over unsorted buckets is silently wrong rather than refused.
func TestBucketBoundsAscendAndEndAtInf(t *testing.T) {
	m := NewHTTPMetrics()
	m.observe("/v1/x", "GET", 200, time.Millisecond)
	var lastSeen float64
	for _, line := range strings.Split(rendered(m), "\n") {
		if !strings.Contains(line, "_bucket{") {
			continue
		}
		_, after, found := strings.Cut(line, `le="`)
		if !found {
			t.Fatalf("a bucket line carries no le label: %q", line)
		}
		le, _, found := strings.Cut(after, `"`)
		if !found {
			t.Fatalf("a bucket line's le label is unterminated: %q", line)
		}
		if le == "+Inf" {
			lastSeen = -1 // reached the terminator
			continue
		}
		if lastSeen == -1 {
			t.Fatal("a finite bucket follows +Inf; the family is not ascending")
		}
		var v float64
		if _, err := fmt.Sscanf(le, "%g", &v); err != nil {
			t.Fatalf("bucket bound %q is not a number: %v", le, err)
		}
		if v <= lastSeen {
			t.Fatalf("bucket bound %g does not exceed the previous %g", v, lastSeen)
		}
		lastSeen = v
	}
	if lastSeen != -1 {
		t.Fatal("the histogram has no +Inf bucket, so _count has nothing to agree with")
	}
}

// TestInFlightRisesInsideTheHandlerAndFallsAfter is the gauge's only
// interesting property. Reading it from INSIDE the handler is the point: a
// gauge that is only ever observed after the fact reads zero forever and looks
// exactly like a working one.
func TestInFlightRisesInsideTheHandlerAndFallsAfter(t *testing.T) {
	m := NewHTTPMetrics()
	var inside string
	h := m.Measure(routeOf("/v1/slow"))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		inside = rendered(m)
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/slow", nil))

	if !strings.Contains(inside, "margince_http_requests_in_flight 1") {
		t.Errorf("in-flight did not read 1 during the request\n--- got ---\n%s", inside)
	}
	if !strings.Contains(rendered(m), "margince_http_requests_in_flight 0") {
		t.Errorf("in-flight did not return to 0 after the request\n--- got ---\n%s", rendered(m))
	}
}

// TestInFlightFallsWhenTheHandlerPanics: the decrement has to be deferred. A
// panicking handler that leaked the gauge would leave a permanently rising
// floor, and RecoverPanics upstream means the process survives to keep doing
// it.
func TestInFlightFallsWhenTheHandlerPanics(t *testing.T) {
	m := NewHTTPMetrics()
	h := m.Measure(routeOf("/v1/boom"))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("handler exploded")
	}))
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Error("the handler did not panic, so this proves nothing about the deferred decrement")
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/boom", nil))
	}()
	if !strings.Contains(rendered(m), "margince_http_requests_in_flight 0") {
		t.Errorf("a panicking handler leaked the in-flight gauge\n--- got ---\n%s", rendered(m))
	}
}

// TestAnUnmatchedRouteFoldsIntoOneSeries. The resolver returns "" when the
// router matched nothing, and the raw path must NOT be substituted: that is
// how an id, a token or a scanner's garbage becomes a permanent label value.
func TestAnUnmatchedRouteFoldsIntoOneSeries(t *testing.T) {
	m := NewHTTPMetrics()
	for _, p := range []string{"/nope", "/also-nope", "/v1/../etc/passwd"} {
		mw := m.Measure(func(*http.Request) string { return "" })(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }))
		mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}
	got := rendered(m)
	if want := `margince_http_requests_total{route="unmatched",method="GET",status="404"} 3`; !strings.Contains(got, want) {
		t.Errorf("three unmatched requests did not fold into one series\n--- got ---\n%s", got)
	}
	for _, leaked := range []string{"/nope", "/also-nope", "passwd"} {
		if strings.Contains(got, leaked) {
			t.Errorf("the request path %q reached a label value", leaked)
		}
	}
}

// TestCardinalityIsCapped is the safety valve. chi's patterns are bounded by
// the generated router, so this cannot fire for /v1 -- but Measure takes the
// route from a caller-supplied resolver, and an unbounded one would grow this
// map until the process died. Folding into one overflow series keeps the
// exposition honest about the requests it could not name.
func TestCardinalityIsCapped(t *testing.T) {
	m := NewHTTPMetrics()
	for i := range maxRouteSeries + 50 {
		m.observe(fmt.Sprintf("/v1/generated/%d", i), "GET", 200, time.Millisecond)
	}
	got := rendered(m)
	if !strings.Contains(got, routeOverflowLabel) {
		t.Errorf("no overflow series once past the cap\n--- got (truncated) ---\n%s", got[:min(len(got), 400)])
	}
	if n := strings.Count(got, "_count{"); n > maxRouteSeries+1 {
		t.Errorf("latency families = %d, want at most the cap plus the overflow series", n)
	}
}

// TestConcurrentRequestsDoNotRace: -race would report a torn map write, and a
// metrics endpoint is scraped WHILE requests are served, so Write races the
// middleware by construction rather than by accident.
func TestConcurrentRequestsDoNotRace(t *testing.T) {
	m := NewHTTPMetrics()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func() { defer wg.Done(); m.observe("/v1/x", "GET", 200, time.Duration(i)*time.Millisecond) }()
		go func() {
			defer wg.Done()
			if rendered(m) == "" {
				t.Error("a scrape racing the middleware rendered nothing")
			}
		}()
	}
	wg.Wait()
	if want := `margince_http_request_duration_seconds_count{route="/v1/x",method="GET"} 50`; !strings.Contains(rendered(m), want) {
		t.Errorf("lost an observation under concurrency; want %q", want)
	}
}

// TestNilMetricsIsInert so a role that wires none is not a nil dereference on
// its first request -- the same "declared or absent" posture the rest of the
// exposition takes.
func TestNilMetricsIsInert(t *testing.T) {
	var m *HTTPMetrics
	called := false
	h := m.Measure(routeOf("/v1/x"))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	if !called {
		t.Fatal("a nil HTTPMetrics dropped the request instead of passing it through")
	}
	var b strings.Builder
	m.Write(&b)
	if b.Len() != 0 {
		t.Errorf("a nil HTTPMetrics wrote an exposition: %q", b.String())
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

// The HTTP layer's own exposition: how many requests each route answered, how
// long it took, and how many are in flight right now.
//
// Everything here was already measured and thrown away. AccessLog has always
// timed the request and captured the status, then written both to a log line --
// so "how slow is this route" was answerable only by parsing logs, one
// installation at a time, and "what is the p95" was not answerable at all.
// This turns the same two numbers into series.
//
// Hand-rolled, like every other family in this process: there is no Prometheus
// client library in this module, and adding one for three metric families would
// bring a registry, a collector interface and a second exposition writer beside
// the one httpserver.Metrics already owns.
//
// THE LABEL IS THE ROUTE PATTERN, NEVER THE PATH. `/v1/companies/{id}` is one
// series; `/v1/companies/9f3c…` is one series per company that has ever been
// read, forever, in a process that also cannot forget them. AccessLog logs the
// path deliberately -- a log line answers "what did clients ask" -- and that is
// exactly the reading a metric label must not take. Measure therefore asks its
// caller's router for the matched pattern and folds a miss into one bucket.

import (
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// latencyBounds are the histogram's upper bounds in seconds, ascending.
//
// Chosen for THIS surface rather than copied: the fast half (5ms-100ms) is
// where a CRUD read on a warm pool lands, so the buckets an operator watches
// for regression are dense there. The tail runs to 10s because that is past
// every ceiling the process imposes on itself -- so a request in the +Inf
// bucket is not "slow", it is a request something failed to bound.
var latencyBounds = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

const (
	// maxRouteSeries caps how many distinct route+method pairs the histogram
	// will hold. chi's patterns come from the generated router, so the real
	// number is fixed at compile time and this can never fire for /v1 -- but
	// Measure takes the route from a caller-supplied resolver, and one that
	// returned anything request-derived would grow these maps until the process
	// died. A metric must not be able to do that.
	maxRouteSeries = 500
	// routeOverflowLabel is where series past the cap are counted. They are
	// counted rather than dropped: an exposition that silently stopped
	// measuring would read as a quiet surface.
	routeOverflowLabel = "__over_cardinality_limit__"
	// unmatchedRoute is where a request the router matched no pattern for is
	// counted -- one series for every 404-by-path, however many distinct paths
	// they arrived on.
	unmatchedRoute = "unmatched"
)

// HTTPMetrics accumulates the request families. Safe for concurrent use: the
// middleware writes it on every request while /metrics reads it on a scrape,
// so the two race by construction rather than by accident.
type HTTPMetrics struct {
	mu       sync.Mutex
	requests map[requestKey]uint64
	latency  map[routeKey]*latencyHistogram
	inFlight atomic.Int64
}

type requestKey struct {
	route  string
	method string
	status int
}

type routeKey struct {
	route  string
	method string
}

type latencyHistogram struct {
	counts []uint64 // one per latencyBounds entry, plus the +Inf terminator
	sum    float64
	count  uint64
}

// NewHTTPMetrics returns an empty, ready store.
func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{
		requests: make(map[requestKey]uint64),
		latency:  make(map[routeKey]*latencyHistogram),
	}
}

// Measure wraps a handler so its requests are counted and timed.
//
// routeOf reads the matched pattern off the request, which only the router that
// matched it can answer -- chi keeps it on the request context, and net/http
// keeps it on the Request. It is a parameter rather than a hard-coded reading
// so this package stays router-agnostic, and so a caller that has no bounded
// answer is forced to say so by returning "" instead of quietly passing a path.
//
// A nil receiver returns the handler unwrapped, so a role that wires no metrics
// is not a nil dereference on its first request.
func (m *HTTPMetrics) Measure(routeOf func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if m == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m.inFlight.Add(1)
			// Deferred, not decremented after ServeHTTP: a panicking handler
			// would otherwise leak the gauge, and RecoverPanics upstream means
			// the process survives to keep leaking it until the floor is the
			// only thing the panel shows.
			defer m.inFlight.Add(-1)

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				// Also deferred, so a panic is still MEASURED. The status it
				// records is whatever reached the client, which for a panic
				// mid-handler is the 200 net/http had already sent or the 500
				// RecoverPanics writes -- either way a real answer, and a
				// request that vanished from the counters would be worse.
				m.observe(routeOf(r), r.Method, rec.status, time.Since(start))
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// observe records one finished request. Separate from Measure so the tests can
// state the histogram's arithmetic directly rather than through a server.
func (m *HTTPMetrics) observe(route, method string, status int, took time.Duration) {
	if m == nil {
		return
	}
	if route == "" {
		route = unmatchedRoute
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	rk := routeKey{route: route, method: method}
	hist, known := m.latency[rk]
	if !known && len(m.latency) >= maxRouteSeries {
		rk = routeKey{route: routeOverflowLabel, method: method}
		route = routeOverflowLabel
		hist, known = m.latency[rk]
	}
	if !known {
		hist = &latencyHistogram{counts: make([]uint64, len(latencyBounds)+1)}
		m.latency[rk] = hist
	}

	m.requests[requestKey{route: route, method: method, status: status}]++

	seconds := took.Seconds()
	hist.sum += seconds
	hist.count++
	// Cumulative at WRITE time rather than at read time: a bucket counts every
	// observation at or below its bound, so incrementing each bound the sample
	// clears keeps the read a straight walk. sort.SearchFloat64s finds the
	// first bound the sample does NOT exceed.
	for i := sort.SearchFloat64s(latencyBounds, seconds); i < len(hist.counts); i++ {
		hist.counts[i]++
	}
}

// Write renders the three families in Prometheus text format. Called by the
// composition layer's metrics section; a nil receiver writes nothing, the same
// "declared or absent" posture every other section here takes.
func (m *HTTPMetrics) Write(w io.Writer) {
	if m == nil {
		return
	}
	m.mu.Lock()
	requests := make(map[requestKey]uint64, len(m.requests))
	for k, v := range m.requests {
		requests[k] = v
	}
	latency := make(map[routeKey]latencyHistogram, len(m.latency))
	for k, v := range m.latency {
		counts := make([]uint64, len(v.counts))
		copy(counts, v.counts)
		latency[k] = latencyHistogram{counts: counts, sum: v.sum, count: v.count}
	}
	inFlight := m.inFlight.Load()
	m.mu.Unlock()

	// Wrapped in the package's own writer rather than each line handling its
	// own error: exposition.go explains the posture, and it holds whether or
	// not the caller already handed one in.
	out := &exposition{w: w}
	writeRequestCounters(out, requests)
	writeLatencyHistogram(out, latency)
	out.printf("# HELP margince_http_requests_in_flight Requests being served right now.\n")
	out.printf("# TYPE margince_http_requests_in_flight gauge\n")
	out.printf("margince_http_requests_in_flight %d\n", inFlight)
}

func writeRequestCounters(out *exposition, requests map[requestKey]uint64) {
	out.printf("# HELP margince_http_requests_total Requests answered, by matched route pattern, method and status.\n")
	out.printf("# TYPE margince_http_requests_total counter\n")
	keys := make([]requestKey, 0, len(requests))
	for k := range requests {
		keys = append(keys, k)
	}
	// Stable output so a scrape diff and a test do not flap on map order.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})
	for _, k := range keys {
		out.printf("margince_http_requests_total{route=%q,method=%q,status=%q} %d\n",
			k.route, k.method, strconv.Itoa(k.status), requests[k])
	}
}

func writeLatencyHistogram(out *exposition, latency map[routeKey]latencyHistogram) {
	out.printf("# HELP margince_http_request_duration_seconds Request duration by matched route pattern and method.\n")
	out.printf("# TYPE margince_http_request_duration_seconds histogram\n")
	keys := make([]routeKey, 0, len(latency))
	for k := range latency {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		return keys[i].method < keys[j].method
	})
	const name = "margince_http_request_duration_seconds"
	for _, k := range keys {
		h := latency[k]
		for i, bound := range latencyBounds {
			out.printf("%s_bucket{route=%q,method=%q,le=%q} %d\n",
				name, k.route, k.method, strconv.FormatFloat(bound, 'g', -1, 64), h.counts[i])
		}
		// The +Inf bucket equals _count by definition, and a histogram without
		// it is not a histogram: every quantile read walks to the terminator.
		out.printf("%s_bucket{route=%q,method=%q,le=\"+Inf\"} %d\n", name, k.route, k.method, h.count)
		out.printf("%s_sum{route=%q,method=%q} %g\n", name, k.route, k.method, h.sum)
		out.printf("%s_count{route=%q,method=%q} %d\n", name, k.route, k.method, h.count)
	}
}

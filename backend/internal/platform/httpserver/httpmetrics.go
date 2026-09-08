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
	"fmt"
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
	// holds. chi's patterns come from the generated router, so the real number
	// is fixed at compile time and this cannot fire for /v1 -- but Measure takes
	// the route from a caller-supplied resolver, and one that returned anything
	// request-derived would grow this map without it.
	//
	// It is the SECOND of two bounds, not the only one: normalizeMethod closes
	// the method dimension, which this cap provably did not. See otherMethod.
	maxRouteSeries = 500
	// maxRequestSeries caps the counter map, which is keyed one dimension wider
	// than the histogram -- route, method AND status. Capped on its own count
	// rather than trusting the histogram's: the two maps do not grow in step, so
	// a bound asserted about one is not a bound on the other. That was the shape
	// of the original mistake and it should not be repeated by inheritance.
	maxRequestSeries = 5000
	// routeOverflowLabel is where series past the cap are counted. They are
	// counted rather than dropped: an exposition that silently stopped
	// measuring would read as a quiet surface.
	routeOverflowLabel = "__over_cardinality_limit__"
	// unmatchedRoute is where a request the router matched no pattern for is
	// counted -- one series for every 404-by-path, however many distinct paths
	// they arrived on.
	unmatchedRoute = "unmatched"
	// overflowStatus stands where a status would go on the overflow series. It
	// is not a status any client saw, and a number there would read as one --
	// the overflow bucket answers "how many requests could not be named", not
	// "how did they end".
	overflowStatus = -1
	// otherMethod is where a method outside RFC 9110's set is counted.
	//
	// This closes the method dimension, which the route cap alone did not.
	// r.Method is a token the CLIENT chooses -- net/http accepts any valid token
	// -- so `EVIL1`, `EVIL2`, ... arrive as distinct label values, and folding
	// the ROUTE into an overflow bucket does not help while the overflow key
	// still carries the method: each new method mints a new overflow series.
	// Measured before this existed: 5000 distinct methods against a 500-route
	// cap produced 5000 series in each map.
	//
	// NOT reachable through the /v1 mount, and the honest bound matters here.
	// Measure runs as chi OPERATION middleware, after the match, so the method
	// is one the generated router registered for that route -- an arbitrary
	// token is answered 405 by chi and never reaches this. So this is a hole in
	// the exported CONTRACT, not a live denial of service: Measure takes its
	// route from a caller-supplied resolver and may be mounted anywhere, and a
	// metric should not depend on every future caller having thought about it.
	otherMethod = "other"
)

// knownMethods is the closed set a method label may take: RFC 9110's nine, and
// nothing else. Bounding at the DOOR rather than by a cap downstream, because a
// cap answers "how many" while this answers "which", and only the second makes
// the label meaningful -- a series named for a method nobody implements tells an
// operator nothing they can act on.
var knownMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodHead: {}, http.MethodPost: {}, http.MethodPut: {},
	http.MethodPatch: {}, http.MethodDelete: {}, http.MethodConnect: {},
	http.MethodOptions: {}, http.MethodTrace: {},
}

// normalizeMethod folds anything outside that set into one series. Counted
// rather than dropped: a flood of nonsense methods is worth SEEING, and it is
// the shape a scanner makes.
func normalizeMethod(method string) string {
	if _, known := knownMethods[method]; known {
		return method
	}
	return otherMethod
}

// HTTPMetrics accumulates the request families. Safe for concurrent use: the
// middleware writes it on every request while /metrics reads it on a scrape,
// so the two race by construction rather than by accident.
type HTTPMetrics struct {
	mu       sync.Mutex
	requests map[requestKey]uint64
	latency  map[routeKey]*Histogram
	inFlight atomic.Int64

	// The overflow bucket is ONE counter and ONE histogram, held OUTSIDE the
	// maps rather than as a key inside them. That is the whole lesson of this
	// file: two earlier attempts folded an over-cap request into a synthetic
	// KEY, and both were defeated within minutes of being measured, because the
	// key still carried a dimension the caller controls -- first the method,
	// then the status. A bucket that cannot grow has to be a field, not a key.
	overflowRequests uint64
	overflowLatency  *Histogram
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

// time: a bucket counts every observation at or below its bound, so
// incrementing each bound the sample clears keeps the read a straight walk.

// NewHTTPMetrics returns an empty, ready store.
func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{
		requests:        make(map[requestKey]uint64),
		latency:         make(map[routeKey]*Histogram),
		overflowLatency: NewHistogram(latencyBounds),
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
			returned := false
			defer func() {
				// Deferred so a panicked request is still measured -- one that
				// vanished from the counters would be worse than a mislabelled
				// one.
				//
				// A PANIC IS RECORDED AS 500, AND NOT FROM rec.status. This
				// wrapper sits INSIDE RecoverPanics (compose/server.go), so
				// the 500 the client receives is written by that outer
				// recover, on the original ResponseWriter, after this defer
				// has already run during unwinding. rec.status is therefore
				// still its 200 default, and recording it would show a route
				// that panics on every request as a 100% success rate -- in
				// the one place an operator would look to see otherwise.
				//
				// `returned` is the only reliable signal here: Go offers no
				// way to ask whether a panic is in flight without recovering
				// it, and recovering is RecoverPanics' job, not a metric's.
				//
				// A handler that wrote a header and THEN panicked keeps what
				// it wrote: the client did receive that status, and the broken
				// body after it is not this counter's story to tell.
				status := rec.status
				if !returned && !rec.wrote {
					status = http.StatusInternalServerError
				}
				m.observe(routeOf(r), r.Method, status, time.Since(start))
			}()
			next.ServeHTTP(rec, r)
			returned = true
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
	method = normalizeMethod(method)
	seconds := took.Seconds()

	m.mu.Lock()
	defer m.mu.Unlock()

	rk := routeKey{route: route, method: method}
	hist, named := m.latency[rk]
	req := requestKey{route: route, method: method, status: status}
	_, counted := m.requests[req]

	// Either map needing a NEW key while at its cap sends the whole
	// observation to the fixed overflow pair. Both are checked, because the two
	// maps are keyed to different widths and do not grow in step: a bound
	// asserted about one is not a bound on the other.
	if (!named && len(m.latency) >= maxRouteSeries) || (!counted && len(m.requests) >= maxRequestSeries) {
		m.overflowRequests++
		m.overflowLatency.Observe(seconds)
		return
	}

	if !named {
		hist = NewHistogram(latencyBounds)
		m.latency[rk] = hist
	}
	m.requests[req]++
	hist.Observe(seconds)
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
	latency := make(map[routeKey]Histogram, len(m.latency))
	for k, v := range m.latency {
		latency[k] = v.Snapshot()
	}
	overflowRequests := m.overflowRequests
	overflowLatency := m.overflowLatency.Snapshot()
	inFlight := m.inFlight.Load()
	m.mu.Unlock()

	// The overflow pair is rendered as ONE series each, with every dimension it
	// could not name spelled `other`. Folded in here rather than carried in the
	// maps, which is what keeps it incapable of growing -- see HTTPMetrics.
	if overflowRequests > 0 {
		requests[requestKey{route: routeOverflowLabel, method: otherMethod, status: overflowStatus}] = overflowRequests
	}
	if overflowLatency.count > 0 {
		latency[routeKey{route: routeOverflowLabel, method: otherMethod}] = overflowLatency
	}

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
		status := strconv.Itoa(k.status)
		if k.status == overflowStatus {
			status = "other"
		}
		out.printf("margince_http_requests_total{route=%s,method=%s,status=%s} %d\n",
			Label(k.route), Label(k.method), Label(status), requests[k])
	}
}

func writeLatencyHistogram(out *exposition, latency map[routeKey]Histogram) {
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
		h.WriteSeries(out, name, fmt.Sprintf("route=%s,method=%s", Label(k.route), Label(k.method)))
	}
}

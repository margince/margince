// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// quietLog keeps a listener's own log lines out of the test output; what is
// under test is what it SERVES, not what it says about itself.
func quietLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// startForTest brings the listener up on a port the KERNEL chooses and answers
// its base URL, read back from the listener itself. Asking for :0 and reading
// Addr leaves no window: reserving a port, closing it and re-binding would let
// another process take it in between, which reads as a flake rather than as
// the race it is.
func startForTest(t *testing.T) string {
	t.Helper()

	// A nil pool and bus are enough for the two endpoints asserted below:
	// /healthz reads nothing, and the metrics handler omits the pool section
	// when it was handed none. /readyz is the one endpoint that would call
	// into them, and it is the integration lane's to prove against real
	// dependencies rather than against a stub that would only restate itself.
	observe, err := startObserveListener(t.Context(), workerConfig{observeAddr: "127.0.0.1:0"},
		nil, nil, &bootGate{}, quietLog())
	if err != nil {
		t.Fatalf("startObserveListener: %v", err)
	}
	t.Cleanup(observe.Stop)
	if observe.Addr == "" {
		t.Fatal("a configured address bound nothing")
	}
	return "http://" + observe.Addr
}

// get fetches one path and answers its status and body.
func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building a request for %s: %v", url, err)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the response body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response to %s: %v", url, err)
	}
	return resp.StatusCode, string(body)
}

// TestAnUnconfiguredObserveAddressBindsNothing — off is the default, and off
// has to mean no listener rather than one bound somewhere the operator did not
// choose. The bound address is what proves it: a listener opened anywhere would
// report the address it took, so the empty string is the absence itself rather
// than an absence inferred from a call that did not error.
//
// The stop function must still be callable, because the caller defers it
// unconditionally and a nil there would panic every worker that never enabled
// the surface.
func TestAnUnconfiguredObserveAddressBindsNothing(t *testing.T) {
	observe, err := startObserveListener(t.Context(), workerConfig{}, nil, nil, &bootGate{}, quietLog())
	if err != nil {
		t.Fatalf("an empty --observe-addr must be a legitimate configuration, got: %v", err)
	}
	if observe.Addr != "" {
		t.Errorf("an empty --observe-addr bound %s; off must mean no listener at all", observe.Addr)
	}
	if observe.Stop == nil {
		t.Fatal("no stop function returned; run() defers it unconditionally and would panic")
	}
	observe.Stop()
}

// TestAnUnusableObserveAddressFailsTheBoot — the listen happens during boot
// rather than inside the serving goroutine, so a busy port or a malformed
// address stops the process with a message naming it. Logged instead, the
// worker would carry on looking healthy while publishing nothing.
func TestAnUnusableObserveAddressFailsTheBoot(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("holding a port: %v", err)
	}
	t.Cleanup(func() {
		if err := held.Close(); err != nil {
			t.Errorf("releasing the held port: %v", err)
		}
	})

	observe, err := startObserveListener(t.Context(),
		workerConfig{observeAddr: held.Addr().String()}, nil, nil, &bootGate{}, quietLog())
	if err == nil {
		observe.Stop()
		t.Fatal("binding a port already in use succeeded; a worker that could not serve its probes must fail its boot")
	}
	if !strings.Contains(err.Error(), held.Addr().String()) {
		t.Errorf("the error does not name the address that failed: %v", err)
	}
}

// TestTheWorkerServesItsLivenessProbe — the point of the listener: an
// orchestrator can ask this process whether it is alive at all.
func TestTheWorkerServesItsLivenessProbe(t *testing.T) {
	status, body := get(t, startForTest(t)+"/healthz")
	if status != http.StatusOK {
		t.Errorf("GET /healthz = %d, want 200", status)
	}
	if body != "ok" {
		t.Errorf("GET /healthz body = %q, want %q", body, "ok")
	}
}

// TestTheWorkerMetricsAreProcessLocalAndReServeNoFleetGauge is the boundary
// the whole listener rests on. What this role adds is per-REPLICA visibility:
// the fleet-wide readings stay a single copy on the api, because two roles
// answering one number is a worse operator surface than one gap. A section
// added here that reads a shared table would silently recreate that.
func TestTheWorkerMetricsAreProcessLocalAndReServeNoFleetGauge(t *testing.T) {
	status, body := get(t, startForTest(t)+"/metrics")
	if status != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", status)
	}

	for _, family := range []string{
		// The runtime collectors, which say which PROCESS is wedged.
		"go_goroutines",
		"go_memstats_heap_alloc_bytes",
		"go_gc_duration_seconds",
		"process_cpu_seconds_total",
		"margince_relay_published_total",
		// The AI counters. This role resolves a model path and runs the briefs,
		// the sweeps and the embedding lane through it, and every Router in the
		// binary increments one process-wide collector — so a worker that
		// renders none of it counts its own calls and tells nobody.
		"margince_ai_calls_total",
		"margince_ai_call_duration_seconds",
		"margince_ai_tokens_total",
	} {
		if !strings.Contains(body, "# TYPE "+family+" ") {
			t.Errorf("the worker publishes no %s; it is process-local and served nowhere else\ngot:\n%s", family, body)
		}
	}

	// Each of these is a projection of a table every role shares, so the api's
	// copy is the fleet's one reading of it.
	for _, family := range []string{
		"margince_job_queue_depth",
		"margince_job_declared_info",
		"margince_sweep_workspaces",
		"margince_sweep_units",
		"margince_outbox_unpublished",
	} {
		if strings.Contains(body, family) {
			t.Errorf("the worker re-serves %s, which is a fleet-wide reading the api already answers; "+
				"two sources for one number is worse than one\ngot:\n%s", family, body)
		}
	}
}

// TestTheWorkerSurfaceSetsTheSameBrowserFacingHeadersAsTheApi — this port
// serves no HTML and is not meant for a browser, which is exactly why the
// headers are worth setting rather than skipping: an operator surface reachable
// from a browser tab must not depend on nobody opening one. The api's chassis
// sets them on everything it answers; a second HTTP surface in the same product
// answering without them is a difference nobody chose.
func TestTheWorkerSurfaceSetsTheSameBrowserFacingHeadersAsTheApi(t *testing.T) {
	base := startForTest(t)
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+path, nil)
		if err != nil {
			t.Fatalf("building a request for %s: %v", path, err)
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the response body: %v", err)
		}
		for header, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		} {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("GET %s: %s = %q, want %q", path, header, got, want)
			}
		}
	}
}

// TestTheWorkerMetricsOmitThePoolSectionRatherThanZeroingIt — a role wired
// without a pool must publish no pool gauges at all. A zero-valued section
// reads exactly like an idle pool, which is the reading an operator would act
// on; the same "declared or absent" posture every other section takes.
func TestTheWorkerMetricsOmitThePoolSectionRatherThanZeroingIt(t *testing.T) {
	_, body := get(t, startForTest(t)+"/metrics")
	if strings.Contains(body, "margince_pgxpool_conns") {
		t.Errorf("a nil pool produced pool gauges, which read as an idle pool rather than as no pool\ngot:\n%s", body)
	}
}

// TestTheWorkerServesItsOwnAICounters — an AI call is routed by ONE process,
// so its counter is a property of that process and no other role can report
// it. With this section absent the api's exposition is the only one, and a
// per-tier error rate computed over it describes api traffic while appearing
// to describe the installation's: the lanes in this binary do the bulk of the
// routing, so the denominator is short by most of its calls.
func TestTheWorkerServesItsOwnAICounters(t *testing.T) {
	_, body := get(t, startForTest(t)+"/metrics")

	if !strings.Contains(body, "# TYPE margince_ai_calls_total counter") {
		t.Errorf("the worker serves no AI section, so every call its lanes route is missing from the "+
			"AI panels\ngot:\n%s", body)
	}
	// Additive, not a replacement: the process section this listener exists
	// for still has to be there beside it.
	if !strings.Contains(body, "go_goroutines") {
		t.Errorf("the process section went missing beside the AI section\ngot:\n%s", body)
	}
}

// The invariant behind "declared or absent", stated as what actually protects a
// rate: a process that has routed nothing must serve no SAMPLE, or a per-tier
// error rate computed over it reads as a healthy tier rather than as no tier.
//
// The family HEADER may be present — it is, from boot, because the collector is
// process-wide and needs no model path to exist. A header carries no sample, so
// rate() and increase() see nothing either way; it is the sample that would
// lie, and there is none until a call is made.
func TestTheWorkerServesNoAISampleBeforeAnyCallIsRouted(t *testing.T) {
	_, body := get(t, startForTest(t)+"/metrics")

	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "margince_ai_") {
			t.Errorf("a worker that has routed no call served an AI sample, which a rate reads as a "+
				"healthy tier: %s", line)
		}
	}
}

// Off is the default for this listener, and the AI section is wired at
// construction — so an off surface must still be a legitimate configuration
// that renders nothing rather than a boot that fails or a nil that panics.
func TestTheAISectionIsHarmlessWhenTheSurfaceIsOff(t *testing.T) {
	observe, err := startObserveListener(t.Context(), workerConfig{}, nil, nil, &bootGate{}, quietLog())
	if err != nil {
		t.Fatalf("an empty --observe-addr must be a legitimate configuration, got: %v", err)
	}
	t.Cleanup(observe.Stop)

	if observe.Addr != "" {
		t.Errorf("an off surface bound %q; nothing should be listening", observe.Addr)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func scrape(t *testing.T, in MetricsInput) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Metrics(in)(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

// The runtime section's whole reason for existing: these are read out of THIS
// process, so they differ per target and say which replica stopped working.
//
// The CPU, RSS and start-time families are what the four hand-read gauges this
// replaced could never reach — cAdvisor answers CPU and memory per CONTAINER,
// which cannot separate two processes in one pod, and knows nothing of
// goroutines, threads, GC pause duration or descriptors.
func TestTheRuntimeSectionCarriesWhatOnlyTheProcessKnows(t *testing.T) {
	body := scrape(t, MetricsInput{})

	for _, family := range []string{
		"go_goroutines",
		"go_threads",
		"go_memstats_heap_alloc_bytes",
		"go_memstats_heap_sys_bytes",
		"go_gc_duration_seconds",
		"process_cpu_seconds_total",
		"process_resident_memory_bytes",
		"process_start_time_seconds",
	} {
		if !strings.Contains(body, "# TYPE "+family+" ") {
			t.Errorf("the runtime section published no %s:\n%s", family, body)
		}
	}
}

// The four hand-read gauges are retired, not merely superseded. Keeping them
// beside the collector's would be two spellings of one number AND a second
// stop-the-world read of runtime.MemStats on every scrape of every replica.
func TestTheHandRolledProcessGaugesAreGone(t *testing.T) {
	body := scrape(t, MetricsInput{})

	for _, retired := range []string{
		"margince_process_goroutines",
		"margince_process_heap_bytes",
		"margince_process_heap_sys_bytes",
		"margince_process_gc_cycles_total",
	} {
		if strings.Contains(body, retired) {
			t.Errorf("%s is still emitted; it now duplicates a go_* family and costs a second ReadMemStats", retired)
		}
	}
}

// Prometheus rejects the WHOLE scrape on a repeated family, so the two halves
// of this exposition — collector-gathered and hand-rolled — must not both
// claim a name. A single # TYPE per family is that invariant, asserted over
// the real handler rather than over either half alone.
func TestNoFamilyIsDeclaredTwiceAcrossTheTwoHalves(t *testing.T) {
	body := scrape(t, MetricsInput{
		Published: func() uint64 { return 0 },
		Backlog:   func(context.Context) (int64, error) { return 0, nil },
	})

	seen := map[string]int{}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "# TYPE ") {
			continue
		}
		if fields := strings.Fields(line); len(fields) >= 3 {
			seen[fields[2]]++
		}
	}
	if len(seen) == 0 {
		t.Fatal("no families were declared at all; this assertion would pass over an empty scrape")
	}
	for family, n := range seen {
		if n != 1 {
			t.Errorf("%s was declared %d times; a repeated family makes Prometheus reject the entire scrape", family, n)
		}
	}
}

// The exposition is one document in one format. promhttp would have
// content-negotiated the collector half from the request's Accept header —
// a Prometheus scraper asks for OpenMetrics first — and produced a response
// whose two halves parsed as neither.
func TestTheCollectorHalfIsPinnedToTheTextFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Accept", "application/openmetrics-text;version=1.0.0,text/plain;version=0.0.4;q=0.5")
	Metrics(MetricsInput{})(rec, req)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain — the hand-rolled half can only be text", got)
	}
	// OpenMetrics terminates the document with this line and the text format
	// never does; its presence would mean one half negotiated away.
	if strings.Contains(rec.Body.String(), "# EOF") {
		t.Errorf("the collector half was encoded as OpenMetrics beside a text-format half:\n%s", rec.Body.String())
	}
}

// A scrape whose reader hung up must not be charged for gathering, and must not
// have half a runtime section pushed at it.
//
// The assertion is on the BYTES, not on out.gone(): the probe write is what
// discovers the writer is dead, so gone() is already true before
// writeRuntimeMetrics is called and a guard on it could never fail whatever the
// function did. What can fail is the section writing anyway.
func TestTheRuntimeSectionWritesNothingForAScrapeThatHasAlreadyGone(t *testing.T) {
	rec := httptest.NewRecorder()
	out := &exposition{w: &hangUp{ResponseWriter: rec, accepts: 0}}
	out.printf("probe\n")
	if !out.gone() {
		t.Fatal("the probe did not exhaust the writer, so this test would prove nothing")
	}
	delivered := rec.Body.Len()

	writeRuntimeMetrics(context.Background(), out)

	if rec.Body.Len() != delivered {
		t.Errorf("the runtime section wrote %d further bytes into a socket that is gone:\n%s",
			rec.Body.Len()-delivered, rec.Body.String()[delivered:])
	}
}

// The registry is this package's own. Registering the default collectors on
// prometheus.DefaultRegisterer would make a SECOND handler construction panic
// on the duplicate — two roles built in one test binary, say — so this asserts
// that a second handler both exists and serves the runtime section, not merely
// that building it survived.
func TestASecondHandlerServesTheRuntimeSectionToo(t *testing.T) {
	first := scrape(t, MetricsInput{})
	second := scrape(t, MetricsInput{})

	for _, body := range []string{first, second} {
		if !strings.Contains(body, "# TYPE go_goroutines ") {
			t.Errorf("a handler served no runtime section:\n%s", body)
		}
	}
}

// go_goroutines is a live reading, not a constant baked at registration.
func TestTheGoroutineGaugeTracksTheRunningProcess(t *testing.T) {
	before := countGoroutines(t)

	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(goroutineProbeCount)
	for range goroutineProbeCount {
		go func() {
			started.Done()
			<-release
		}()
	}
	started.Wait()
	after := countGoroutines(t)
	close(release)

	// Half the probe count, not all of it: goroutines from earlier tests in this
	// binary retire on their own schedule and move the baseline down between the
	// two scrapes. The question is whether the gauge MOVES with the runtime, and
	// a tolerance says so without inviting a future tightening into a flake.
	if after-before < goroutineProbeCount/2 {
		t.Errorf("go_goroutines read %d then %d while %d goroutines were parked; the gauge is not live",
			before, after, goroutineProbeCount)
	}
}

// goroutineProbeCount is large enough that the reading cannot rise by chance
// from unrelated runtime activity between the two scrapes.
const goroutineProbeCount = 50

func countGoroutines(t *testing.T) int {
	t.Helper()
	for _, line := range strings.Split(scrape(t, MetricsInput{}), "\n") {
		if after, found := strings.CutPrefix(line, "go_goroutines "); found {
			n, err := strconv.Atoi(strings.TrimSpace(after))
			if err != nil {
				t.Fatalf("go_goroutines carried %q, which is not a number", after)
			}
			return n
		}
	}
	t.Fatalf("go_goroutines was absent from the scrape, so this test proves nothing (runtime reports %d)", runtime.NumGoroutine())
	return 0
}

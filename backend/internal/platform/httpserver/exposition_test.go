// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

// One posture for a refused write, held from both ends.
//
// The job section already stopped the handler; every other section discarded
// its write errors, so a scrape whose reader hung up during the FIRST section
// went on to query the outbox, the job table and the mirror for a socket that
// was not there — and answered 200 with a body nobody received. The distinction
// was invisible in the source, which is how the runtime section inherited the
// silent half without anybody choosing it.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// hangUp is a ResponseWriter whose body refuses after the first write, which is
// what a scraper that closed its connection looks like from in here.
type hangUp struct {
	http.ResponseWriter
	writes int
	// accepts is how many writes land before the reader is gone. One by
	// default; the runtime section's own probe needs two, because its first
	// two writes are the header it sends before measuring anything.
	accepts int
}

var errClientGone = errors.New("connection reset by peer")

func (h *hangUp) Write(p []byte) (int, error) {
	h.writes++
	if h.writes > h.accepts {
		return 0, errClientGone
	}
	return len(p), nil
}

func TestNothingIsMeasuredForAScrapeThatHasAlreadyGone(t *testing.T) {
	measured := map[string]bool{}
	// One write lands, so the runtime section's own header probe is what
	// discovers the writer is gone — before it stops the world to read memory
	// statistics nobody will receive. That saving is not observable from out
	// here, so it is not asserted; what is asserted is that no section after
	// it measures anything.
	w := &hangUp{ResponseWriter: httptest.NewRecorder(), accepts: 1}

	Metrics(nil,
		func(context.Context) (int64, error) { measured["backlog"] = true; return 0, nil },
		func() uint64 { return 0 },
		func(io.Writer) { measured["extra"] = true },
		func(context.Context, io.Writer) error { measured["jobs"] = true; return nil },
		&OverlayMetrics{
			SourceLag: func(context.Context) (map[string]time.Duration, error) {
				measured["overlay"] = true
				return map[string]time.Duration{}, nil
			},
			SyncedTotal:   func() uint64 { return 0 },
			ConflictTotal: func() uint64 { return 0 },
			DeletedTotal:  func() uint64 { return 0 },
		},
	)(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	// The runtime section is first and does the refusing, so everything after
	// it is work for a body that cannot be delivered.
	for _, section := range []string{"backlog", "extra", "jobs", "overlay"} {
		if measured[section] {
			t.Errorf("the %s section was measured after the scrape's writer was already gone — "+
				"a database read nobody will see the result of, on a socket that is not there", section)
		}
	}
}

// The remembered error, not nil. A section that DOES check its writes would
// otherwise be told each one succeeded and carry on assembling into nothing.
func TestTheExpositionKeepsRefusingAfterTheFirstFailure(t *testing.T) {
	out := &exposition{w: &hangUp{ResponseWriter: httptest.NewRecorder(), accepts: 1}}

	out.printf("first\n")
	out.printf("second\n")

	if !out.gone() {
		t.Fatal("the exposition reported nothing wrong after its writer refused")
	}
	if _, err := out.Write([]byte("third")); !errors.Is(err, errClientGone) {
		t.Errorf("a write after the refusal answered %v, want the remembered %v — "+
			"claiming success after failing lets a caller that checks keep writing into nothing", err, errClientGone)
	}
}

// Every supplier, not only the expensive ones. printf goes quiet after a
// refusal, but Go evaluates its arguments first, so a counter read still
// happens unless the section that would print it is guarded. Cheap here — the
// suppliers are atomic loads — and the point is the rule rather than the cost:
// "nothing is measured for a scrape that has gone" is either true of all of
// them or it is a claim a reader has to check one section at a time.
func TestNoCounterIsReadForAScrapeThatHasAlreadyGone(t *testing.T) {
	read := map[string]bool{}
	w := &hangUp{ResponseWriter: httptest.NewRecorder(), accepts: 1}

	Metrics(nil, nil,
		func() uint64 { read["published"] = true; return 0 },
		nil, nil,
		&OverlayMetrics{
			SourceLag:     func(context.Context) (map[string]time.Duration, error) { return map[string]time.Duration{}, nil },
			SyncedTotal:   func() uint64 { read["synced"] = true; return 0 },
			ConflictTotal: func() uint64 { read["conflict"] = true; return 0 },
			DeletedTotal:  func() uint64 { read["deleted"] = true; return 0 },
		},
	)(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	for _, supplier := range []string{"published", "synced", "conflict", "deleted"} {
		if read[supplier] {
			t.Errorf("the %s counter was read after the scrape's writer was already gone", supplier)
		}
	}
}

// A probe body is not an exposition, and it takes the same posture for a
// smaller reason: there is nothing to log, but a half-written answer is still
// not worth assembling.
func TestReadyzStopsWritingWhenItsReaderHangsUp(t *testing.T) {
	w := &hangUp{ResponseWriter: httptest.NewRecorder(), accepts: 1}
	resolved := false

	Readyz("declared", func(context.Context) string { resolved = true; return "ready" })(
		w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if resolved {
		t.Error("the embed marker was resolved after the probe's reader was gone — a read for a " +
			"body that cannot be delivered")
	}

	// "ready" lands, the ai line is refused, and the embed line must not add a
	// second attempt on a writer that already said no.
	if w.writes != 2 {
		t.Errorf("the probe body attempted %d writes, want 2 — the first is the answer, the second "+
			"is what discovers the reader is gone, and nothing after it should be tried", w.writes)
	}
}

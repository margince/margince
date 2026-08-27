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
}

var errClientGone = errors.New("connection reset by peer")

func (h *hangUp) Write(p []byte) (int, error) {
	h.writes++
	if h.writes > 1 {
		return 0, errClientGone
	}
	return len(p), nil
}

func TestNothingIsMeasuredForAScrapeThatHasAlreadyGone(t *testing.T) {
	measured := map[string]bool{}
	w := &hangUp{ResponseWriter: httptest.NewRecorder()}

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
	out := &exposition{w: &hangUp{ResponseWriter: httptest.NewRecorder()}}

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

// A probe body is not an exposition, and it takes the same posture for a
// smaller reason: there is nothing to log, but a half-written answer is still
// not worth assembling.
func TestReadyzStopsWritingWhenItsReaderHangsUp(t *testing.T) {
	w := &hangUp{ResponseWriter: httptest.NewRecorder()}

	Readyz("declared", func(context.Context) string { return "ready" })(
		w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	// "ready" lands, the ai line is refused, and the embed line must not add a
	// second attempt on a writer that already said no.
	if w.writes != 2 {
		t.Errorf("the probe body attempted %d writes, want 2 — the first is the answer, the second "+
			"is what discovers the reader is gone, and nothing after it should be tried", w.writes)
	}
}

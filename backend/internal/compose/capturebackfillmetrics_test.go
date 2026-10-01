// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestTheBackfillFleetWritesAZeroForEveryAdmittedStatus(t *testing.T) {
	var buf bytes.Buffer
	err := writeBackfillFleet(&buf, capture.BackfillFleet{
		Runs: map[string]int64{"running": 2, "done": 5, "paused": 1},
		Live: capture.BackfillProgress{Scanned: 470, Captured: 300, Skipped: 120, TotalEstimate: 9000},
	})
	if err != nil {
		t.Fatalf("writeBackfillFleet: %v", err)
	}
	for _, line := range []string{
		"# TYPE margince_capture_backfill_runs gauge",
		`margince_capture_backfill_runs{status="queued"} 0`,
		`margince_capture_backfill_runs{status="running"} 2`,
		`margince_capture_backfill_runs{status="done"} 5`,
		`margince_capture_backfill_runs{status="error"} 0`,
		`margince_capture_backfill_runs{status="cancelled"} 0`,
		`margince_capture_backfill_runs{status="paused"} 1`,
		"# TYPE margince_capture_backfill_progress gauge",
		`margince_capture_backfill_progress{field="scanned"} 470`,
		`margince_capture_backfill_progress{field="captured"} 300`,
		`margince_capture_backfill_progress{field="skipped"} 120`,
		`margince_capture_backfill_progress{field="total_estimate"} 9000`,
	} {
		if !strings.Contains(buf.String(), line+"\n") {
			t.Errorf("exposition missing %q\ngot:\n%s", line, buf.String())
		}
	}
}

// A read that failed writes nothing rather than zeroes, which would read as no
// import running.
func TestAFailedBackfillReadWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	section := backfillFleetSection(func(context.Context) (capture.BackfillFleet, error) {
		return capture.BackfillFleet{}, errors.New("database down")
	})
	if err := section(t.Context(), &buf); err != nil {
		t.Fatalf("a failed read is not a refused write: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("a failed read wrote:\n%s", buf.String())
	}
}

// refusingWriter fails every write after the first n bytes.
type refusingWriter struct{ n int }

func (w *refusingWriter) Write(p []byte) (int, error) {
	if len(p) > w.n {
		return 0, io.ErrClosedPipe
	}
	w.n -= len(p)
	return len(p), nil
}

func TestARefusedWriteStopsTheFleetSections(t *testing.T) {
	ran := 0
	refused := func(context.Context, io.Writer) error { ran++; return io.ErrClosedPipe }
	after := func(context.Context, io.Writer) error { ran++; return nil }
	if err := fleetSections(refused, after)(t.Context(), io.Discard); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("fleetSections = %v, want the refused write", err)
	}
	if ran != 1 {
		t.Errorf("%d sections ran, want the one that refused and none after it", ran)
	}

	for budget := range 600 {
		if err := writeBackfillFleet(&refusingWriter{n: budget}, capture.BackfillFleet{}); err == nil {
			t.Fatalf("a writer refusing after %d bytes was reported as written in full", budget)
		}
	}
}

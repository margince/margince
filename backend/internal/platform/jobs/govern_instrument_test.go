// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/instrument"
	"github.com/margince/margince/backend/pkg/extension"
)

type probeArgs struct{}

func (probeArgs) Kind() string { return "probe_kind" }

type probeWork struct{ panics bool }

func (p probeWork) Work(context.Context, *river.Job[probeArgs]) error {
	if p.panics {
		panic("job blew up")
	}
	return nil
}

func TestGovernRunsEachJobInsideTheWorkInstruments(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var got extension.Work
	instrument.Install([]instrument.Started{{Unit: "u", Hooks: extension.Instruments{
		Work: func(ctx context.Context, w extension.Work, run func(context.Context) error) error {
			got = w
			return run(ctx)
		},
	}}}, slog.New(slog.DiscardHandler))
	w := governedWorker[probeArgs]{work: probeWork{}}
	if err := w.Work(context.Background(), &river.Job[probeArgs]{}); err != nil {
		t.Fatal(err)
	}
	if got.Kind != extension.WorkJob || got.Name != "probe_kind" {
		t.Fatalf("work = %+v", got)
	}
}

func TestGovernReportsAPanicAndStillLetsRiverRecoverIt(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var reported any
	instrument.Install([]instrument.Started{{Unit: "u", Hooks: extension.Instruments{
		Panic: func(_ context.Context, v any, _ []byte) { reported = v },
	}}}, slog.New(slog.DiscardHandler))
	w := governedWorker[probeArgs]{work: probeWork{panics: true}}
	var repanicked any
	func() {
		defer func() { repanicked = recover() }()
		//craft:ignore swallowed-errors the work panics, so Work never returns a value
		_ = w.Work(context.Background(), &river.Job[probeArgs]{})
	}()
	if reported != "job blew up" || repanicked != "job blew up" {
		t.Fatalf("reported=%v repanicked=%v", reported, repanicked)
	}
}

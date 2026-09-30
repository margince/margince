// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The receipt each capture repair pass leaves behind, so the capture-health
// page can say when it last succeeded.
//
// The passes do not run in one transaction — each repair commits on its own —
// so the receipt is a separate small write after the pass returns, carrying
// what was committed so far. A receipt that cannot be written is reported
// beside the pass's own error and undoes nothing. A pass that panics still
// leaves a failed receipt; a process that dies mid-pass leaves none, and the
// card's overdue warning is what reports that.

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// sweepTally is what one pass committed, and whether it stopped at its
// per-tick bound with more left in its selection.
type sweepTally struct {
	processed int
	capHit    bool
}

// sweepLedger is where a receipt goes: capture.SweepLedger in production.
type sweepLedger interface {
	RecordSweep(ctx context.Context, r capture.SweepReceipt) error
}

// sweepReceiptTimeout bounds the receipt write, which runs even after the
// pass's own context has been cancelled.
const sweepReceiptTimeout = 5 * time.Second

// unclassifiedSweepFailure is the class of a failure the job fault vocabulary
// cannot name. The cause itself goes to the job's log, never to the receipt.
const unclassifiedSweepFailure = "unclassified"

// panickedSweepFailure is the class of a pass that panicked rather than
// returning an error.
const panickedSweepFailure = "panicked"

type sweepRecorder struct {
	ledger sweepLedger
	now    func() time.Time
}

func newSweepRecorder(pool *pgxpool.Pool) sweepRecorder {
	return sweepRecorder{ledger: capture.NewSweepLedger(InstallationDB(pool)), now: time.Now}
}

// run times one pass and records how it ended. The pass counts into tally as
// it commits, so a panic partway still reports what it had done; the panic is
// recorded and then re-raised, never absorbed.
func (r sweepRecorder) run(
	ctx context.Context, sweep capture.Sweep, pass func(tally *sweepTally) error,
) (tally sweepTally, err error) {
	started := r.now()
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		receipt := sweepReceiptFor(sweep, started, r.now(), tally, errors.New("the pass panicked"))
		receipt.ErrorClass = panickedSweepFailure
		if recordErr := r.record(ctx, receipt); recordErr != nil {
			slog.ErrorContext(ctx, "capture: the receipt of a panicked pass was not written",
				"sweep", string(sweep), "err", recordErr)
		}
		panic(recovered)
	}()
	err = pass(&tally)
	receipt := sweepReceiptFor(sweep, started, r.now(), tally, err)
	return tally, errors.Join(err, r.record(ctx, receipt))
}

// skipped records a pass that never ran because an earlier stage failed.
func (r sweepRecorder) skipped(ctx context.Context, sweep capture.Sweep) error {
	at := r.now()
	return r.record(ctx, capture.SweepReceipt{
		Sweep: sweep, StartedAt: at, FinishedAt: at, Outcome: capture.SweepSkipped,
	})
}

func (r sweepRecorder) record(ctx context.Context, receipt capture.SweepReceipt) error {
	// Detached from the pass's cancellation: a pass that ran out of time is
	// the one whose failed receipt most needs to land.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sweepReceiptTimeout)
	defer cancel()
	return r.ledger.RecordSweep(writeCtx, receipt)
}

// sweepReceiptFor turns a pass's result into its receipt.
func sweepReceiptFor(
	sweep capture.Sweep, started, finished time.Time, tally sweepTally, err error,
) capture.SweepReceipt {
	out := capture.SweepReceipt{
		Sweep: sweep, StartedAt: started, FinishedAt: finished,
		Processed: tally.processed, CapHit: tally.capHit, Outcome: capture.SweepOK,
	}
	switch {
	case err != nil:
		out.Outcome = capture.SweepFailed
		out.ErrorClass = jobs.ClassFor(err)
		if out.ErrorClass == "" {
			out.ErrorClass = unclassifiedSweepFailure
		}
	case tally.capHit:
		out.Outcome = capture.SweepPartial
	}
	return out
}

// vettedSweepClass admits a stored class onto the page only when it is one the
// vocabulary owns. The column's CHECK admits any token; the page promises one
// an operator can look up.
func vettedSweepClass(stored string) string {
	if stored == "" || stored == unclassifiedSweepFailure || stored == panickedSweepFailure ||
		slices.Contains(jobs.CoreFailureClasses(), stored) {
		return stored
	}
	return unclassifiedSweepFailure
}

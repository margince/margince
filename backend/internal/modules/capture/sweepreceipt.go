// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// When each of capture's repair passes last ran, and how it ended.
//
// River cannot answer it: one job row covers every workspace and every stage,
// and a completed row is deleted after a day. So each pass writes its own
// receipt after it returns — operational bookkeeping about a pass, not a record
// fact, so no audit row and no event. It names no message, contact or address:
// a sweep, two times, an outcome, a count and a class token.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// Sweep names one repair pass.
type Sweep string

// The passes that write a receipt, in the order a report lists them.
const (
	SweepSettledThreadVerdicts Sweep = "settled_thread_verdicts"
	SweepStrandedContacts      Sweep = "stranded_contacts"
	SweepFiledMeetingHolds     Sweep = "filed_meeting_holds"
)

// Sweeps lists every pass that writes a receipt. The table's CHECK and the
// contract's enum spell the same set; held by
// TestEverySweepAndOutcomeIsAcceptedByTheTable (compose).
func Sweeps() []Sweep {
	return []Sweep{SweepSettledThreadVerdicts, SweepStrandedContacts, SweepFiledMeetingHolds}
}

// SweepOutcome is how one pass ended.
type SweepOutcome string

const (
	// SweepOK ran to completion with nothing left in its selection.
	SweepOK SweepOutcome = "ok"
	// SweepPartial ran cleanly but stopped at its per-tick bound, so a backlog
	// remains for the next tick.
	SweepPartial SweepOutcome = "partial"
	// SweepFailed returned an error; what it committed before stays committed.
	SweepFailed SweepOutcome = "failed"
	// SweepSkipped never ran because an earlier stage of the same turn failed.
	SweepSkipped SweepOutcome = "skipped"
)

// Succeeded reports whether the pass did its work. A partial pass did: the
// bound is the design, and "never succeeded" would be false of it.
func (o SweepOutcome) Succeeded() bool { return o == SweepOK || o == SweepPartial }

// SweepReceipt is one pass's record of itself.
type SweepReceipt struct {
	Sweep      Sweep
	StartedAt  time.Time
	FinishedAt time.Time
	Outcome    SweepOutcome
	Processed  int
	CapHit     bool
	// ErrorClass is a job fault class token, set exactly when the pass failed.
	ErrorClass string
}

// SweepReceiptsKept bounds the history per pass. Enough for a day of the
// ten-minute pass; the newest success is kept beyond it.
const SweepReceiptsKept = 200

// SweepLedger writes and reads the receipts.
type SweepLedger struct{ db *database.DB }

// NewSweepLedger builds the ledger on a handle bound to the workspace it serves.
func NewSweepLedger(db *database.DB) *SweepLedger { return &SweepLedger{db: db} }

// RecordSweep stores one receipt and prunes that pass's history in the same
// transaction.
//
// The prune never deletes the newest successful receipt, so "when did this
// last work" survives any run of failures longer than the history.
func (l *SweepLedger) RecordSweep(ctx context.Context, r SweepReceipt) error {
	var errorClass *string
	if r.ErrorClass != "" {
		errorClass = &r.ErrorClass
	}
	err := l.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_sweep_run
			       (sweep, started_at, finished_at, outcome, processed, cap_hit, error_class)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			string(r.Sweep), r.StartedAt, r.FinishedAt, string(r.Outcome),
			r.Processed, r.CapHit, errorClass); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			DELETE FROM capture_sweep_run r
			 WHERE r.sweep = $1
			   AND r.id NOT IN (SELECT k.id FROM capture_sweep_run k
			                     WHERE k.sweep = $1
			                     ORDER BY k.finished_at DESC, k.id DESC
			                     LIMIT $2)
			   AND r.id IS DISTINCT FROM (SELECT s.id FROM capture_sweep_run s
			                               WHERE s.sweep = $1 AND s.outcome IN ('ok', 'partial')
			                               ORDER BY s.finished_at DESC, s.id DESC
			                               LIMIT 1)`,
			string(r.Sweep), SweepReceiptsKept)
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: recording the %s pass: %w", r.Sweep, err)
	}
	return nil
}

// SweepState is where one pass stands: its latest receipt, and when it last
// succeeded.
type SweepState struct {
	Sweep Sweep
	// Latest is nil when the pass has never written a receipt.
	Latest *SweepReceipt
	// LastSucceededAt is nil when no receipt on record succeeded.
	LastSucceededAt *time.Time
}

// SweepStatesTx answers every pass's state, in Sweeps order, on the caller's
// transaction so it reads the same instant as the report around it.
func SweepStatesTx(ctx context.Context, tx pgx.Tx) ([]SweepState, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.sweep, l.started_at, l.finished_at, l.outcome, l.processed, l.cap_hit,
		       coalesce(l.error_class, ''),
		       (SELECT max(k.finished_at) FROM capture_sweep_run k
		         WHERE k.sweep = s.sweep AND k.outcome IN ('ok', 'partial'))
		  FROM unnest($1::text[]) WITH ORDINALITY AS s(sweep, ord)
		  LEFT JOIN LATERAL (SELECT * FROM capture_sweep_run r
		                      WHERE r.sweep = s.sweep
		                      ORDER BY r.finished_at DESC, r.id DESC
		                      LIMIT 1) l ON true
		 ORDER BY s.ord`, sweepNames())
	if err != nil {
		return nil, fmt.Errorf("capture: reading when each repair pass last ran: %w", err)
	}
	defer rows.Close()
	out := []SweepState{}
	for rows.Next() {
		var (
			state             SweepState
			started, finished *time.Time
			outcome, class    *string
			processed         *int
			capHit            *bool
		)
		if err := rows.Scan(&state.Sweep, &started, &finished, &outcome, &processed,
			&capHit, &class, &state.LastSucceededAt); err != nil {
			return nil, fmt.Errorf("capture: reading when each repair pass last ran: %w", err)
		}
		if outcome != nil {
			state.Latest = &SweepReceipt{
				Sweep: state.Sweep, StartedAt: *started, FinishedAt: *finished,
				Outcome: SweepOutcome(*outcome), Processed: *processed, CapHit: *capHit,
				ErrorClass: *class,
			}
		}
		out = append(out, state)
	}
	return out, rows.Err()
}

func sweepNames() []string {
	out := make([]string, 0, len(Sweeps()))
	for _, s := range Sweeps() {
		out = append(out, string(s))
	}
	return out
}

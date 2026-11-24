// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package purgerun ties a purge's per-message audit rows to its receipt. Each
// run gets a fresh id that its rows and its receipt carry as batch_id. The
// receipt counts its own run, and a run with no receipt can be found and
// repaired.
//
// The rule purge and the personal-mail sweep share it: one function stamps
// the run, one counts it, one repairs a missing receipt.
package purgerun

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// EntityRule is the receipt's entity for an owner's or admin's rule purge.
	EntityRule = "capture_exclusion"
	// EntityPersonalSweep is the receipt's entity for one seat's personal-mail pass.
	EntityPersonalSweep = "capture_personal_sweep"
	// RepairActor is the system principal a repaired receipt is written as.
	RepairActor = "system:capture-purge-receipt"

	orphanLookback = 14 * 24 * time.Hour
	orphanBatch    = 500
	lockPoll       = 200 * time.Millisecond
)

// Subject is what a receipt is about: the entity it is written against.
type Subject struct {
	Entity string
	ID     ids.UUID
}

// Rule is the subject of an exclusion rule's purge.
func Rule(id ids.UUID) Subject { return Subject{Entity: EntityRule, ID: id} }

// Seat is the subject of one seat's personal-mail pass.
func Seat(id ids.UUID) Subject { return Subject{Entity: EntityPersonalSweep, ID: id} }

// fingerprint is the part of a run id that names its subject. The per-message
// rows cannot name it themselves: an erase row keeps no custom evidence once it
// is redacted. So the run id carries 62 bits of a hash of the subject, which
// lets a receipt-less run be attributed by checking it against each candidate.
func fingerprint(subject ids.UUID) [8]byte {
	sum := sha256.Sum256(subject[:])
	var fp [8]byte
	copy(fp[:], sum[:8])
	fp[0] = fp[0]&0x3f | 0x80 // the UUID variant bits
	return fp
}

// NewRunID makes the id of one run: time-ordered, with the subject's
// fingerprint in the low half. It is version 8, which no other batch id is, so
// a scan tells a purge run from a bulk change by its text.
func NewRunID(subject ids.UUID) ids.UUID {
	run := ids.NewV7()
	run[6] = run[6]&0x0f | 0x80
	fp := fingerprint(subject)
	copy(run[8:], fp[:])
	return run
}

func attributes(run, subject ids.UUID) bool {
	fp := fingerprint(subject)
	return run[6]>>4 == 8 && [8]byte(run[8:]) == fp
}

// Hold runs fn as one run of the subject, holding the subject's lock until fn
// returns. Two runs of one subject never overlap, a rule's removal waits for
// the run, and the sweep leaves it alone. The lock sits on its own connection,
// so it ends with the process.
//
// fn gets a context stamped with the run id: every audit row written under it
// carries the run as batch_id.
func Hold(ctx context.Context, pool *pgxpool.Pool, subject Subject, fn func(ctx context.Context, run ids.UUID) error) error {
	key := storekit.PurgeSubjectLockKey(subject.ID)
	for {
		lease, held, err := database.TrySessionLease(ctx, pool, key, slog.Default())
		if err != nil {
			return fmt.Errorf("purge run: locking the subject: %w", err)
		}
		if held {
			defer lease.Close(context.WithoutCancel(ctx))
			run := NewRunID(subject.ID)
			return fn(storekit.WithBatch(ctx, run), run)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lockPoll):
		}
	}
}

// Tally is what a run's per-message rows say was done.
type Tally struct{ Destroyed, Released, Anonymised int }

// Evidence is the tally as receipt evidence.
func (t Tally) Evidence() map[string]any {
	return map[string]any{"destroyed": t.Destroyed, "released": t.Released, "anonymised": t.Anonymised}
}

// rowKinds is the rows a cascade writes: an erase per destroyed message, an
// archive per released one and an anonymize per stripped contact. Only action,
// entity type and run id are read, because redaction can strip an image.
const rowKinds = `((a.action = 'erase' AND a.entity_type = 'activity')
	  OR (a.action = 'archive' AND a.entity_type = 'activity')
	  OR (a.action = 'anonymize' AND a.entity_type = 'contact'))`

// TallyTx counts the rows of one run.
func TallyTx(ctx context.Context, tx pgx.Tx, run ids.UUID) (Tally, error) {
	var t Tally
	err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT a.entity_id) FILTER (WHERE a.action = 'erase'),
		       count(DISTINCT a.entity_id) FILTER (WHERE a.action = 'archive'),
		       count(DISTINCT a.entity_id) FILTER (WHERE a.action = 'anonymize')
		  FROM audit_log a WHERE a.batch_id = @run AND `+rowKinds,
		pgx.StrictNamedArgs{"run": run}).Scan(&t.Destroyed, &t.Released, &t.Anonymised)
	if err != nil {
		return Tally{}, fmt.Errorf("purge run: counting what the run did: %w", err)
	}
	return t, nil
}

// WriteReceipt counts the run and writes its receipt in one transaction. build
// turns the count into the receipt's evidence, or returns nil for a run that
// left nothing to certify. ctx must carry the run (Hold's does). It returns
// the count the receipt states.
func WriteReceipt(ctx context.Context, pool *pgxpool.Pool, subject Subject, run ids.UUID, build func(Tally) map[string]any) (Tally, error) {
	var tally Tally
	err := database.WithWorkspaceTx(storekit.WithBatch(ctx, run), pool, func(tx pgx.Tx) error {
		var err error
		if tally, err = TallyTx(ctx, tx, run); err != nil {
			return err
		}
		evidence := build(tally)
		if evidence == nil {
			return nil
		}
		_, err = storekit.AuditWithEvidence(storekit.WithBatch(ctx, run), tx, "archive", subject.Entity, subject.ID, nil, nil, evidence)
		return err
	})
	return tally, err
}

// orphanRunsTx lists recent runs that have per-message rows and no receipt.
func orphanRunsTx(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT a.batch_id FROM audit_log a
		 WHERE a.batch_id IS NOT NULL AND substr(a.batch_id::text, 15, 1) = '8'
		   AND a.occurred_at > now() - make_interval(secs => @lookback_secs)
		   AND `+rowKinds+`
		   AND NOT EXISTS (SELECT 1 FROM audit_log r WHERE r.batch_id = a.batch_id
		                    AND r.entity_type = ANY(@receipt_entities))
		 LIMIT @limit`,
		pgx.StrictNamedArgs{
			"lookback_secs": orphanLookback.Seconds(), "limit": orphanBatch,
			"receipt_entities": []string{EntityRule, EntityPersonalSweep},
		})
	if err != nil {
		return nil, fmt.Errorf("purge run: finding runs without a receipt: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// RepairTx writes the receipt of every run of the subject that has rows and no
// receipt. The caller holds the subject's lock, so no run is in flight. A
// repaired receipt says `recovered` and has no kept counts, because what a
// purge left alone wrote no row.
func RepairTx(ctx context.Context, tx pgx.Tx, subject Subject) (int, error) {
	ctx = principal.SystemActing(ctx, RepairActor)
	orphans, err := orphanRunsTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, run := range orphans {
		if !attributes(run, subject.ID) {
			continue
		}
		tally, err := TallyTx(ctx, tx, run)
		if err != nil {
			return written, err
		}
		evidence := tally.Evidence()
		evidence["recovered"] = true
		if _, err := storekit.AuditWithEvidence(storekit.WithBatch(ctx, run), tx, "archive",
			subject.Entity, subject.ID, nil, nil, evidence); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// SweepResult is what one sweep did.
type SweepResult struct {
	// Written is the receipts it wrote.
	Written int
	// Unattributed is the runs it found no rule or seat for: the subject is
	// gone, so there is no entity to write a receipt against. They are left
	// alone and reported.
	Unattributed int
}

// Sweep repairs every attributable run that has no receipt, for each subject
// whose lock it can take. A subject whose lock is held has a run in flight, and
// is left for the next pass.
func Sweep(ctx context.Context, pool *pgxpool.Pool) (SweepResult, error) {
	var orphans []ids.UUID
	var subjects []Subject
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) (err error) {
		if orphans, err = orphanRunsTx(ctx, tx); err != nil || len(orphans) == 0 {
			return err
		}
		subjects, err = candidateSubjectsTx(ctx, tx)
		return err
	}); err != nil {
		return SweepResult{}, err
	}
	var result SweepResult
	owed := map[Subject]bool{}
	for _, run := range orphans {
		found := false
		for _, s := range subjects {
			if attributes(run, s.ID) {
				owed[s], found = true, true
				break
			}
		}
		if !found {
			result.Unattributed++
		}
	}
	for s := range owed {
		n, err := repairIfIdle(ctx, pool, s)
		result.Written += n
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func candidateSubjectsTx(ctx context.Context, tx pgx.Tx) ([]Subject, error) {
	var out []Subject
	for _, source := range []struct {
		entity, query string
	}{
		{EntityRule, `SELECT id FROM capture_exclusion`},
		{EntityPersonalSweep, `SELECT id FROM app_user`},
	} {
		rows, err := tx.Query(ctx, source.query)
		if err != nil {
			return nil, fmt.Errorf("purge run: listing who a run could belong to: %w", err)
		}
		found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return nil, fmt.Errorf("purge run: listing who a run could belong to: %w", err)
		}
		for _, id := range found {
			out = append(out, Subject{Entity: source.entity, ID: id})
		}
	}
	return out, nil
}

func repairIfIdle(ctx context.Context, pool *pgxpool.Pool, s Subject) (int, error) {
	lease, held, err := database.TrySessionLease(ctx, pool, storekit.PurgeSubjectLockKey(s.ID), slog.Default())
	if err != nil || !held {
		return 0, err
	}
	defer lease.Close(context.WithoutCancel(ctx))
	written := 0
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) (err error) {
		written, err = RepairTx(ctx, tx, s)
		return err
	})
	return written, err
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The engine's source kinds, as stored in import_run.connector. The
// migrate-in connectors (csv/hubspot/salesforce) are in the column's
// CHECK — the DDL is the chapter's pinned arrival shape — but they get
// their Go constants when their connectors land (UC-E11-03), not before.
const (
	// ConnectorMirror is the overlay→native flip: the frozen mirror snapshot.
	ConnectorMirror = "mirror"
	// ConnectorBundle is reconstruction from a pre-flip export bundle.
	ConnectorBundle = "bundle"
)

// pageSize bounds one Source.Rows read: large enough to amortize the
// round-trip, small enough that a resumed run re-reads at most one page.
const pageSize = 200

// Row is one source record: the incumbent/external id, the canonical
// field map (keys are native column names — the mirror ingest projector
// already speaks this shape), and the record's last sync instant.
type Row struct {
	ExternalID string
	Fields     map[string]any
	// OwnerExternalID is the source's owner id, kept OUT of Fields on
	// purpose: Fields is the canonical payload and its emptiness is what
	// the empty_payload skip reads. Carrying transport metadata in there
	// would make every owned-but-blank system entry look like a record
	// worth creating, and the writer would land a nameless native row.
	OwnerExternalID string
	LastSyncedAt    time.Time
	// Line is the file line this row came from, for a source that has one.
	//
	// CARRIED rather than derived. It used to be recovered from the external
	// id's text shape, because a row the source could not identify is disclosed
	// as "line N" — but a file with its own key column gives every row a real
	// id, so that parse failed and every issue in such a file reported line 0.
	// Zero for a source with no file behind it, which is the honest answer
	// there.
	Line int
}

// Assoc is one detangled source edge, applied after both endpoints
// landed (IEM-FORM-2: edges become FKs or typed relationship rows —
// the Writers implementation owns that mapping).
type Assoc struct {
	FromType string
	FromID   string
	ToType   string
	ToID     string
	Category string
	Label    string
}

// Source is one estate to import. Objects fixes the import order
// (parents before dependents); Rows pages a stable, deterministic
// ordering — the checkpoint's resume contract depends on it.
type Source interface {
	Objects() []string
	Counts(ctx context.Context) (map[string]int, error)
	Rows(ctx context.Context, object string, offset, limit int) ([]Row, error)
	Associations(ctx context.Context) ([]Assoc, error)
}

// EnsureResult reports what one Writers.Ensure did. Skips and
// disclosures are never silent — both land in the run report
// (AC-mode-flip-7: skipped rows carry reasons).
type EnsureResult struct {
	Created bool
	// Unchanged marks a row that already landed under this provenance
	// and was NOT rewritten — a resumed run replaying its last page, or
	// a re-run over the same frozen source. It is neither a create nor
	// an update: counting it as either would inflate the disposition
	// table with work that never happened.
	Unchanged  bool
	Skipped    bool
	SkipReason string
	// Duplicate marks a row that named a record the estate ALREADY held and
	// that this importer had not landed before. It is not an outcome — the row
	// is also Created, or Skipped when the run asked for that — which is why it
	// is a flag beside them rather than a fifth case.
	//
	// It is answered on the commit for the reason the preview answers it: the
	// estate moves between the two. A colleague creates one of the companies,
	// or an earlier run lands it, and a finished report carrying only the
	// prediction states a duplicate count that was true when the preview ran.
	Duplicate bool
	// Disclosure names a lossy-but-disclosed mapping decision (e.g. a
	// deal materialized onto the default pipeline because the source
	// stage identity did not resolve).
	Disclosure string
}

// AssocResult reports what one Writers.Associate did — an edge whose
// endpoint never landed, or whose shape the native model has no target
// for, is DISCLOSED here rather than vanishing into a bare nil.
type AssocResult struct {
	Applied bool
	// Reason explains a non-applied edge; empty when Applied.
	Reason string
}

// Writers is the native-record seam: compose implements it over the
// people/deals/activities stores so this module never imports a sibling.
// Every method must be idempotent on the row's provenance key — the
// checkpointed run loop may replay the row after a crash, and a re-run
// of the whole source must converge (IEM-FORM-1's upsert-by-key).
type Writers interface {
	Exists(ctx context.Context, object, externalID string) (bool, error)
	// ReconcileIdentities repairs the record of what already landed
	// before a RESUMED run walks its source again: a writer whose native
	// create and identity write were separate transactions left records
	// nothing can now recognize, and the resume would create them a
	// second time. Called only when resuming, and answering nothing when
	// the writer lands both in one transaction.
	ReconcileIdentities(ctx context.Context) error
	Ensure(ctx context.Context, object string, row Row) (EnsureResult, error)
	Associate(ctx context.Context, a Assoc) (AssocResult, error)
}

// runRecords is what the loop needs from the run store — an interface so
// the loop's checkpoint/resume contract is provable without Postgres.
type runRecords interface {
	Get(ctx context.Context, id RunID) (Run, error)
	advanceCheckpoint(ctx context.Context, id RunID, checkpoint int) error
	complete(ctx context.Context, id RunID, rep Report) error
	failRun(ctx context.Context, id RunID, rep Report, cause error) error
}

// Engine runs one Source through the Writers seam. It owns
// classification and the checkpointed loop; it owns no SQL of its own
// beyond the RunStore's run records.
type Engine struct {
	runs runRecords
	w    Writers
}

// NewEngine wires the engine over its two seams.
func NewEngine(runs *RunStore, w Writers) *Engine {
	return &Engine{runs: runs, w: w}
}

// DryRun classifies every source row without writing one native record
// (AC-M5 / AC-mode-flip-7): per object it reports how many rows would
// create vs update, and which are skipped with reasons.
func (e *Engine) DryRun(ctx context.Context, src Source) (Report, error) {
	counts, err := src.Counts(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("migration dry-run: counting source rows: %w", err)
	}
	var rep Report
	for _, object := range src.Objects() {
		or := ObjectReport{Object: object, MirrorCount: counts[object]}
		for offset := 0; ; offset += pageSize {
			rows, err := src.Rows(ctx, object, offset, pageSize)
			if err != nil {
				return Report{}, fmt.Errorf("migration dry-run: reading %s rows at %d: %w", object, offset, err)
			}
			if len(rows) == 0 {
				break
			}
			for _, row := range rows {
				if len(row.Fields) == 0 {
					or.Skipped = append(or.Skipped, SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Reason: skipReasonEmptyPayload})
					continue
				}
				exists, err := e.w.Exists(ctx, object, row.ExternalID)
				if err != nil {
					return Report{}, fmt.Errorf("migration dry-run: classifying %s %s: %w", object, row.ExternalID, err)
				}
				if exists {
					or.WillUpdate++
				} else {
					or.WillCreate++
				}
			}
			if len(rows) < pageSize {
				break
			}
		}
		rep.Objects = append(rep.Objects, or)
	}
	assocs, err := src.Associations(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("migration dry-run: reading associations: %w", err)
	}
	// The dry-run writes nothing, so it cannot resolve endpoints: this is
	// the count of edges the source OFFERS, and the run's own report is
	// what says how many were applied.
	rep.Associations = len(assocs)
	return rep, nil
}

// Run executes the import for an already-created run record, resuming
// from its checkpoint: rows are processed in the Source's stable order,
// and the checkpoint advances after every upsert (IEM-FORM-1), so a
// killed run re-entered with the same run id converges on the identical
// end state as an uninterrupted one. The association phase follows the
// row phase and is idempotent as a whole; the run completes (with its
// report persisted) or fails with the error recorded.
func (e *Engine) Run(ctx context.Context, runID RunID, src Source) (Report, error) {
	run, err := e.runs.Get(ctx, runID)
	if err != nil {
		return Report{}, err
	}
	if run.Status != StatusRunning {
		return Report{}, fmt.Errorf("migration run %s is %s, not %s: %w", runID, run.Status, StatusRunning, apperrors.ErrConflict)
	}
	// Unconditionally, before the loop can duplicate anything: adopt
	// records an earlier attempt created but never got to record.
	//
	// Gating this on the run's checkpoint looked like a free
	// optimization and was a hole. The checkpoint advances only AFTER a
	// row lands, so a crash on the very first row leaves it at zero —
	// and a re-created run (a fresh bundle upload, a re-sealed snapshot)
	// starts at zero with a previous attempt's orphans still on disk.
	// Both are exactly the case the repair exists for.
	if err := e.w.ReconcileIdentities(ctx); err != nil {
		return Report{}, e.fail(ctx, runID, Report{}, fmt.Errorf("migration run: reconciling a previous attempt's records: %w", err))
	}
	counts, err := src.Counts(ctx)
	if err != nil {
		return Report{}, e.fail(ctx, runID, Report{}, fmt.Errorf("migration run: counting source rows: %w", err))
	}

	rep := Report{}
	done := run.Checkpoint // rows already processed across the ordered objects
	seen := 0              // global index of the row about to be processed
	for _, object := range src.Objects() {
		or, advanced, err := e.importObject(ctx, runID, src, object, counts[object], done, seen, &rep)
		if err != nil {
			return Report{}, err
		}
		rep.Objects = append(rep.Objects, or)
		rep.Imported += int64(or.Created + or.Updated)
		// Only the global cursor advances. `done` is the checkpoint this
		// attempt STARTED from and must stay put: reassigning it would
		// make the next class compute a zero local offset and re-walk
		// rows the run already landed, which the store's monotonic
		// cursor then refuses — wedging every retry of a crashed flip.
		seen = advanced
	}

	assocs, err := src.Associations(ctx)
	if err != nil {
		return Report{}, e.fail(ctx, runID, rep, fmt.Errorf("migration run: reading associations: %w", err))
	}
	for _, a := range assocs {
		res, err := e.w.Associate(ctx, a)
		if err != nil {
			return Report{}, e.fail(ctx, runID, rep, fmt.Errorf("migration run: applying association %s/%s→%s/%s: %w", a.FromType, a.FromID, a.ToType, a.ToID, err))
		}
		if res.Applied {
			rep.Associations++
			continue
		}
		rep.AssociationsSkipped = append(rep.AssociationsSkipped, SkippedAssoc{
			From:   a.FromType + "/" + a.FromID,
			To:     a.ToType + "/" + a.ToID,
			Reason: res.Reason,
		})
	}

	if err := e.runs.complete(ctx, runID, rep); err != nil {
		return Report{}, e.fail(ctx, runID, rep, fmt.Errorf("migration run: recording completion: %w", err))
	}
	return rep, nil
}

// importObject runs one object class's rows, resuming past the prefix
// an earlier attempt already landed. It returns the class's disposition
// and the new global cursor — the checkpoint advances after every row,
// so a kill mid-class resumes from the next one.
func (e *Engine) importObject(ctx context.Context, runID RunID, src Source, object string, total, done, seen int, rep *Report) (ObjectReport, int, error) {
	or := ObjectReport{Object: object, MirrorCount: total}
	// A whole already-processed class is skipped without re-reading it.
	if done >= seen+total {
		return or, seen + total, nil
	}
	localOffset := max(done-seen, 0)
	cursor := seen + localOffset
	for offset := localOffset; ; {
		rows, err := src.Rows(ctx, object, offset, pageSize)
		if err != nil {
			return ObjectReport{}, 0, e.fail(ctx, runID, withPartial(*rep, or), fmt.Errorf("migration run: reading %s rows at %d: %w", object, offset, err))
		}
		if len(rows) == 0 {
			return or, cursor, nil
		}
		for _, row := range rows {
			res, err := e.ensureRow(ctx, object, row)
			if err != nil {
				return ObjectReport{}, 0, e.fail(ctx, runID, withPartial(*rep, or), err)
			}
			or.record(row, res)
			cursor++
			if err := e.runs.advanceCheckpoint(ctx, runID, cursor); err != nil {
				return ObjectReport{}, 0, e.fail(ctx, runID, withPartial(*rep, or), err)
			}
		}
		offset += len(rows)
		if len(rows) < pageSize {
			return or, cursor, nil
		}
	}
}

func (e *Engine) ensureRow(ctx context.Context, object string, row Row) (EnsureResult, error) {
	if len(row.Fields) == 0 {
		return EnsureResult{Skipped: true, SkipReason: skipReasonEmptyPayload}, nil
	}
	res, err := e.w.Ensure(ctx, object, row)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("migration run: importing %s %s: %w", object, row.ExternalID, err)
	}
	return res, nil
}

// fail records the run as failed — with the dispositions this attempt
// managed before it stopped — and returns the original error joined with
// any record-keeping failure, so the caller always sees why the run
// stopped. The partial report matters because a resumed run only ever
// reports its own attempt: without persisting this one, every record
// landed before the crash vanishes from the operator's final count.
func (e *Engine) fail(ctx context.Context, runID RunID, rep Report, cause error) error {
	if ferr := e.runs.failRun(ctx, runID, rep, cause); ferr != nil {
		return fmt.Errorf("%w (and recording the failure failed: %v)", cause, ferr)
	}
	return cause
}

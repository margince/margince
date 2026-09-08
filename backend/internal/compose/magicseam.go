// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Wiring the machinery's receipt.
//
// The one edge that needs explaining is the brief: the window this surface
// reports over defaults to when the night last READ the records, which is what
// the reader has already seen. That instant lives in compose/briefs, a sibling
// package, so it arrives through a seam rather than an import.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// newMagicService assembles the receipt's read.
func newMagicService(pool *pgxpool.Pool, now func() time.Time) *magic.Service {
	db := InstallationDB(pool)
	return magic.NewService(pool, magicBriefCutoff{
		engine: briefs.NewBriefEngine(pool, nil),
		now:    now,
	}, now).
		WithTroubledRuns(automation.NewAutomationStore(db))
}

// magicBriefCutoff answers when the acting rep's night last read the records.
//
// The run's as_of, not its generated_at. A run written at 06:42 over records
// read at 06:00 has a 42-minute window in which the machinery kept working, and
// reporting from the write time would hide exactly the overnight work this
// surface exists to show.
type magicBriefCutoff struct {
	engine *briefs.BriefEngine
	now    func() time.Time
}

// CutoffFor answers the cutoff and whether a run exists at all.
//
// No run is not a failure: the reader simply has no brief to date the window
// from, and the service falls back to a day. The refusal is reported as
// not-found rather than as an error for the same reason — a rep who has never
// had a brief is an ordinary state, not a broken read.
func (m magicBriefCutoff) CutoffFor(ctx context.Context) (time.Time, bool, error) {
	run, err := m.engine.LatestRun(ctx, m.now())
	if errors.Is(err, apperrors.ErrNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return run.AsOf, true, nil
}

// magicUndoJudge answers the receipt's "can this be taken back".
//
// TWO PATHS, asked in this order, because the product has two reversals and the
// generic one cannot see the other. A close-date correction is refused by
// undoability — it writes close_date_provisional, which the ordinary update
// shape cannot spell — while deals.RevertCorrection restores it perfectly. Ask
// only the generic evaluator and every correction this receipt exists to
// surface reads "cannot be undone", which is the exact defect the hardcoded
// false already was.
//
// The generic evaluator is still asked for everything else, from the SAME seam
// the restore route uses: the line offering an Undo and the write performing it
// must agree, and two constructions of one judgment drift.
type magicUndoJudge struct {
	seam RestoreSeam
	// corrections answers whether an audit row is a machine correction with its
	// own reversal. Nil is a real state — an installation wired without it
	// simply has no corrections to offer.
	corrections *deals.Store
}

// JudgeUndoPage answers a whole page.
//
// The workspace-level questions are asked ONCE here rather than per line: the
// object grant and the installation's overlay posture are properties of the
// caller and the workspace, and asking them a hundred times over is a hundred
// round trips for one answer. isOverlayUncached in particular opens its own
// transaction, so per-line it would take a second connection while this one
// holds the page — the deadlock shape every reader here is written to avoid.
func (j magicUndoJudge) JudgeUndoPage(
	ctx context.Context, tx pgx.Tx, subjects []magic.UndoSubject,
) (map[ids.UUID]*crmcontracts.MagicUndo, error) {
	out := make(map[ids.UUID]*crmcontracts.MagicUndo, len(subjects))
	if len(subjects) == 0 {
		return out, nil
	}
	// The object grant the WRITE takes. Without it a rep who owns the row but
	// holds no update permission is offered a control that can only 403 —
	// row authority alone is not the question the writer asks.
	posture := undoPosture{mayWrite: auth.Require(ctx, "deal", principal.ActionUpdate) == nil}
	external, err := j.seam.evaluator.ExternallyGoverned(ctx)
	if err != nil {
		return nil, err
	}
	posture.externallyGoverned = external
	for _, subject := range subjects {
		answer, err := j.judgeOne(ctx, tx, subject, posture)
		if err != nil {
			return nil, err
		}
		if answer != nil {
			out[subject.AuditID] = answer
		}
	}
	return out, nil
}

// undoPosture is what the page resolved once for every line on it: facts about
// the CALLER and the WORKSPACE rather than about any one entry.
type undoPosture struct {
	// mayWrite is the object grant the write itself takes. Row authority alone
	// is not the question: a rep who owns the row but holds no update
	// permission would be offered a control that can only 403.
	mayWrite bool
	// externallyGoverned marks a workspace whose records live in another
	// system, where no reversal this server makes can reach them.
	externallyGoverned bool
}

// judgeOne answers for one entry, or declines to answer at all.
//
// A nil answer means "not judged" and the page reads it as not-evaluated. That
// is deliberate for a subject this path does not serve: inventing a verdict for
// an entry it never looked at is how a greyed control acquires a reason that is
// not true.
func (j magicUndoJudge) judgeOne(
	ctx context.Context, tx pgx.Tx, subject magic.UndoSubject, posture undoPosture,
) (*crmcontracts.MagicUndo, error) {
	if !servesRecordType(subject.EntityType) {
		return magicRefusal(string(ReasonUnsupportedRecordType)), nil
	}
	if posture.externallyGoverned {
		return magicRefusal(string(ReasonNotRestorableByThisPath)), nil
	}
	if !posture.mayWrite {
		return magicRefusal(string(ReasonNotWritableByCaller)), nil
	}
	var row AuditRow
	err := tx.QueryRow(ctx, `
		SELECT id, entity_type, entity_id, action, before, after, occurred_at
		  FROM audit_log
		 WHERE id = $1`, subject.AuditID).
		Scan(&row.ID, &row.EntityType, &row.EntityID, &row.Action,
			&row.Before, &row.After, &row.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return magicRefusal(string(ReasonUnsupportedRecordType)), nil
	}
	if err != nil {
		return nil, err
	}
	// A scope MISS is an answer — no control for a record this reader cannot
	// change — while a broken read is not. Collapsing both into a silent "no"
	// would hide a failing gate behind a greyed button nobody questions.
	if err := j.seam.visible(ctx, tx, row.EntityType, row.EntityID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return magicRefusal(string(ReasonUnsupportedRecordType)), nil
		}
		return nil, err
	}
	served, err := privacy.HistoryServesEntry(ctx, tx, row.EntityType, row.EntityID, subject.AuditID)
	if err != nil {
		return nil, err
	}
	if !served {
		return magicRefusal(string(ReasonUnsupportedRecordType)), nil
	}
	// THE CORRECTION PATH FIRST. A machine correction has its own reversal, and
	// the generic evaluator refuses exactly those rows — so asking it first
	// would answer "cannot be undone" for the changes this receipt exists to
	// show.
	if answer, decided, err := j.judgeCorrection(ctx, tx, subject.AuditID); err != nil || decided {
		return answer, err
	}
	verdict, err := j.seam.evaluator.Evaluate(ctx, tx, row, Advisory)
	if err != nil {
		return nil, err
	}
	if verdict.Undoable {
		return magicOffer(subject.AuditID), nil
	}
	return magicRefusal(string(verdict.Reason)), nil
}

// judgeCorrection answers for a machine correction, and says whether it was one.
func (j magicUndoJudge) judgeCorrection(
	ctx context.Context, tx pgx.Tx, auditID ids.UUID,
) (*crmcontracts.MagicUndo, bool, error) {
	if j.corrections == nil {
		return nil, false, nil
	}
	correction, err := j.corrections.CorrectionForAudit(ctx, tx, auditID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if correction.Reversed() {
		return magicRefusal(string(ReasonAlreadyUndone)), true, nil
	}
	return magicOffer(auditID), true, nil
}

func magicOffer(auditID ids.UUID) *crmcontracts.MagicUndo {
	id := openapi_types.UUID(auditID)
	return &crmcontracts.MagicUndo{Undoable: true, AuditId: &id}
}

func magicRefusal(reason string) *crmcontracts.MagicUndo {
	return &crmcontracts.MagicUndo{Undoable: false, Reason: &reason}
}

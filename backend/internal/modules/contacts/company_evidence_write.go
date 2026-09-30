// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// evidenceWrite names what differs between the two evidence sidecars a human
// can correct or confirm — the facts table and the profile-field table. Both
// are written in the one shape writeEvidence spells; everything a caller has
// to supply is here, and a third sidecar would supply the same five things.
type evidenceWrite[T any] struct {
	// table is the sidecar being written, for the guarded patch and the audit row.
	table string
	// archived is the write's archived-row filter, declared here beside its own
	// table so the claim "this table has no archived_at" stays checkable at the
	// call site rather than being inherited from the shared writer.
	archived storekit.ArchivedFilter
	// changedKey names the claim in the emitted company.updated event.
	changedKey string
	// value is the corrected value, or nil for a confirmation — agreeing with a
	// claim changes who stands behind it, not what it says.
	value     *string
	ifVersion *int64
	// locate names WHICH row this write is about, and mints it for the verb
	// that creates one. Its id and nothing else: the writer locks that id
	// before anything about the row is read. The bool says the row did not
	// exist and this write minted it — there is then no prior state, and the
	// audit records a creation rather than an update against an image nobody
	// ever wrote.
	locate func(context.Context, pgx.Tx) (ids.UUID, bool, error)
	// readLocked returns the machine's claim in full, which becomes the audit
	// before-image. It runs UNDER the row lock, which is what makes the image
	// describe the state the patch below actually replaces.
	readLocked func(context.Context, pgx.Tx, ids.UUID) (evidenceRow, error)
	// canonical moves the corrected value out of the sidecar and onto the
	// record it describes. Nil for a claim that lives only in the sidecar and
	// so has nothing to keep in step.
	canonical func(context.Context, pgx.Tx) error
	// readAfter re-reads the written row as its wire shape. Nil for a removal,
	// which has no row left to read.
	readAfter func(context.Context, pgx.Tx) (T, error)
	// remove deletes the row instead of patching it, for the one verb that says
	// "this is not a fact about this company at all" — which a correction
	// cannot say, since it can only change what the claim states. The audit
	// before-image is the whole removed claim, so what was taken away stays
	// answerable.
	remove bool
}

// writeEvidence is the one way a human correction or confirmation reaches an
// evidence sidecar (PO-AC-N-2).
//
// The machine's proposal is NOT overwritten: evidence_snippet, source_url and
// confidence stay exactly as extracted, and the before image carries them into
// the audit trail. What changes is who now stands behind the value.
//
// Neither sidecar is archivable: each is deleted with the company it
// describes rather than retired on its own, which is why both declare
// NoArchiveColumn below.
func writeEvidence[T any](
	ctx context.Context, s *Store, companyID ids.CompanyID, w evidenceWrite[T],
) (T, error) {
	var out T
	// A claim is an assertion about the company, so it is the
	// company's own update grant that governs it — there is no separate
	// object to grant, and inventing one would let a role edit a company's
	// industry through its receipt while being denied it on the record.
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return out, err
	}
	// A confirmation names the human who gave it; a principal with no user
	// cannot confirm anything on anyone's behalf.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == (ids.UUID{}) {
		return out, fmt.Errorf(
			"confirming a claim records who agreed, and this call carries no user: %w",
			apperrors.ErrPermissionDenied)
	}

	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureCompanyWritable(ctx, tx, companyID); err != nil {
			return err
		}
		// The transaction's own clock, so every row this write stamps agrees
		// and a test can pin it without the store carrying a clock.
		var now time.Time
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
			return fmt.Errorf("read transaction time: %w", err)
		}
		id, created, err := w.locate(ctx, tx)
		if err != nil {
			return err
		}
		// The LOCK, before anything about the row is read.
		//
		// The before-image and the patch have to describe one row state, and
		// they did not: the read happened here and the lock was taken twenty
		// lines below, inside the patch. A concurrent write landing in that
		// window left an audit entry answering "what did it say before I fixed
		// it" with a value the fix never replaced — and, for a removal, naming
		// a claim other than the one that was deleted.
		lock, err := storekit.LockRow(ctx, tx, w.table, id, w.archived)
		if err != nil {
			return err
		}
		before, err := w.readLocked(ctx, tx, id)
		if err != nil {
			return err
		}
		// The caller's If-Match, decided against the version read under that
		// lock. A plain comparison rather than a CAS on the update: nothing can
		// move the version now, and LockRow has already answered not-found for
		// the row that is gone — which is the only other thing a CAS's second
		// query was there to tell apart.
		if w.ifVersion != nil && *w.ifVersion != before.Version {
			return apperrors.ErrVersionSkew
		}

		p, err := humanVerdictPatch(ctx, before, w.value, actor.UserID, now)
		if err != nil {
			return err
		}

		// The patch runs even for a removal, so the row a delete is about
		// carries the same provenance the audit entry below reports — and it
		// runs under the lock above rather than taking one of its own.
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		if w.remove {
			return removeEvidenceRow(ctx, tx, companyID, w, before)
		}
		if w.canonical != nil {
			if err := w.canonical(ctx, tx); err != nil {
				return err
			}
		}

		// A row this write minted has no before-image, and calling it an update
		// against the empty row we just inserted would put a state nobody ever
		// wrote into the audit trail — the one record that answers "what did it
		// say before I changed it". AuditEvent is the door for a write with no
		// prior state; Audit refuses an update carrying no before-image.
		//
		// Spelled inline rather than behind a helper: the audit and the emit
		// below are one obligation, and a helper holding only the audit half
		// puts them in separate functions where nothing local shows they travel
		// together — which is exactly what the write-shape gate reads.
		var auditID ids.UUID
		if created {
			auditID, err = storekit.AuditEvent(ctx, tx, "create", w.table, before.ID, p.After())
		} else {
			auditID, err = storekit.Audit(ctx, tx, "update", w.table,
				before.ID, before.auditImage(), p.After())
		}
		if err != nil {
			return fmt.Errorf("audit %s write: %w", w.table, err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID,
			crmcontracts.PublicEventCompanyUpdated{
				ChangedFields: map[string]any{w.changedKey: p.After()},
			}); err != nil {
			return fmt.Errorf("emit company.updated: %w", err)
		}

		out, err = w.readAfter(ctx, tx)
		return err
	})
	return out, err
}

// humanVerdictPatch is what a correction or a confirmation writes: the value
// when there is one, and in either case the provenance saying a human now
// stands behind the claim.
//
// The machine's own proposal is NOT in here. evidence_snippet, source_url and
// confidence stay exactly as extracted, which is what lets the before-image
// carry them into the audit trail rather than an answer overwriting them.
func humanVerdictPatch(
	ctx context.Context, before evidenceRow, value *string, by ids.UUID, now time.Time,
) (*storekit.Patch, error) {
	p := storekit.NewPatch()
	if value != nil {
		p.Set(auditKeyValue, before.Value, *value)
	}
	p.Set(auditKeySource, before.Source, CompanySourceHuman)
	p.Set(auditKeyVerifiedAt, before.VerifiedAt, now)
	p.Set(auditKeyVerifiedBy, before.VerifiedBy, by)
	// The row changes HANDS, not just provenance. Both enrichment upserts
	// decline to overwrite a row whose captured_by is a human, and they test
	// that column rather than `source` — so a verdict that moved source
	// alone was reclaimed by the next ordinary refresh, silently undoing
	// the correction a contact had just made.
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	p.Set(auditKeyCapturedBy, before.CapturedBy, capturedBy)
	return p, nil
}

// removeEvidenceRow deletes one sidecar row, after the writer above has taken
// the row lock and enforced any version precondition. What is left to it is the
// delete itself and the two records it owes: one audit row holding the whole
// removed claim as its before-image, and one event.
//
// `delete` is the honest audit action here because the row really is gone —
// the bytes are not kept under a flag — which is what the action's own
// migration reserves it for.
func removeEvidenceRow[T any](
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
	w evidenceWrite[T], before evidenceRow,
) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM `+pgx.Identifier{w.table}.Sanitize()+` WHERE id = $1`,
		before.ID); err != nil {
		return fmt.Errorf("delete %s row: %w", w.table, err)
	}
	auditID, err := storekit.Audit(ctx, tx, "delete", w.table,
		before.ID, before.auditImage(), nil)
	if err != nil {
		return fmt.Errorf("audit %s removal: %w", w.table, err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, companyID.UUID,
		crmcontracts.PublicEventCompanyUpdated{
			ChangedFields: map[string]any{w.changedKey: nil},
		}); err != nil {
		return fmt.Errorf("emit company.updated: %w", err)
	}
	return nil
}

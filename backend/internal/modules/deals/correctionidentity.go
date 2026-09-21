// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a machine correction IS, so that something can refer to one.
//
// The sweep used to write a deal's field and an audit row describing the change,
// and nothing else. With no row that is the correction, Undo could not name what
// it was reversing, rejection memory could not survive a reversal (it reads
// refused APPROVALS, and a restore writes none), and a replay after a crash
// could not recognise the effect it had already produced.
//
// This is not a second audit ledger. The audit row still owns the before/after
// images and this row points at it; what lives here is the lifecycle the audit
// cannot represent — whether the change has since been taken back, and by whom.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealCorrection is one committed machine correction and its lifecycle.
type DealCorrection struct {
	ID         ids.UUID
	DealID     ids.DealID
	AuditLogID ids.UUID
	Correction string
	Fields     []string
	AppliedAt  time.Time
	ReversedAt *time.Time
	ReversedBy *string
	Evidence   CorrectionEvidence
}

// Reversed reports whether this correction has already been taken back.
func (c DealCorrection) Reversed() bool { return c.ReversedAt != nil }

// CorrectionEvidence is the question a correction answered, in the vocabulary
// RefusalProbe already compares.
//
// Deliberately not the date. A proposed close date is today plus a stage-velocity
// offset, so it moves every calendar day and the standing date moves with it —
// an identity keyed on either recognises a correction for exactly one night,
// which is indistinguishable from no identity at all on every night after.
//
// FIELD-FOR-FIELD with RefusalProbe, and EvidenceOf's conversion depends on it.
// The two are not duplication to collapse: they ask one question of different
// records — the probe of a refused approval, this of a committed correction —
// and keeping the shapes identical means a change to either fails to compile
// rather than letting the two memories drift apart.
type CorrectionEvidence struct {
	RemainingOpenStages string
	Asking              string
	StandingCloseDate   string
}

// EvidenceOf reads the identity out of a staged proposal, so the row a
// correction writes and the probe a later pass compares are built from one
// source rather than two spellings of it.
func EvidenceOf(proposal CloseDateCorrection, standing *time.Time) CorrectionEvidence {
	return CorrectionEvidence(ProbeFor(proposal, standing))
}

// SameQuestionAs reports whether an earlier correction answered this question.
//
// The same judgment RefusalProbe makes about a refused approval, asked of a
// correction instead: the stage count and the question must match, and the deal
// must still be standing where that correction left it. A deal a contact has
// re-dated since is a different situation and may be asked about again.
func (e CorrectionEvidence) SameQuestionAs(earlier CorrectionEvidence) bool {
	if e.RemainingOpenStages == "" || earlier.RemainingOpenStages == "" {
		// A row written before this identity existed carries no stage count,
		// and two unknowns are not a match — reading them as one would let the
		// oldest correction silence every deal that ever reached it.
		return false
	}
	if e.Asking == "" || earlier.Asking == "" {
		return false
	}
	return e.RemainingOpenStages == earlier.RemainingOpenStages &&
		e.Asking == earlier.Asking &&
		e.StandingCloseDate == earlier.StandingCloseDate
}

// recordCorrection writes the lifecycle row for a correction that has just
// committed, inside that same transaction.
//
// Inside it, because a correction whose row landed in a second transaction can
// be interrupted between the two — and then the deal has changed while nothing
// records that it did, which is the state this table exists to prevent.
//
// A conflict on the audit row is not an error: it means this exact change is
// already recorded, which is what a replay looks like. The existing row is
// returned so the caller settles against the correction that really happened.
func recordCorrection(
	ctx context.Context, tx pgx.Tx, in DealCorrection, runID *ids.UUID,
) (DealCorrection, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return DealCorrection{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return DealCorrection{}, err
	}
	var out DealCorrection
	err = tx.QueryRow(ctx, `
		INSERT INTO deal_correction
		    (deal_id, run_id, audit_log_id, correction, fields,
		     remaining_open_stages, asking, standing_close_date, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (audit_log_id) DO UPDATE SET updated_at = now()
		RETURNING id, deal_id, audit_log_id, correction, fields, applied_at,
		          reversed_at, reversed_by,
		          remaining_open_stages, asking, standing_close_date`,
		in.DealID, runID, in.AuditLogID, in.Correction, in.Fields,
		in.Evidence.RemainingOpenStages, in.Evidence.Asking,
		in.Evidence.StandingCloseDate, capturedBy).
		Scan(&out.ID, &out.DealID, &out.AuditLogID, &out.Correction, &out.Fields,
			&out.AppliedAt, &out.ReversedAt, &out.ReversedBy,
			&out.Evidence.RemainingOpenStages, &out.Evidence.Asking,
			&out.Evidence.StandingCloseDate)
	if err != nil {
		return DealCorrection{}, fmt.Errorf("deals: recording the correction: %w", err)
	}
	return out, nil
}

// CorrectionForAudit finds the correction an audit row records, if it is one.
//
// This is what lets Undo tell a machine correction from an ordinary edit: the
// generic restore path cannot put a sweep's change back (a past close date is
// refused by the update shape, and close_date_provisional is not writable
// through it at all), so a correction takes the reversal path instead.
//
// Returns apperrors.ErrNotFound for an audit row that is not a correction,
// which is the ordinary case for every human edit.
func (s *Store) CorrectionForAudit(ctx context.Context, tx pgx.Tx, auditID ids.UUID) (DealCorrection, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return DealCorrection{}, err
	}
	var out DealCorrection
	err := tx.QueryRow(ctx, `
		SELECT id, deal_id, audit_log_id, correction, fields, applied_at,
		       reversed_at, reversed_by,
		       remaining_open_stages, asking, standing_close_date
		  FROM deal_correction
		 WHERE audit_log_id = $1`, auditID).
		Scan(&out.ID, &out.DealID, &out.AuditLogID, &out.Correction, &out.Fields,
			&out.AppliedAt, &out.ReversedAt, &out.ReversedBy,
			&out.Evidence.RemainingOpenStages, &out.Evidence.Asking,
			&out.Evidence.StandingCloseDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return DealCorrection{}, apperrors.ErrNotFound
	}
	if err != nil {
		return DealCorrection{}, fmt.Errorf("deals: reading the correction: %w", err)
	}
	// The object grant alone is not enough: a correction names a deal, so
	// handing one back is a read of that deal. Without this a caller with
	// deal:read but no scope over the row learns it exists — and learns what a
	// machine did to it — from a lookup keyed on an audit id.
	if err := auth.EnsureVisible(ctx, tx, dealTable, out.DealID.UUID); err != nil {
		return DealCorrection{}, err
	}
	return out, nil
}

// reversedCorrections is the memory an Undo leaves behind: the questions a
// contact has already answered by taking a correction back.
//
// The gap this closes is the whole reason the row exists. Existing rejection
// memory reads REFUSED APPROVALS, and a reversal writes no approval — a rep
// could undo a correction and watch the same one reappear the next night, their
// answer lasting exactly until the next sweep.
func reversedCorrections(ctx context.Context, tx pgx.Tx, dealID ids.DealID) ([]CorrectionEvidence, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT remaining_open_stages, asking, standing_close_date
		  FROM deal_correction
		 WHERE deal_id = $1 AND reversed_at IS NOT NULL`, dealID)
	if err != nil {
		return nil, fmt.Errorf("deals: reading reversed corrections: %w", err)
	}
	defer rows.Close()
	var out []CorrectionEvidence
	for rows.Next() {
		var e CorrectionEvidence
		if err := rows.Scan(&e.RemainingOpenStages, &e.Asking, &e.StandingCloseDate); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReversalAnsweredThis reports whether a contact has already taken back a
// correction answering this same question.
//
// Exported for the approval path, which is a SECOND door onto the same write:
// close_date_correction is in approvals.AutoApplyKinds, so a rep with autonomy
// on has staged confirms redeemed unattended. The staging's own memory is read
// when the card is raised; a reversal landing between then and the redemption
// would otherwise reapply the very change that reversal undid.
//
// The version pin makes that window small — a reversal writes the deal, so a
// pinned redemption loses the compare — but the pin is a property of the KIND's
// staging configuration, and a memory that only holds while a configuration
// elsewhere stays put is not one anybody can rely on.
func (s *Store) ReversalAnsweredThis(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, asking CorrectionEvidence,
) (bool, error) {
	earlier, err := reversedCorrections(ctx, tx, dealID)
	if err != nil {
		return false, err
	}
	for _, e := range earlier {
		if asking.SameQuestionAs(e) {
			return true, nil
		}
	}
	return false, nil
}

// markReversed stamps a correction as taken back, in the transaction that
// restores the deal's fields.
//
// The `reversed_at IS NULL` predicate is the concurrency guard: two Undo calls
// racing on one correction both issue it, exactly one matches, and the loser
// affects nothing rather than stamping a second reversal over the first.
// Reporting that to the caller as "already reversed" is what makes a repeated
// Undo idempotent instead of a toggle.
func markReversed(
	ctx context.Context, tx pgx.Tx, correctionID, reversalAuditID ids.UUID, by string,
) (bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE deal_correction
		   SET reversed_at = now(), reversed_by = $2, reversal_audit_id = $3,
		       updated_at = now(), version = version + 1
		 WHERE id = $1 AND reversed_at IS NULL`, correctionID, by, reversalAuditID)
	if err != nil {
		return false, fmt.Errorf("deals: marking the correction reversed: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// movedFields names the columns a patch actually changed, in a stable order so
// two identical corrections record identical rows.
func movedFields(patch *storekit.Patch) []string {
	moved := patch.Moved()
	fields := make([]string, 0, len(moved))
	for column := range moved {
		fields = append(fields, column)
	}
	sort.Strings(fields)
	return fields
}

// reversedSameQuestion reports whether a contact has already taken back a
// correction that answered this same question.
//
// The comparison is CorrectionEvidence's, not the date's: a reversal is an
// answer to the reasoning ("this deal has N stages left, so push it out by a
// stage-worth of the usual pace"), and that reasoning is the same tomorrow. It
// stops being the same when the deal advances a stage or a contact puts their
// own date on it — then the situation is genuinely different, and asking again
// is right.
func (c *CloseDateCorrector) reversedSameQuestion(
	ctx context.Context, dealID ids.DealID, asking CorrectionEvidence,
) (bool, error) {
	var found bool
	err := c.db.Tx(ctx, func(tx pgx.Tx) error {
		earlier, err := reversedCorrections(ctx, tx, dealID)
		if err != nil {
			return err
		}
		for _, e := range earlier {
			if asking.SameQuestionAs(e) {
				found = true
				return nil
			}
		}
		return nil
	})
	return found, err
}

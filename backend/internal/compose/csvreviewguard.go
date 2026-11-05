// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Ensure lands one row: created the first time, updated when the file has
// since changed, unchanged when it has not. It skips a row the importer may
// not create, then a row with a cell the store would refuse. A contact whose
// company link finds no single company follows the run's policy.
func (w *csvWriters) Ensure(ctx context.Context, object string, row migration.Row) (migration.EnsureResult, error) {
	if err := w.reviewSource(ctx, row); err != nil {
		return migration.EnsureResult{}, err
	}
	if w.reviewRows != nil {
		var landed migration.EnsureResult
		err := database.WithWorkspaceSnapshot(ctx, w.pool, func(ctx context.Context) error {
			replayed, err := w.lockReviewedTarget(ctx, row)
			if err != nil {
				return err
			}
			if replayed {
				landed.Unchanged = true
				return nil
			}
			landed, err = w.ensureReviewedRow(ctx, object, row)
			return err
		})
		return landed, err
	}
	return w.ensureReviewedRow(ctx, object, row)
}

func (w *csvWriters) ensureReviewedRow(ctx context.Context, object string, row migration.Row) (migration.EnsureResult, error) {
	if err := w.validateReviewedLinks(ctx, row.ExternalID); err != nil {
		return migration.EnsureResult{}, err
	}
	if err := w.validateReviewedDisposition(ctx, row); err != nil {
		return migration.EnsureResult{}, err
	}
	landed, err := w.ensureOne(ctx, object, row)
	if err == nil && landed.Skipped {
		w.links.markRefused(object, row.ExternalID)
	}
	return landed, err
}

func (w *csvWriters) lockReviewedTarget(ctx context.Context, row migration.Row) (bool, error) {
	expected, found := w.reviewRows[row.ExternalID]
	if !found {
		return false, apperrors.ErrVersionSkew
	}
	if expected.TargetID == "" {
		return false, nil
	}
	id, err := ids.Parse(expected.TargetID)
	if err != nil {
		return false, apperrors.ErrVersionSkew
	}
	// The ambient row transaction also holds tag and relationship writes.
	replayed := false
	err = database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		table, err := w.lockReviewedImportRow(ctx, tx, id)
		if err != nil {
			return err
		}
		err = storekit.EnsureIfMatch(ctx, tx, table, id, &expected.TargetVersion)
		if !errors.Is(err, apperrors.ErrVersionSkew) {
			return err
		}
		replayed, err = w.reviewedRowWasApplied(ctx, tx, row, id)
		if err != nil {
			return err
		}
		if !replayed {
			return apperrors.ErrVersionSkew
		}
		return nil
	})
	return replayed, err
}

func (w *csvWriters) reviewedRowWasApplied(ctx context.Context, tx pgx.Tx, row migration.Row, id ids.UUID) (bool, error) {
	var ownLastWrite bool
	err := tx.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT batch_id = @run_id FROM audit_log
			WHERE entity_type = @object AND entity_id = @id
			ORDER BY occurred_at DESC, id DESC LIMIT 1
		), false)`, pgx.StrictNamedArgs{importRunArgument: w.runID, objectSourceField: w.object, "id": id}).Scan(&ownLastWrite)
	if err != nil || !ownLastWrite {
		return false, err
	}
	// A row's fields and satellites commit together, so its own final audit
	// plus unchanged native intent proves a completed landing after a lost checkpoint.
	outcome, err := w.Predict(ctx, row)
	return outcome == predictUnchanged, err
}

func (b *bundleWriters) bindReviewPlan(report *migration.Report) error {
	if report == nil || report.ReviewPlan == nil || !report.ReviewPlan.Enforced {
		return nil
	}
	plan := report.ReviewPlan
	if plan.Version != 1 || !plan.Complete || len(plan.Rows) > migration.ReviewRowLimit {
		return apperrors.ErrVersionSkew
	}
	b.reviewPlan = plan
	for _, part := range b.parts {
		part.reviewRows = map[string]migration.ReviewRow{}
	}
	for _, row := range plan.Rows {
		part, found := b.parts[row.Object]
		if !found {
			return apperrors.ErrVersionSkew
		}
		if _, duplicate := part.reviewRows[row.ExternalID]; duplicate {
			return apperrors.ErrVersionSkew
		}
		part.reviewRows[row.ExternalID] = row
	}
	return nil
}

func (w *csvWriters) reviewSource(ctx context.Context, row migration.Row) error {
	if w.reviewRows == nil {
		return nil
	}
	expected, found := w.reviewRows[row.ExternalID]
	if !found {
		return apperrors.ErrVersionSkew
	}
	current, err := json.Marshal(row.Fields)
	if err != nil {
		return err
	}
	reviewed, err := json.Marshal(expected.Values)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, reviewed) || row.OwnerExternalID != expected.OwnerExternalID || !row.LastSyncedAt.Equal(expected.LastSyncedAt) {
		return apperrors.ErrVersionSkew
	}
	if expected.Outcome != importRowSkip {
		return w.reviewCheckedSource(ctx, row, expected)
	}
	if expected.Outcome == importRowSkip {
		outcome, err := w.Predict(ctx, row)
		if err != nil {
			return err
		}
		if outcome != predictUnwritable && outcome != predictCollidesSkipped {
			return apperrors.ErrVersionSkew
		}
	}
	return nil
}

func (b *bundleWriters) validateReviewSource(ctx context.Context, source migration.Source) error {
	if b.reviewPlan == nil {
		return nil
	}
	objects := source.Objects()
	if len(objects) != len(b.parts) {
		return apperrors.ErrVersionSkew
	}
	seenObjects := map[string]bool{}
	for _, object := range objects {
		if seenObjects[object] {
			return apperrors.ErrVersionSkew
		}
		seenObjects[object] = true
		part, err := b.part(object)
		if err != nil {
			return err
		}
		if part.reviewRows == nil {
			continue
		}
		if err := part.validateReviewRows(ctx, source, object); err != nil {
			return err
		}
	}
	associations, err := source.Associations(ctx)
	if err != nil {
		return err
	}
	if !slices.Equal(associations, b.reviewPlan.Associations) {
		return apperrors.ErrVersionSkew
	}
	return nil
}

func (w *csvWriters) reviewedTarget(external string, id ids.UUID) error {
	if w.reviewRows == nil {
		return nil
	}
	expected, found := w.reviewRows[external]
	if !found || expected.TargetID != id.String() || expected.TargetVersion <= 0 {
		return apperrors.ErrVersionSkew
	}
	return nil
}

func (w *csvWriters) reviewedBinding(ctx context.Context, row migration.Row, id ids.UUID) (bool, error) {
	if w.reviewRows == nil {
		return false, nil
	}
	expected, found := w.reviewRows[row.ExternalID]
	if !found {
		return false, apperrors.ErrVersionSkew
	}
	if expected.TargetID != "" {
		return false, w.reviewedTarget(row.ExternalID, id)
	}
	binding, found, err := w.bindingOf(ctx, row.ExternalID)
	if err != nil {
		return false, err
	}
	if !found || binding.RunID != w.runID || binding.NativeID != id {
		return false, apperrors.ErrVersionSkew
	}
	outcome, err := w.Predict(ctx, row)
	if err != nil {
		return false, err
	}
	if outcome != predictUnchanged {
		return false, apperrors.ErrVersionSkew
	}
	return true, nil
}

func (w *csvWriters) reviewedVersion(id ids.UUID) *int64 {
	for _, row := range w.reviewRows {
		if row.TargetID == id.String() {
			return new(row.TargetVersion)
		}
	}
	return nil
}

func (w *csvWriters) validateReviewedDisposition(ctx context.Context, row migration.Row) error {
	if w.reviewRows == nil {
		return nil
	}
	expected := w.reviewRows[row.ExternalID]
	predicted, err := w.Predict(ctx, row)
	if err != nil {
		return err
	}
	disposition, err := reviewDisposition(predicted)
	if err != nil {
		return err
	}
	if disposition == expected.Outcome {
		return nil
	}
	if expected.TargetID == "" && predicted == predictUnchanged {
		binding, found, err := w.bindingOf(ctx, row.ExternalID)
		if err != nil {
			return err
		}
		if found && binding.RunID == w.runID {
			return nil
		}
	}
	return apperrors.ErrVersionSkew
}

func (w *csvWriters) validateReviewRows(ctx context.Context, source migration.Source, object string) error {
	seen := map[string]bool{}
	for offset := 0; ; offset += importPredictPage {
		rows, err := source.Rows(ctx, object, offset, importPredictPage)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if seen[row.ExternalID] {
				return apperrors.ErrVersionSkew
			}
			seen[row.ExternalID] = true
			if err := w.reviewSource(ctx, row); err != nil {
				return err
			}
		}
		if len(rows) < importPredictPage {
			break
		}
	}
	if len(seen) != len(w.reviewRows) {
		return apperrors.ErrVersionSkew
	}
	return nil
}

func (w *csvWriters) reviewCheckedSource(ctx context.Context, row migration.Row, expected migration.ReviewRow) error {
	typed, err := w.reviewCheckedCells(ctx, row)
	if err != nil {
		return err
	}
	current, err := json.Marshal(typed)
	if err != nil {
		return err
	}
	reviewed, err := json.Marshal(expected.CheckedValues)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, reviewed) {
		return apperrors.ErrVersionSkew
	}
	return nil
}

func (w *csvWriters) lockReviewedImportRow(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	switch w.object {
	case migration.ObjectCompany:
		_, err := w.contacts.LockCompanyForBulkTx(ctx, tx, ids.From[ids.CompanyKind](id))
		return tableCompany, err
	case migration.ObjectContact:
		_, err := w.contacts.LockContactForBulkTx(ctx, tx, ids.From[ids.ContactKind](id))
		return tableContact, err
	case migration.ObjectLead:
		_, err := w.contacts.LockLeadForBulkTx(ctx, tx, ids.From[ids.LeadKind](id))
		return tableLead, err
	case migration.ObjectDeal:
		_, err := w.deals.LockDealForBulkTx(ctx, tx, ids.From[ids.DealKind](id))
		return tableDeal, err
	case migration.ObjectActivity:
		return tableActivity, activities.LockActivityLive(ctx, tx, ids.From[ids.ActivityKind](id))
	default:
		return "", apperrors.ErrVersionSkew
	}
}

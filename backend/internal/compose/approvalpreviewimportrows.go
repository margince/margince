// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (r approvalPreviewReader) importRows(ctx context.Context, run migration.Run, out *crmcontracts.ApprovalPreview) ([]crmcontracts.ApprovalPreviewImportRow, error) {
	plan := run.Report.ReviewPlan
	if run.Actor == nil || run.Actor.ID != plan.ActorID {
		return nil, apperrors.ErrVersionSkew
	}
	creator, err := (storedActor{authority: identity.NewService(r.db.Pool())}).rebuild(ctx, *run.Actor, principal.ScopeWrite)
	if err != nil {
		return nil, err
	}
	execCtx := principal.WithActor(ctx, creator)
	source, err := importSourceFromReview(*plan, *run.Mapping)
	if err != nil {
		return nil, err
	}
	writers, err := newBundleWriters(execCtx, r.db, run.ID, run.Mapping, source)
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, name := range writers.tags.missing {
		details = append(details, previewDetail("import_create_tag", name))
	}
	out.Details = &details
	rows := make([]crmcontracts.ApprovalPreviewImportRow, 0, len(plan.Rows))
	index := 0
	for _, object := range source.Objects() {
		writer := writers.parts[object]
		if err := writer.preloadBindings(execCtx); err != nil {
			return nil, err
		}
		for _, row := range source.rows[object] {
			outcome, _, err := writer.predictRow(execCtx, row)
			if err != nil {
				return nil, err
			}
			current, err := writer.reviewRow(execCtx, row, outcome)
			if err != nil {
				return nil, err
			}
			expected := plan.Rows[index]
			if err := importReviewRowMatches(plan, index, current, expected); err != nil {
				return nil, err
			}
			projected, err := r.importRow(ctx, execCtx, writer, row, expected, creator)
			if err != nil {
				return nil, err
			}
			rows = append(rows, projected)
			writer.markPredictedRow(row, outcome)
			index++
		}
	}
	return rows, nil
}

func (r approvalPreviewReader) importRow(ctx, execCtx context.Context, writer *csvWriters, row migration.Row, reviewed migration.ReviewRow, creator principal.Principal) (crmcontracts.ApprovalPreviewImportRow, error) {
	out := crmcontracts.ApprovalPreviewImportRow{
		Object: reviewed.Object, SourceKey: reviewed.ExternalID, Line: reviewed.Line,
		Outcome: crmcontracts.ApprovalPreviewImportRowOutcome(reviewed.Outcome),
		Changes: []crmcontracts.ApprovalPreviewChange{}, Details: []crmcontracts.ApprovalPreviewDetail{}, Effects: []string{},
	}
	if reviewed.Outcome == importRowSkip {
		return out, nil
	}
	linkDetails, err := r.importLinkDetails(ctx, reviewed.Links)
	if err != nil {
		return out, err
	}
	typed, invalid, _ := writer.landingCells(row)
	if invalid != nil {
		return out, apperrors.ErrVersionSkew
	}
	link, err := writer.linkProblemOf(execCtx, row)
	if err != nil {
		return out, err
	}
	typed = link.into(typed)
	if reviewed.TargetID == "" {
		out.Details, out.Effects, err = r.importCreationDetails(ctx, execCtx, writer, row, typed, creator)
		if err != nil {
			return out, err
		}
		out.Details = omitPendingImportDefaults(out.Details, reviewed.Links)
		out.Details = append(out.Details, linkDetails...)
		err = r.importRowTags(ctx, execCtx, writer, row, true, &out)
		return out, err
	}
	target, err := writer.reviewTarget(ctx, row)
	if err != nil {
		return out, err
	}
	value, err := r.referenceValue(ctx, writer.object+"_id", target.String())
	if err != nil {
		return out, err
	}
	out.Target = &value
	out.Details = append(out.Details, linkDetails...)
	current, err := writer.read(ctx, target)
	if err != nil {
		return out, err
	}
	out.Changes, err = r.importFieldChanges(ctx, writer.object, current, textFields(row.Fields), typed)
	if err == nil {
		err = r.importPendingCompanyChange(ctx, writer.object, current, reviewed.Links, &out)
	}
	if err == nil {
		err = r.importRowTags(ctx, execCtx, writer, row, false, &out)
	}
	return out, err
}

func (r approvalPreviewReader) importFieldChanges(ctx context.Context, object string, current []byte, fields map[string]string, typed map[string]any) ([]crmcontracts.ApprovalPreviewChange, error) {
	changed, err := changedFields(current, fields, typed)
	if err != nil {
		return nil, err
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(current, &stored); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(changed))
	for name := range changed {
		names = append(names, name)
	}
	sort.Strings(names)
	changes := make([]crmcontracts.ApprovalPreviewChange, 0, len(names))
	for _, field := range names {
		if isCreateOnly(object, field) {
			continue
		}
		change, err := r.importFieldChange(ctx, object, field, changed[field], stored, fields, typed)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func importPreviewField(object, field string) (string, string) {
	display := field
	if object == migration.ObjectDeal {
		if renamed, ok := dealStoredColumns[field]; ok {
			display = renamed
		}
	}
	if object == migration.ObjectActivity {
		if renamed, ok := activityStoredColumns[field]; ok {
			display = renamed
		}
	}
	if field == csvTargetOwner {
		display = bulkParamOwner
	}
	if object == migration.ObjectLead && field == leadTargetStatus {
		display = "lead_status"
	}
	if object == migration.ObjectDeal && field == csvTargetCompanyRef {
		display = refColCompany
	}
	if strings.HasPrefix(field, "address.") {
		display = strings.ReplaceAll(field, ".", "_")
	}
	return display, importMaskField(object, field, display)
}

func importReviewRowMatches(plan *migration.ReviewPlan, index int, current, expected migration.ReviewRow) error {
	if current.Object != expected.Object || current.ExternalID != expected.ExternalID || current.Outcome != expected.Outcome ||
		current.TargetID != expected.TargetID || current.TargetVersion != expected.TargetVersion {
		return apperrors.ErrVersionSkew
	}
	currentChecked, err := json.Marshal(current.CheckedValues)
	if err != nil {
		return err
	}
	expectedChecked, err := json.Marshal(expected.CheckedValues)
	if err != nil {
		return err
	}
	if !bytes.Equal(currentChecked, expectedChecked) {
		return apperrors.ErrVersionSkew
	}
	comparison := *plan
	comparison.Rows = slices.Clone(plan.Rows)
	for i := range comparison.Rows {
		comparison.Rows[i].Links = slices.Clone(comparison.Rows[i].Links)
	}
	comparison.Rows[index] = current
	settleReviewLinks(&comparison)
	if !comparison.Complete || !slices.Equal(comparison.Rows[index].Links, expected.Links) {
		return apperrors.ErrVersionSkew
	}
	return nil
}

func (r approvalPreviewReader) importFieldChange(ctx context.Context, object, field, changed string, stored map[string]json.RawMessage, fields map[string]string, typed map[string]any) (crmcontracts.ApprovalPreviewChange, error) {
	displayField, maskField := importPreviewField(object, field)
	incoming := any(canonicalFor(field, changed))
	if checked, found := typed[field]; found {
		incoming = checked
	}
	if err := creationFieldsVisible(ctx, object, map[string]any{maskField: incoming}); err != nil {
		return crmcontracts.ApprovalPreviewChange{}, err
	}
	raw, err := json.Marshal(incoming)
	if err != nil {
		return crmcontracts.ApprovalPreviewChange{}, err
	}
	beforeRaw := storedValue(stored, field)
	if object == migration.ObjectDeal && field == csvTargetCompanyRef {
		beforeRaw = stored[refColCompany]
	}
	if len(beforeRaw) == 0 {
		beforeRaw = json.RawMessage("null")
	}
	before, err := r.value(ctx, displayField, beforeRaw)
	if err != nil {
		return crmcontracts.ApprovalPreviewChange{}, err
	}
	after, err := r.value(ctx, displayField, raw)
	if err != nil {
		return crmcontracts.ApprovalPreviewChange{}, err
	}
	if displayField == acceptFieldAmountMinor {
		if err := r.importMoneyValues(ctx, displayField, &before, &after, stored, fields); err != nil {
			return crmcontracts.ApprovalPreviewChange{}, err
		}
	}
	return crmcontracts.ApprovalPreviewChange{Field: displayField, Before: before, After: after}, nil
}

func importMaskField(object, field, display string) string {
	mask := display
	if object == migration.ObjectLead && field == leadTargetStatus {
		mask = previewFieldStatus
	}
	if strings.HasPrefix(field, "address.") {
		mask = addressField
	}
	if object == migration.ObjectCompany && field == fieldDomain {
		mask = previewFieldDomains
	}
	if object == migration.ObjectContact {
		switch field {
		case fieldEmail:
			mask = previewFieldEmails
		case fieldPhone:
			mask = previewFieldPhones
		case fieldLinkedIn:
			mask = previewFieldSocial
		}
	}
	return mask
}

func (r approvalPreviewReader) importMoneyValues(ctx context.Context, displayField string, before, after *crmcontracts.ApprovalPreviewValue, stored map[string]json.RawMessage, fields map[string]string) error {
	if err := r.moneyValue(ctx, displayField, before, stored); err != nil {
		return err
	}
	currency := stored[dealTargetCurrency]
	if value, found := fields[dealTargetCurrency]; found {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		currency = encoded
	}
	return r.moneyValue(ctx, displayField, after, map[string]json.RawMessage{dealTargetCurrency: currency})
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// previewEffect answers ctx bound to the principal a proposal's effect runs
// as, and whether that principal is an agent rather than the caller.
type previewEffect func(context.Context) (context.Context, bool, error)

func (r approvalPreviewReader) read(ctx context.Context, approval crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
	return r.readAs(ctx, approval, approvals.EffectContext)
}

// readAs is the one path every kind's preview takes. An agent's proposal is
// built as the agent, whose retry runs it. Only a viewer who can open every
// record it touches sees it, and with that viewer's field masks.
func (r approvalPreviewReader) readAs(ctx context.Context, approval crmcontracts.Approval, effectOf previewEffect) (*crmcontracts.ApprovalPreview, error) {
	if approval.Status != crmcontracts.ApprovalStatusPending {
		return nil, nil //nolint:nilnil // Terminal decisions retain their recorded context.
	}
	effect, asAgent, err := effectOf(ctx)
	if previewUnavailable(err) {
		// An agent that can no longer call cannot spend the approval.
		return &crmcontracts.ApprovalPreview{Version: 1, State: previewUnavailableState, Changes: []crmcontracts.ApprovalPreviewChange{}, ReviewerReason: previewReviewer(ctx, approval)}, nil
	}
	if err != nil {
		return nil, err
	}
	r.touched = &previewTouched{}
	preview, err := r.build(effect, approval)
	if err != nil || preview == nil {
		return preview, err
	}
	if asAgent {
		if preview.ReviewerReason != "" {
			preview.ReviewerReason = previewReviewer(ctx, approval)
		}
		open, err := r.viewerOpensPreview(ctx, approval, preview)
		if err != nil {
			return nil, err
		}
		if !open {
			withholdPreview(preview)
			return preview, nil
		}
	}
	return preview, maskForViewer(ctx, approval, preview)
}

// previewTouched collects, by type, records a build read and names only as
// text, so the sight check covers them as well as the referenced ones.
type previewTouched struct{ byType map[string][]ids.UUID }

func (t *previewTouched) add(entityType string, id ids.UUID) {
	if id.IsZero() {
		return
	}
	if t.byType == nil {
		t.byType = map[string][]ids.UUID{}
	}
	t.byType[entityType] = append(t.byType[entityType], id)
}

func (r approvalPreviewReader) touch(byType map[string][]ids.UUID) {
	if r.touched == nil {
		return
	}
	for entityType, want := range byType {
		for _, id := range want {
			r.touched.add(entityType, id)
		}
	}
}

// previewReferences lists the target and every record the preview refers to.
func previewReferences(approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview, touched *previewTouched) *previewTouched {
	out := &previewTouched{}
	if touched != nil {
		for entityType, want := range touched.byType {
			for _, id := range want {
				out.add(entityType, id)
			}
		}
	}
	if approval.TargetEntityType != nil && approval.TargetEntityId != nil {
		out.add(*approval.TargetEntityType, ids.UUID(*approval.TargetEntityId))
	}
	value := func(v *crmcontracts.ApprovalPreviewValue) {
		if v != nil && v.EntityType != nil && v.EntityId != nil {
			out.add(*v.EntityType, ids.UUID(*v.EntityId))
		}
	}
	changes := func(list []crmcontracts.ApprovalPreviewChange) {
		for i := range list {
			value(&list[i].Before)
			value(&list[i].After)
		}
	}
	details := func(list []crmcontracts.ApprovalPreviewDetail) {
		for i := range list {
			value(&list[i].Value)
		}
	}
	changes(preview.Changes)
	if preview.Details != nil {
		details(*preview.Details)
	}
	if preview.ImportRows != nil {
		for i := range *preview.ImportRows {
			row := &(*preview.ImportRows)[i]
			value(row.Target)
			changes(row.Changes)
			details(row.Details)
		}
	}
	if preview.Evidence != nil {
		for _, item := range *preview.Evidence {
			if item.SourceType != nil && item.SourceId != nil {
				out.add(string(*item.SourceType), ids.UUID(*item.SourceId))
			}
		}
	}
	return out
}

func (r approvalPreviewReader) viewerOpensPreview(ctx context.Context, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview) (bool, error) {
	open, err := r.viewerReadsEvery(ctx, previewReferences(approval, preview, r.touched).byType)
	if err != nil || !open || preview.Files == nil || len(*preview.Files) == 0 {
		return open, err
	}
	files := make([]ids.UUID, 0, len(*preview.Files))
	for _, file := range *preview.Files {
		files = append(files, ids.UUID(file.Id))
	}
	if _, err := r.activities.PreviewSendAttachments(ctx, files); err != nil {
		if previewUnavailable(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// viewerReadsEvery answers whether the caller can open every record named. An
// activity whose content they may not read counts as one they cannot open, and
// a type nothing can read for them counts as unreadable.
func (r approvalPreviewReader) viewerReadsEvery(ctx context.Context, touched map[string][]ids.UUID) (bool, error) {
	for entityType, want := range touched {
		labels, named, err := r.names.named(ctx, entityType, want)
		if err != nil {
			return false, err
		}
		for _, id := range want {
			if named {
				if strings.TrimSpace(labels[id]) == "" {
					return false, nil
				}
				continue
			}
			if open, err := r.viewerOpens(ctx, entityType, id); err != nil || !open {
				return false, err
			}
		}
	}
	return true, nil
}

// viewerOpens reads one record no label covers through the store that owns it.
func (r approvalPreviewReader) viewerOpens(ctx context.Context, entityType string, id ids.UUID) (bool, error) {
	var err error
	switch entityType {
	case string(recordTypeUserAdmin):
		// A seat under administration opens as its member's name does.
		return r.viewerReadsEvery(ctx, map[string][]ids.UUID{entityUser: {id}})
	case string(recordTypeImportRun):
		_, err = importHandlers{db: r.db}.stagedFor(ctx, openapi_types.UUID(id))
	case tableSignal:
		_, err = r.signals.GetSignal(ctx, ids.From[ids.SignalKind](id), storekit.LiveOnly)
	default:
		_, err = r.records.Read(ctx, datasource.EntityRef{Type: datasource.EntityType(entityType), ID: id})
	}
	if _, unsupported := errors.AsType[*datasource.UnsupportedEntityError](err); unsupported || previewUnavailable(err) {
		return false, nil
	}
	return err == nil, err
}

// withholdPreview leaves a preview that shows nothing of the plan.
func withholdPreview(preview *crmcontracts.ApprovalPreview) {
	preview.State = approvals.PreviewNeedsWiderSight
	preview.Changes = []crmcontracts.ApprovalPreviewChange{}
	preview.Evidence = &[]crmcontracts.ApprovalEvidence{}
	preview.Details, preview.Effects, preview.Files, preview.ImportRows = nil, nil, nil, nil
	preview.Conflicts, preview.LifecycleEvidence, preview.CurrentVersion = nil, nil, nil
}

// previewDetailSources names the record fields a plain-text detail repeats
// under another key, so a mask on the field reaches the detail.
var previewDetailSources = map[string][]string{
	"promotion_contact_name":     {fieldFullName},
	"promotion_contact_email":    {fieldEmail},
	"promotion_contact_phone":    {fieldPhone, previewFieldPhones},
	"promotion_contact_title":    {fieldTitle},
	"promotion_contact_source":   {fieldSource},
	"promotion_contact_owner":    {bulkParamOwner},
	"promotion_deal_owner":       {bulkParamOwner},
	"promotion_company_industry": {fieldIndustry},
	"promotion_company_proposed": {companyNameField},
}

// previewMaskedObjects names, per kind, the objects besides the target whose
// fields its details repeat: a promotion shows the contact the lead becomes.
var previewMaskedObjects = map[string][]string{
	operationPromoteLead: {string(recordTypeContact)},
}

const previewFieldPhones = "phones"

// maskForViewer withholds each change and detail repeating a field the
// viewer's role masks on the target. An import row is masked by its own object.
func maskForViewer(ctx context.Context, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview) error {
	hasDetails := preview.Details != nil && len(*preview.Details) > 0
	hasRows := preview.ImportRows != nil && len(*preview.ImportRows) > 0
	if len(preview.Changes) == 0 && !hasDetails && !hasRows {
		return nil
	}
	viewer, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	var masked []string
	if approval.TargetEntityType != nil {
		masked = auth.MaskedFields(viewer, *approval.TargetEntityType, true)
	}
	for _, object := range previewMaskedObjects[approval.Kind] {
		masked = append(masked, auth.MaskedFields(viewer, object, false)...)
	}
	maskChanges(preview.Changes, masked)
	if hasDetails {
		maskDetails(*preview.Details, masked)
	}
	if hasRows {
		for i := range *preview.ImportRows {
			row := &(*preview.ImportRows)[i]
			rowMasked := auth.MaskedFields(viewer, row.Object, true)
			maskChanges(row.Changes, rowMasked)
			maskDetails(row.Details, rowMasked)
		}
	}
	return nil
}

func maskChanges(changes []crmcontracts.ApprovalPreviewChange, masked []string) {
	for i := range changes {
		if slices.Contains(masked, changes[i].Field) {
			changes[i].Before = crmcontracts.ApprovalPreviewValue{Kind: previewValueUnavailable}
			changes[i].After = crmcontracts.ApprovalPreviewValue{Kind: previewValueUnavailable}
		}
	}
}

func maskDetails(details []crmcontracts.ApprovalPreviewDetail, masked []string) {
	for i := range details {
		sources := append([]string{details[i].Field}, previewDetailSources[details[i].Field]...)
		if slices.ContainsFunc(sources, func(field string) bool { return slices.Contains(masked, field) }) {
			details[i].Value = crmcontracts.ApprovalPreviewValue{Kind: previewValueUnavailable}
		}
	}
}

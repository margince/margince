// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	touched := &previewTouched{}
	builder := r
	builder.touched = touched
	if asAgent {
		builder = builder.recording(touched)
	}
	preview, err := builder.buildPreview(effect, approval)
	if err != nil || preview == nil {
		return preview, err
	}
	if asAgent {
		if preview.ReviewerReason != "" {
			preview.ReviewerReason = previewReviewer(ctx, approval)
		}
		withholdConflictSummaries(preview)
		open, err := r.viewerOpensPreview(ctx, approval, preview, touched)
		if err != nil {
			return nil, err
		}
		if !open {
			withholdPreview(preview)
			return preview, nil
		}
	}
	return preview, maskForViewer(ctx, approval, preview, touched)
}

// recording makes every record a label or a record read resolves count as
// touched, so the sight check covers what the agent's build read.
func (r approvalPreviewReader) recording(touched *previewTouched) approvalPreviewReader {
	r.names.seen = touched.add
	if provider, ok := r.records.(*Provider); ok {
		r.records = recordedReads{Provider: provider, touched: touched}
	}
	return r
}

// recordedReads is the record provider that notes each record it read.
type recordedReads struct {
	*Provider
	touched *previewTouched
}

func (p recordedReads) Read(ctx context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	record, err := p.Provider.Read(ctx, ref)
	if err == nil {
		p.touched.add(string(ref.Type), ref.ID)
	}
	return record, err
}

// withholdConflictSummaries keeps a conflict's kind and drops its sentence,
// which an agent's build may have written from records the viewer cannot open.
func withholdConflictSummaries(preview *crmcontracts.ApprovalPreview) {
	if preview.Conflicts == nil {
		return
	}
	for i := range *preview.Conflicts {
		(*preview.Conflicts)[i].Summary = ""
	}
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
	out.addChanges(preview.Changes)
	if preview.Details != nil {
		out.addDetails(*preview.Details)
	}
	if preview.ImportRows != nil {
		for i := range *preview.ImportRows {
			row := &(*preview.ImportRows)[i]
			out.addValue(row.Target)
			out.addChanges(row.Changes)
			out.addDetails(row.Details)
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

func (t *previewTouched) addValue(value *crmcontracts.ApprovalPreviewValue) {
	if value != nil && value.EntityType != nil && value.EntityId != nil {
		t.add(*value.EntityType, ids.UUID(*value.EntityId))
	}
}

func (t *previewTouched) addChanges(changes []crmcontracts.ApprovalPreviewChange) {
	for i := range changes {
		t.addValue(&changes[i].Before)
		t.addValue(&changes[i].After)
	}
}

func (t *previewTouched) addDetails(details []crmcontracts.ApprovalPreviewDetail) {
	for i := range details {
		t.addValue(&details[i].Value)
	}
}

func (r approvalPreviewReader) viewerOpensPreview(ctx context.Context, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview, touched *previewTouched) (bool, error) {
	open, err := r.viewerReadsEvery(ctx, previewReferences(approval, preview, touched).byType)
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
	case previewEntityContract:
		_, err = contracts.NewStore(r.db, r.deals.FreezeRateAt, ContractTimezone()).GetContract(ctx, ids.From[ids.ContractKind](id))
	case previewEntityInvoice:
		_, _, err = finance.NewStore(r.db, FinanceRates()).InvoiceReference(ctx, id)
	case previewEntityAttachment:
		_, err = r.activities.GetAttachmentMeta(ctx, id)
	case previewEntityPage:
		// A cited web page is public and outside row scope, as the approval's
		// own readability rule, which checks only cited activities, treats it.
		return true, nil
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

const (
	previewEntityInvoice    = "invoice"
	previewEntityAttachment = "attachment"
	previewEntityPage       = "page"
	previewEntityContract   = "contract"
)

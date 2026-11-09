// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/platform/approvalplan"
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
	if approval.Status != crmcontracts.ApprovalStatusPending {
		return nil, nil //nolint:nilnil // A plan no longer pending is read through its footprint.
	}
	preview, _, err := r.readAs(ctx, approval, approvals.EffectContext)
	return preview, err
}

// footprint builds approval's plan as a pending read does and keeps only what
// it touched. A writer stores it whatever the status now reads.
func (r approvalPreviewReader) footprint(ctx context.Context, approval crmcontracts.Approval) (*approvalplan.Footprint, error) {
	_, footprint, err := r.readAs(ctx, approval, approvals.EffectContext)
	return footprint, err
}

// readAs is the one path every kind's preview takes. An agent's proposal is
// built as the agent, whose retry runs it. Only a viewer who can open every
// record it touches sees it, and with that viewer's field masks. The
// footprint is taken from the build, before any viewer's sight narrows it.
func (r approvalPreviewReader) readAs(ctx context.Context, approval crmcontracts.Approval, effectOf previewEffect) (*crmcontracts.ApprovalPreview, *approvalplan.Footprint, error) {
	effect, asAgent, err := effectOf(ctx)
	if previewUnavailable(err) {
		// An agent that can no longer call cannot spend the approval.
		return &crmcontracts.ApprovalPreview{Version: 1, State: previewUnavailableState, Changes: []crmcontracts.ApprovalPreviewChange{}, ReviewerReason: previewReviewer(ctx, approval)}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	touched := &previewTouched{}
	builder := r
	builder.touched = touched
	if asAgent {
		builder = builder.recording(touched)
	}
	preview, err := builder.buildPreview(effect, approval)
	if err != nil {
		return nil, nil, err
	}
	footprint := planFootprint(approval, preview, touched, asAgent)
	if preview == nil {
		return nil, footprint, nil
	}
	if asAgent {
		if preview.ReviewerReason != "" {
			preview.ReviewerReason = previewReviewer(ctx, approval)
		}
		withholdConflictSummaries(preview)
		open, err := r.viewerOpensPreview(ctx, approval, preview, touched)
		if err != nil {
			return nil, nil, err
		}
		if !open {
			withholdPreview(preview)
			return preview, footprint, nil
		}
	}
	return preview, footprint, maskForViewer(ctx, approval, preview, touched)
}

// planFootprint keeps what the sight and mask checks read of a built plan. A
// plan its build state withholds from every reader keeps none, and a kind
// with no preview names only its target.
func planFootprint(approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview, touched *previewTouched, asAgent bool) *approvalplan.Footprint {
	if (approvalplan.PreviewRead{Preview: preview, BuiltAsAgent: asAgent}).PayloadWithheld() {
		return nil
	}
	if preview == nil {
		preview = &crmcontracts.ApprovalPreview{}
	}
	refs := previewReferences(approval, preview, touched)
	objects := previewMaskObjects(approval, refs)
	var rows []approvalplan.ImportRowFields
	if preview.ImportRows != nil {
		for i := range *preview.ImportRows {
			row := &(*preview.ImportRows)[i]
			objects = append(objects, row.Object)
			fields := previewFieldNames(&crmcontracts.ApprovalPreview{Changes: row.Changes, Details: &row.Details})
			rows = append(rows, approvalplan.ImportRowFields{Object: row.Object, Fields: fields})
		}
	}
	slices.Sort(objects)
	return &approvalplan.Footprint{Refs: refs.byType, Objects: slices.Compact(objects), Fields: previewFieldNames(preview), Rows: rows, Files: previewFiles(preview)}
}

// viewerClears answers whether the caller opens every record a stored plan
// touched and sees every field it named. Without a footprint only a kind no
// preview covers clears, as its pending card showed its plan to every decider.
func (r approvalPreviewReader) viewerClears(ctx context.Context, approval crmcontracts.Approval, footprint *approvalplan.Footprint) (bool, error) {
	if footprint == nil {
		return hasTypedProposalDisplay(approval.Kind), nil
	}
	viewer, ok := principal.Actor(ctx)
	if !ok {
		return false, apperrors.ErrPermissionDenied
	}
	writable, err := r.viewerWritesTarget(ctx, viewer, approval)
	if err != nil {
		return false, err
	}
	masked := maskedFieldsFor(viewer, approvalTargetType(approval), writable, footprint.Objects)
	if slices.ContainsFunc(footprint.Fields, func(field string) bool { return slices.Contains(masked, field) }) {
		return false, nil
	}
	for _, row := range footprint.Rows {
		rowMasked := auth.MaskedFields(viewer, row.Object, true)
		if slices.ContainsFunc(row.Fields, func(field string) bool { return slices.Contains(rowMasked, field) }) {
			return false, nil
		}
	}
	open, err := r.viewerReadsEvery(ctx, footprint.Refs)
	if err != nil || !open {
		return false, err
	}
	return r.viewerSendsFiles(ctx, footprint.Files)
}

// viewerWritesTarget answers whether the viewer may still write the approval's
// target. Only then does a mask conditioned on write authority lift, because a
// reader who lost that authority after deciding must not see the value again.
func (r approvalPreviewReader) viewerWritesTarget(ctx context.Context, viewer principal.Principal, approval crmcontracts.Approval) (bool, error) {
	target := approvalTargetType(approval)
	if target == "" || approval.TargetEntityId == nil {
		return false, nil
	}
	if slices.Equal(auth.MaskedFields(viewer, target, true), auth.MaskedFields(viewer, target, false)) {
		return true, nil
	}
	id := ids.UUID(*approval.TargetEntityId)
	var writable map[ids.UUID]bool
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		writable, err = auth.WritableSubset(ctx, tx, target, []ids.UUID{id})
		return err
	})
	return writable[id], err
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
	if err != nil || !open {
		return open, err
	}
	return r.viewerSendsFiles(ctx, previewFiles(preview))
}

// viewerSendsFiles answers whether the caller could attach every file, by the
// rule a send applies. A withheld or empty file counts as one they cannot open.
func (r approvalPreviewReader) viewerSendsFiles(ctx context.Context, files []ids.UUID) (bool, error) {
	if len(files) == 0 {
		return true, nil
	}
	if _, err := r.activities.PreviewSendAttachments(ctx, files); err != nil {
		if previewUnavailable(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func previewFiles(preview *crmcontracts.ApprovalPreview) []ids.UUID {
	if preview.Files == nil {
		return nil
	}
	files := make([]ids.UUID, 0, len(*preview.Files))
	for _, file := range *preview.Files {
		files = append(files, ids.UUID(file.Id))
	}
	return files
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

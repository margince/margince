// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	"github.com/margince/margince/backend/internal/compose/lifecyclereview"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func (r approvalPreviewReader) lifecycleEvidence(ctx context.Context, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview) error {
	if approval.Kind != lifecycleProposalKind {
		return nil
	}
	evidence := []crmcontracts.ApprovalEvidence{}
	preview.Evidence = &evidence
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return err
	}
	var proposal lifecycleProposal
	decodeErr := json.Unmarshal(raw, &proposal)
	if decodeErr != nil || proposal.SignalID.IsZero() || approval.TargetEntityId == nil || proposal.CompanyID.UUID != ids.UUID(*approval.TargetEntityId) {
		preview.State = previewIncompleteState
		return nil //nolint:nilerr // Invalid stored proposals are presented as incomplete rather than failing the inbox.
	}
	if proposal.Evidence == nil || proposal.Evidence.SourceID.IsZero() {
		preview.State = previewIncompleteState
		return nil
	}
	r.touch(map[string][]ids.UUID{entityActivity: {proposal.Evidence.SourceID}, tableSignal: {proposal.SignalID}})
	var current lifecyclereview.Finding
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := activities.ReadActivityContentTx(ctx, tx, ids.From[ids.ActivityKind](proposal.Evidence.SourceID), storekit.LiveOnly); err != nil {
			return err
		}
		var readErr error
		current, readErr = lifecyclereview.Reconcile(ctx, tx, proposal.CompanyID, r.now())
		return readErr
	})
	if state := evidenceFailureState(err); state != "" {
		preview.State = state
		return nil
	}
	if err != nil {
		return err
	}
	setLifecyclePreviewContext(preview, current, proposal.Evidence)
	if !lifecyclereview.Matches(current, proposal.Evidence) {
		if !current.SourceID.IsZero() {
			evidence = append(evidence, crmcontracts.ApprovalEvidence{
				EvidenceSnippet: approvals.SanitizeSummary(current.Quote),
				SourceType:      new(crmcontracts.ApprovalEvidenceSourceTypeActivity), SourceId: new(openapi_types.UUID(current.SourceID)),
			})
		}
		preview.State = previewStaleState
		return nil
	}
	signal, err := r.signals.GetSignal(ctx, ids.From[ids.SignalKind](proposal.SignalID), storekit.LiveOnly)
	if previewUnavailable(err) {
		preview.State = previewUnavailableState
		return nil
	}
	if err != nil {
		return err
	}
	if signal.Version == nil || *signal.Version != current.SignalVersion {
		preview.State = previewStaleState
		return nil
	}
	sources, err := r.signalActivityEvidence(ctx, signal)
	if state := evidenceFailureState(err); state != "" {
		preview.State = state
		return nil
	}
	if err != nil {
		return err
	}
	evidence = append(evidence, sources...)
	sourceType := crmcontracts.ApprovalEvidenceSourceType(tableSignal)
	evidence = append(evidence, crmcontracts.ApprovalEvidence{EvidenceSnippet: approvals.SanitizeSummary(signal.Summary), SourceType: &sourceType, SourceId: &signal.Id})
	return nil
}

func (r approvalPreviewReader) signalActivityEvidence(ctx context.Context, signal crmcontracts.Signal) ([]crmcontracts.ApprovalEvidence, error) {
	out := []crmcontracts.ApprovalEvidence{}
	for _, citation := range signal.Evidence {
		if citation.SourceType == nil || string(*citation.SourceType) != string(recordTypeActivity) || citation.SourceId == nil {
			return nil, apperrors.ErrInvalidArgument
		}
		id, err := ids.Parse(*citation.SourceId)
		if err != nil {
			return nil, apperrors.ErrInvalidArgument
		}
		source, err := r.activities.GetActivityContent(ctx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		if citation.SourceQuote == nil || source.Body == nil {
			return nil, apperrors.ErrVersionSkew
		}
		proof := lifecyclereview.QuoteProofOf(*citation.SourceQuote)
		quote, valid := textlang.VerifySenderQuote(*source.Body, proof)
		if !valid {
			return nil, apperrors.ErrVersionSkew
		}
		sourceType := crmcontracts.ApprovalEvidenceSourceType(recordTypeActivity)
		current := crmcontracts.ApprovalEvidence{EvidenceSnippet: approvals.SanitizeSummary(quote), SourceType: &sourceType, SourceId: new(openapi_types.UUID(id))}
		out = append(out, current)
	}
	return out, nil
}

func (r approvalPreviewReader) currentEvidence(ctx context.Context, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview) error {
	if approval.Kind == deals.StageProgressionKind {
		return r.progressionEvidencePreview(ctx, approval, preview)
	}
	if approval.Kind == lifecycleProposalKind {
		return r.lifecycleEvidence(ctx, approval, preview)
	}
	evidence := []crmcontracts.ApprovalEvidence{}
	if approval.Evidence != nil {
		for _, item := range *approval.Evidence {
			current, err := r.currentCitation(ctx, item)
			if state := evidenceFailureState(err); state != "" {
				preview.State = state
				return nil
			}
			if err != nil {
				return err
			}

			evidence = append(evidence, current)
		}
	}
	preview.Evidence = &evidence
	return nil
}

func (r approvalPreviewReader) currentCitation(ctx context.Context, item crmcontracts.ApprovalEvidence) (crmcontracts.ApprovalEvidence, error) {
	if item.SourceType == nil || item.SourceId == nil {
		return item, apperrors.ErrInvalidArgument
	}
	var text string
	switch string(*item.SourceType) {
	case string(recordTypeActivity):
		source, err := r.activities.GetActivityContent(ctx, ids.From[ids.ActivityKind](ids.UUID(*item.SourceId)), storekit.LiveOnly)
		if err != nil {
			return item, err
		}
		if source.Subject != nil {
			text = *source.Subject + "\n"
		}
		if source.Body != nil {
			text += *source.Body
		}
	case tableSignal:
		source, err := r.signals.GetSignal(ctx, ids.From[ids.SignalKind](ids.UUID(*item.SourceId)), storekit.LiveOnly)
		if err != nil {
			return item, err
		}
		if string(source.Status) != commitmentStateOpen {
			return item, apperrors.ErrVersionSkew
		}
		text = source.Summary
	default:
		return item, apperrors.ErrInvalidArgument
	}
	if !values.Quoted(text, item.EvidenceSnippet) {
		return item, apperrors.ErrVersionSkew
	}
	item.EvidenceSnippet = approvals.SanitizeSummary(item.EvidenceSnippet)
	// Stored line offsets may move when the source changes; the quote still opens its source.
	item.SourceLines = nil
	return item, nil
}

func evidenceFailureState(err error) string {
	switch {
	case errors.Is(err, apperrors.ErrVersionSkew):
		return previewStaleState
	case errors.Is(err, apperrors.ErrInvalidArgument):
		return previewIncompleteState
	case previewUnavailable(err):
		return previewUnavailableState
	default:
		return ""
	}
}

func setLifecyclePreviewContext(preview *crmcontracts.ApprovalPreview, current lifecyclereview.Finding, shown *lifecyclereview.Finding) {
	if !current.SourceID.IsZero() {
		preview.LifecycleEvidence = &crmcontracts.LifecyclePreviewEvidence{
			Scope: crmcontracts.LifecyclePreviewEvidenceScope(current.Scope), OccurredAt: current.OccurredAt,
			OccurredPrecision: *activitywhen.PrecisionOf(occurredday.Moment{At: current.OccurredAt, DateOnly: current.DateOnly}),
		}
		if current.Workstream != "" {
			preview.LifecycleEvidence.Workstream = &current.Workstream
		}
	}
	conflicts := []crmcontracts.ApprovalPreviewConflict{}
	reason := lifecyclereview.BlockingReason(current)
	if reason == "" && !lifecyclereview.Matches(current, shown) {
		reason = "newer_evidence"
	}
	if reason != "" {
		conflicts = append(conflicts, crmcontracts.ApprovalPreviewConflict{Kind: reason, BlocksApproval: true})
	}
	preview.Conflicts = &conflicts
}

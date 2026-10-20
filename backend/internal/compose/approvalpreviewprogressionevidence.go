// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) progressionEvidencePreview(ctx context.Context, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview) error {
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return err
	}
	change, err := deals.ReadProgressionChange(raw)
	if err != nil {
		preview.State = previewIncompleteState
		return nil
	}
	evidence, err := r.progressionCitations(ctx, change)
	if state := evidenceFailureState(err); state != "" {
		preview.State = state
		return nil
	}
	if err != nil {
		return err
	}
	details := progressionChecklist(change.Criteria)
	preview.Details = &details
	preview.Evidence = &evidence
	return nil
}

func (r approvalPreviewReader) progressionCitations(ctx context.Context, change deals.StageProgressionChange) ([]crmcontracts.ApprovalEvidence, error) {
	if err := r.validateProgressionCriteria(ctx, change); err != nil {
		return nil, err
	}
	rows, err := r.deals.ListStageEvidence(ctx, change.DealID)
	if err != nil {
		return nil, err
	}
	byID := make(map[ids.UUID]crmcontracts.StageEvidence, len(rows))
	for _, row := range rows {
		byID[ids.UUID(row.Id)] = row
	}
	facts := deals.StageProgressionFacts{}
	for _, criterion := range change.Criteria {
		if !criterion.Met {
			continue
		}
		if len(criterion.EvidenceIDs) == 0 || len(criterion.CitedSourceIDs) == 0 {
			return nil, apperrors.ErrInvalidArgument
		}
		current := deals.ProgressionCriterion{Label: criterion.Label, Met: true}
		for _, id := range criterion.EvidenceIDs {
			row, ok := byID[id]
			if !ok || !row.Met || row.RefutedAt != nil || ids.UUID(row.CriterionId) != criterion.CriterionID.UUID {
				return nil, apperrors.ErrVersionSkew
			}
			if row.SourceMissing {
				return nil, apperrors.ErrPermissionDenied
			}
			if !slices.Contains(criterion.CitedSourceIDs, ids.UUID(row.SourceId)) {
				return nil, apperrors.ErrInvalidArgument
			}
			source := deals.EvidenceSource{SourceType: string(row.SourceType), SourceID: ids.UUID(row.SourceId)}
			if row.Snippet != nil {
				source.Snippet = *row.Snippet
			}
			if err := r.validateProgressionQuotation(ctx, row); err != nil {
				return nil, err
			}
			current.Sources = append(current.Sources, source)
		}
		facts.Criteria = append(facts.Criteria, current)
	}
	citations := progressionEvidence(facts)
	if len(citations) == 0 {
		return nil, apperrors.ErrInvalidArgument
	}
	out := make([]crmcontracts.ApprovalEvidence, 0, len(citations))
	for _, citation := range citations {
		item := crmcontracts.ApprovalEvidence{EvidenceSnippet: approvals.SanitizeSummary(citation.Snippet), SourceType: new(crmcontracts.ApprovalEvidenceSourceType(citation.SourceType)), SourceId: new(crmcontracts.Id(citation.SourceID))}

		out = append(out, item)
	}
	return out, nil
}

func (r approvalPreviewReader) validateProgressionQuotation(ctx context.Context, row crmcontracts.StageEvidence) error {
	if row.Snippet == nil || *row.Snippet == "" {
		// Deterministic writers cite a recorded fact; model claims must retain their quote.
		if row.ExtractedBy != deals.ExtractedByDeterministic {
			return apperrors.ErrInvalidArgument
		}
		return nil
	}
	if row.SourceType != "activity" {
		return nil
	}
	_, err := r.currentCitation(ctx, crmcontracts.ApprovalEvidence{
		EvidenceSnippet: *row.Snippet, SourceType: new(crmcontracts.ApprovalEvidenceSourceType(row.SourceType)), SourceId: &row.SourceId,
	})
	return err
}

func progressionChecklist(criteria []deals.ProposedCriterion) []crmcontracts.ApprovalPreviewDetail {
	out := make([]crmcontracts.ApprovalPreviewDetail, 0, len(criteria))
	for _, criterion := range criteria {
		field := "stage_optional_criterion"
		if criterion.Required {
			field = "stage_required_criterion"
		}
		if criterion.Met {
			field += "_met"
		} else {
			field += "_unmet"
		}
		out = append(out, previewDetail(field, criterion.Label))
	}
	return out
}

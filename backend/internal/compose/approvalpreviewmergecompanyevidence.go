// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func mergeCompanyEvidenceDetails(scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, fact := range scope.CompanyFacts {
		if fact.Source.Id == nil {
			return nil, apperrors.ErrInvalidArgument
		}
		field := "merge_company_fact_moved"
		if fact.Target != nil {
			field = "merge_company_fact_discarded"
		}
		details = append(details, mergeIdentityDetail(field, fact.Source.Value, ids.UUID(*fact.Source.Id)), previewDetail("merge_company_fact_field", string(fact.Source.Field)), previewDetail("merge_company_fact_category", string(fact.Source.Category)))
		if fact.Target != nil {
			if fact.Target.Id == nil {
				return nil, apperrors.ErrInvalidArgument
			}
			details = append(details, mergeIdentityDetail("merge_company_fact_kept", fact.Target.Value, ids.UUID(*fact.Target.Id)))
		}
	}
	for _, field := range scope.CompanyProfiles {
		if field.Source.Id == nil {
			return nil, apperrors.ErrInvalidArgument
		}
		caption := "merge_company_profile_moved"
		if field.Target != nil {
			caption = "merge_company_profile_discarded"
		}
		details = append(details, mergeIdentityDetail(caption, field.Source.Value, ids.UUID(*field.Source.Id)), previewDetail("merge_company_profile_field", string(field.Source.Field)))
		if field.Target != nil {
			if field.Target.Id == nil {
				return nil, apperrors.ErrInvalidArgument
			}
			details = append(details, mergeIdentityDetail("merge_company_profile_kept", field.Target.Value, ids.UUID(*field.Target.Id)))
		}
	}
	for _, kind := range scope.CompanyRelationshipTypes {
		details = append(details, previewDetail("merge_company_relationship_type", kind))
	}
	for _, singleton := range scope.CompanySingletons {
		action := "_moved"
		if singleton.Retained {
			action = "_retained"
		}
		details = append(details, mergeBooleanDetail("merge_company_"+singleton.Kind+action, true))
	}
	return details, nil
}

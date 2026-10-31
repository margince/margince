// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// promotionTouches lists, by type, the records an agent's promotion writes,
// links or relinks. Its approver must be able to open each one.
func promotionTouches(lead ids.UUID, plan contacts.LeadContactChange) map[string][]ids.UUID {
	touched := map[string][]ids.UUID{entityLead: {lead}}
	add := func(entityType string, id *ids.UUID) {
		if id != nil && !id.IsZero() {
			touched[entityType] = append(touched[entityType], *id)
		}
	}
	add(entityContact, &plan.ContactID.UUID)
	if plan.ReviewContact != nil {
		add(entityContact, &plan.ReviewContact.UUID)
	}
	for _, company := range []*ids.CompanyID{plan.Employer.CompanyID, plan.EmployerPreview.CompanyID} {
		if company != nil {
			add(entityCompany, &company.UUID)
		}
	}
	activity := string(datasource.EntityActivity)
	if plan.Evidence != nil {
		add(activity, &plan.Evidence.UUID)
	}
	for _, moved := range plan.Activities {
		add(activity, &moved.ID)
	}
	for _, basis := range plan.ConsentLinks.CommunicationBases {
		add(activity, basis.SourceActivityID)
	}
	return touched
}

// viewerReadsEvery answers whether the caller can open every record named. An
// activity whose content they may not read counts as one they cannot open.
func (r approvalPreviewReader) viewerReadsEvery(ctx context.Context, touched map[string][]ids.UUID) (bool, error) {
	for entityType, want := range touched {
		labels, err := r.names.Labels(ctx, entityType, want)
		if err != nil {
			return false, err
		}
		for _, id := range want {
			if strings.TrimSpace(labels[id]) == "" {
				return false, nil
			}
		}
	}
	return true, nil
}

const previewFieldPhones = "phones"

// promotionDetailSources names the lead and contact fields each plain-text
// detail repeats, so the viewer's field masks reach the card.
var promotionDetailSources = map[string][]string{
	"promotion_contact_name":     {fieldFullName},
	"promotion_contact_email":    {fieldEmail},
	"promotion_contact_phone":    {fieldPhone, previewFieldPhones},
	"promotion_contact_title":    {fieldTitle},
	"promotion_contact_source":   {fieldSource},
	"promotion_contact_owner":    {bulkParamOwner},
	"promotion_deal_owner":       {bulkParamOwner},
	"promotion_company_industry": {fieldIndustry},
	"promotion_company_proposed": {companyNameField},
	companyNameField:             {companyNameField},
}

// maskPromotionDetails withholds every detail that repeats a field the
// viewer's role masks on the lead or on the contact it becomes.
func maskPromotionDetails(ctx context.Context, details []crmcontracts.ApprovalPreviewDetail) error {
	viewer, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	masked := append(auth.MaskedFields(viewer, string(recordTypeLead), true), auth.MaskedFields(viewer, string(recordTypeContact), false)...)
	for i := range details {
		for _, field := range promotionDetailSources[details[i].Field] {
			if slices.Contains(masked, field) {
				details[i].Value = crmcontracts.ApprovalPreviewValue{Kind: previewValueUnavailable}
			}
		}
	}
	return nil
}

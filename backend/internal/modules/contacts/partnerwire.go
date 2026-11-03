// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// How a partner row meets the wire: the kinds and their deprecated single
// role, the stage's label and meaning, and the activation stamp.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// wirePartner maps the row and withholds nothing. wirePartners applies the
// role's masks around it, and a read reaches for that. A record mapped here
// alone carries the margin tier a masked seat may not have.
func wirePartner(p partnerRow, stages map[string]partnerStageFacts) crmcontracts.Partner {
	out := crmcontracts.Partner{
		CompanyId:  openapi_types.UUID(p.CompanyID.UUID),
		CertStatus: crmcontracts.PartnerCertStatus(p.CertStatus),
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
		ArchivedAt: p.ArchivedAt,
	}
	version := crmcontracts.RowVersion(p.Version)
	out.Version = &version
	kinds := make([]crmcontracts.PartnerKind, 0, len(p.PartnerKinds))
	for _, kind := range p.PartnerKinds {
		kinds = append(kinds, crmcontracts.PartnerKind(kind))
	}
	out.PartnerKinds = &kinds
	if role := legacyPartnerRole(p.PartnerKinds); role != nil {
		legacy := crmcontracts.PartnerPartnerRole(*role)
		out.PartnerRole = &legacy
	}
	if p.MarginTier != nil {
		tier := crmcontracts.PartnerMarginTier(*p.MarginTier)
		out.MarginTier = &tier
	}
	out.RelationshipStage = &p.RelationshipStage
	if facts, ok := stages[p.RelationshipStage]; ok {
		out.StageLabel, out.StageLabelKey, out.StageMeaning = &facts.label, facts.labelKey, &facts.meaning
	}
	wirePartnerActivation(&out, p.Activation)
	out.NextStep = p.NextStep
	if p.NextStepDueAt != nil {
		out.NextStepDueAt = &openapi_types.Date{Time: *p.NextStepDueAt}
	}
	if p.ServedSegments != nil {
		segments := p.ServedSegments
		out.ServedSegments = &segments
	}
	out.PartnerFitScore = intPtr(p.FitScore)
	out.PartnerFitScoreComputed = intPtr(p.FitScoreComputed)
	out.PartnerFitOverrideReason = p.FitOverrideReason
	out.RelationshipHealth = p.RelationshipHealth
	out.LastContactAt = p.LastContactAt
	metrics := map[string]any{"certified_staff": p.CertifiedStaff}
	if p.RetentionRate != nil {
		metrics["retention_rate"] = *p.RetentionRate
	}
	out.GateMetrics = &metrics
	return out
}

// wirePartnerActivation copies the activation stamp onto the record.
func wirePartnerActivation(out *crmcontracts.Partner, a partnerActivation) {
	out.ActivatedAt, out.ActivatedBy = a.At, a.By
	if a.Basis != nil {
		basis := crmcontracts.PartnerActivationBasis(*a.Basis)
		out.ActivationBasis = &basis
	}
	if a.LeadID != nil {
		lead := openapi_types.UUID(*a.LeadID)
		out.ActivationLeadId = &lead
	}
	if a.DealID != nil {
		deal := openapi_types.UUID(*a.DealID)
		out.ActivationDealId = &deal
	}
}

// intPtr widens the DB's smallint to the contract's int without sharing
// the row's storage.
func intPtr(v *int16) *int {
	if v == nil {
		return nil
	}
	w := int(*v)
	return &w
}

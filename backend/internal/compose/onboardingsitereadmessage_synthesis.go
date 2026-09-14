// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/modules/contacts"

// Factual identity fields deliberately do not appear here: a legal name,
// address, registration, VAT number, or display name must occur in its cited
// evidence. Interpretive fields may combine only the dossier concepts mapped
// to them below.
var companySynthesisEvidence = map[string]map[string]struct{}{
	fieldIndustry:         evidenceFields(fieldIndustry, contacts.FactServedIndustry, contacts.FactService, contacts.FactProduct, contacts.FactCapability),
	fieldOfferSummary:     evidenceFields(fieldOfferSummary, contacts.FactService, contacts.FactProduct, contacts.FactCapability),
	fieldICP:              evidenceFields(fieldICP, fieldOfferSummary, contacts.FactServedIndustry, contacts.FactCompanySize, contacts.FactGeography, contacts.FactNamedCustomer, contacts.FactService, contacts.FactProduct, contacts.FactCapability),
	fieldValueProposition: evidenceFields(fieldValueProposition, fieldOfferSummary, fieldCustomerPains, fieldDesiredOutcomes, contacts.FactQuantifiedOutcome),
	fieldUSP:              evidenceFields(fieldUSP, fieldValueProposition, contacts.FactCapability, contacts.FactTechnology, contacts.FactCertification, contacts.FactQuantifiedOutcome),
	fieldCustomerPains:    evidenceFields(fieldCustomerPains, fieldOfferSummary, contacts.FactService, contacts.FactProduct, contacts.FactCapability),
	fieldDesiredOutcomes:  evidenceFields(fieldDesiredOutcomes, fieldValueProposition, contacts.FactQuantifiedOutcome),
	fieldBuyingCenter:     evidenceFields(fieldBuyingCenter, fieldICP, contacts.FactNamedCustomer),
	fieldBuyingIntents:    evidenceFields(fieldBuyingIntents, fieldCustomerPains, fieldOfferSummary, contacts.FactService, contacts.FactProduct),
	fieldCommonObjections: evidenceFields(fieldCommonObjections, fieldCustomerPains),
	fieldSalesMotion:      evidenceFields(fieldSalesMotion, fieldBuyingCenter, contacts.FactContactEmail),
	fieldHistory:          evidenceFields(fieldHistory, contacts.FactFoundedYear, contacts.FactLocation),
}

func evidenceFields(fields ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		set[field] = struct{}{}
	}
	return set
}

func companyRecommendationSupportsSynthesis(replyKind, targetField string, sourceIDs map[string]struct{}, known map[string]companyReadEvidence) bool {
	if replyKind != companyConversationRecommendation {
		return false
	}
	relevant, ok := companySynthesisEvidence[targetField]
	if !ok {
		return false
	}
	for sourceID := range sourceIDs {
		if _, ok := relevant[known[sourceID].Field]; ok {
			return true
		}
	}
	return false
}

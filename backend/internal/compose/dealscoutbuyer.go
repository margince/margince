// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
)

func scoutBuyerSignal(signal string) string {
	return `EXISTS (SELECT 1 FROM jsonb_array_elements(` + signal + `.evidence) buyer_cite
 JOIN activity buyer_source ON buyer_source.id = ` + storekit.CitedActivityID("buyer_cite") + `
 JOIN activity_participant buyer_party ON buyer_party.activity_id = buyer_source.id
  AND buyer_party.contact_id = ` + storekit.CitedEntityID("buyer_cite", "buyer_contact_id") + `
 JOIN relationship buyer_job ON buyer_job.contact_id = buyer_party.contact_id
  AND buyer_job.kind = 'employment' AND buyer_job.archived_at IS NULL
  AND ` + buyerEmployment("buyer_job", "buyer_source") + `
 WHERE buyer_cite->>'commercial_role' = 'buyer' AND buyer_cite->>'source_type' = 'activity'
  AND buyer_job.company_id = ` + signal + `.resolved_company_id
  AND NOT EXISTS (SELECT 1 FROM relationship other_job
   JOIN company other_company ON other_company.id = other_job.company_id
    AND other_company.archived_at IS NULL AND NOT other_company.is_anchor
   WHERE other_job.contact_id = buyer_party.contact_id
    AND other_job.company_id <> buyer_job.company_id
    AND ` + buyerEmployment("other_job", "buyer_source") + `)
  AND ` + workspaceEvidence("buyer_source") + `)`
}

func buyerEmployment(alias, activity string) string {
	return alias + `.kind = 'employment' AND ` + alias + `.archived_at IS NULL AND ` +
		employment.IsCurrentSQL(alias+".ended_at") + ` AND ` + employment.InPlaceForSQL(alias, activity)
}

func unlinkedCommercialMatter(signal string) string {
	return `NOT EXISTS (SELECT 1 FROM jsonb_array_elements(` + signal + `.evidence) linked_cite
 JOIN activity_link linked ON linked.activity_id = ` + storekit.CitedActivityID("linked_cite") + `
 LEFT JOIN deal linked_deal ON linked_deal.id = linked.deal_id AND linked_deal.archived_at IS NULL
  AND linked_deal.company_id = ` + signal + `.resolved_company_id
 LEFT JOIN project linked_project ON linked_project.id = linked.project_id AND linked_project.archived_at IS NULL
  AND linked_project.company_id = ` + signal + `.resolved_company_id
 WHERE linked_cite->>'source_type' = 'activity'
 AND (linked_deal.id IS NOT NULL OR linked_project.id IS NOT NULL))`
}

func verifiedSuggestionBuyer(suggestion, company, since, asOf string) string {
	return `EXISTS (SELECT 1 FROM deal_suggestion_evidence buyer_ev
 JOIN signal buyer_signal ON buyer_signal.id = buyer_ev.signal_id
 WHERE buyer_ev.suggestion_id = ` + suggestion + ` AND buyer_signal.kind = 'new_opportunity'
 AND buyer_signal.status = 'open' AND buyer_signal.archived_at IS NULL
 AND buyer_signal.resolved_company_id = ` + company + ` AND ` + scoutBuyerSignal("buyer_signal") + `
 AND ` + unlinkedCommercialMatter("buyer_signal") + `
 AND ` + storekit.CitedActivityTime("buyer_signal.evidence") + ` > ` + since + `
 AND ` + storekit.CitedActivityTime("buyer_signal.evidence") + ` <= ` + asOf + `)`
}

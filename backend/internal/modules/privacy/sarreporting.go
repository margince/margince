// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import "github.com/margince/margince/backend/internal/platform/database/storekit"

func sarReportingSections(pkg *SARPackage) sarSection {
	p := storekit.Placeholders([]int{0})
	return sarSection{&pkg.ReportingContributions, `SELECT c.metric,c.context_id,c.fact->'row'->'occurred_at' AS occurred_at,
 c.fact->'row'->'value' AS value,e.captured_at,e.redacted_at
 FROM report_edition_contribution c JOIN report_edition e ON e.id=c.edition_id WHERE
 (c.source_type='deal' AND c.source_id IN (SELECT deal_id FROM relationship WHERE contact_id=` + p + ` AND kind='deal_stakeholder')) OR
 (c.source_type='activity' AND c.source_id IN (SELECT activity_id FROM activity_link WHERE contact_id=` + p + `)) OR
 (c.source_type='sdr_handoff' AND c.source_id IN (SELECT id FROM sdr_handoff WHERE contact_id=` + p + ` OR lead_id IN (SELECT id FROM lead WHERE promoted_contact_id=` + p + `)))`, nil}
}

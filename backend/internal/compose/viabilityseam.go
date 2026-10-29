// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How the forecast reads a deal's viability: from the receipt the input check
// left and any review still open on it. The check writes; this only reads, and
// it reads the same fingerprint the check stored, so "the evidence moved since"
// has one definition.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// viabilityFingerprintSQL is what a viability assessment read about one deal.
// The check stores it on the receipt, and the forecast compares it with the
// deal's current one: any difference means the evidence moved since the check.
// Work the product filed itself is left out, so the review's own task does not
// make its receipt stale. Archiving an activity drops it from the count, which
// changes the fingerprint too.
func viabilityFingerprintSQL(deal string) string {
	return `md5(concat_ws('|', ` + deal + `.owner_id, ` + deal + `.status, ` + deal + `.archived_at, ` +
		deal + `.expected_close_date, ` + deal + `.close_date_provisional, ` + deal + `.wait_until,
		(SELECT concat_ws('|', count(*), max(greatest(fa.updated_at, fl.created_at)))
		   FROM activity_link fl JOIN activity fa ON fa.id = fl.activity_id
		  WHERE fl.deal_id = ` + deal + `.id AND fa.archived_at IS NULL AND ` +
		relstrength.OriginIsEngagementSQL("fa") + `)))`
}

// viabilityQualificationSQL is one deal's treatment in the qualified forecast.
//
// Eligible needs a CURRENT receipt: this policy, not past its validity, judged
// on the evidence the deal still carries. Without one the deal is unknown, even
// with no finding open, because no finding is not the same as judged and fine.
// A judged deal with an open review is excluded; one whose review somebody
// resolved or overrode counts again.
func viabilityQualificationSQL(deal string, arg func(any) int) string {
	return fmt.Sprintf(`(CASE
		WHEN %[1]s.status <> 'open' THEN '%[5]s'
		WHEN NOT EXISTS (SELECT 1 FROM assurance_deal_receipt vr
		                  WHERE vr.deal_id = %[1]s.id AND vr.policy_version = $%[2]d
		                    AND vr.outcome <> '%[8]s' AND vr.valid_until > now()
		                    AND vr.input_fingerprint = %[3]s) THEN '%[7]s'
		WHEN EXISTS (SELECT 1 FROM assurance_exception vx
		              WHERE vx.subject_kind = 'deal' AND vx.subject_id = %[1]s.id
		                AND vx.type = $%[4]d AND vx.status = 'open') THEN '%[6]s'
		ELSE '%[5]s' END)`,
		deal, arg(viability.PolicyVersion), viabilityFingerprintSQL(deal), arg(assurance.TypeDealViability),
		viability.QualificationEligible, viability.QualificationExcluded, viability.QualificationUnknown,
		viability.OutcomeUnknown)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How the forecast reads a deal's viability: from the receipt the input check
// left and any review still open on it. The check writes; this only reads, and
// it reads the same fingerprint the check stored, so "the evidence moved since"
// has one definition.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// viabilityFingerprintSQL is what a viability assessment read about one deal.
// The check stores it on the receipt, and the forecast compares it with the
// deal's current one. A difference means the evidence moved since the check.
// Work the product filed itself is left out, so the review's own task does not
// make its receipt stale. Archiving an activity drops it from the count.
//
// Beside the deal and its activities it covers what decides whether an
// activity counts as the customer: each party's role, contact and response,
// the contacts the activity is filed under, and which contact owns a
// senderless message's address. It also covers the answers to the retired
// buyer-silent check, which defer the question.
func viabilityFingerprintSQL(deal string) string {
	return `md5(concat_ws('|', ` + deal + `.owner_id, ` + deal + `.status, ` + deal + `.archived_at, ` +
		deal + `.expected_close_date, ` + deal + `.close_date_provisional, ` + deal + `.wait_until,
		(SELECT concat_ws('|', count(*), max(greatest(fa.updated_at, fl.created_at)),
		        count(*) FILTER (WHERE fa.reply_verdict = '` + activities.ReplyVerdictNegative + `'),
		        max(fa.reply_verdict_at),
		        (SELECT md5(coalesce(string_agg(concat_ws(':', fp.activity_id, fp.role, fp.contact_id, fp.address, fp.response),
		                 ',' ORDER BY fp.activity_id, fp.id), ''))
		           FROM activity_participant fp WHERE fp.activity_id = ANY (array_agg(fa.id))),
		        (SELECT md5(coalesce(string_agg(concat_ws(':', fc.activity_id, fc.contact_id), ',' ORDER BY fc.activity_id, fc.contact_id), ''))
		           FROM activity_link fc WHERE fc.activity_id = ANY (array_agg(fa.id)) AND fc.contact_id IS NOT NULL),
		        (SELECT md5(coalesce(string_agg(concat_ws(':', fe.email, fe.contact_id, fe.archived_at), ',' ORDER BY fe.email, fe.contact_id), ''))
		           FROM contact_email fe WHERE fe.email = ANY (array_agg(fa.counterparty_email))))
		   FROM activity_link fl JOIN activity fa ON fa.id = fl.activity_id
		  WHERE fl.deal_id = ` + deal + `.id AND fa.archived_at IS NULL AND ` + visibleToAllSQL("fa") + ` AND ` +
		relstrength.OriginIsEngagementSQL("fa") + `),
		(SELECT concat_ws('|', count(*), max(fr.created_at))
		   FROM assurance_resolution fr JOIN assurance_exception fx ON fx.id = fr.exception_id
		  WHERE fx.subject_kind = 'deal' AND fx.subject_id = ` + deal + `.id AND fx.type = '` + assurance.TypeBuyerSilent + `')))`
}

// viabilityQualificationSQL is one deal's treatment in the qualified forecast.
//
// Eligible needs a CURRENT receipt: this policy, not past its validity, judged
// on the evidence the deal still carries. Without one the deal is unknown, even
// with no finding open, because no finding is not the same as judged and fine.
// A judged deal with an open review is excluded, and so is one judged
// unsupported, which has no review to answer. An override counts the deal
// again until it ends; once it has ended the deal is unknown until the next
// check reopens the review.
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
		WHEN EXISTS (SELECT 1 FROM assurance_deal_receipt vr
		              WHERE vr.deal_id = %[1]s.id AND vr.outcome = '%[10]s') THEN '%[11]s'
		WHEN EXISTS (SELECT 1 FROM assurance_exception vx
		              WHERE vx.subject_kind = 'deal' AND vx.subject_id = %[1]s.id
		                AND vx.type = $%[4]d AND vx.status = 'resolved'
		                AND NOT EXISTS (SELECT 1 FROM assurance_resolution vs
		                                 WHERE vs.exception_id = vx.id AND vs.outcome = '%[9]s'
		                                   AND vs.expires_at > now())) THEN '%[7]s'
		ELSE '%[5]s' END)`,
		deal, arg(viability.PolicyVersion), viabilityFingerprintSQL(deal), arg(assurance.TypeDealViability),
		viability.QualificationEligible, viability.QualificationExcluded, viability.QualificationUnknown,
		viability.OutcomeUnknown, assurance.OutcomeValueCorrect,
		viability.OutcomeUnsupported, viability.QualificationUnsupported)
}

// viabilityBasisSQL is what a deal's qualification rested on, as four columns.
// They are the policy enforced, the last assessment, the review standing, and
// whether the deal was judged on current evidence. It makes the same receipt
// and review reads as viabilityQualificationSQL, frozen beside its answer.
func viabilityBasisSQL(deal string, arg func(any) int) string {
	return fmt.Sprintf(`$%[2]d::text,
		(SELECT vr.assessed_at FROM assurance_deal_receipt vr WHERE vr.deal_id = %[1]s.id),
		(CASE
		  WHEN EXISTS (SELECT 1 FROM assurance_exception vx
		                WHERE vx.subject_kind = 'deal' AND vx.subject_id = %[1]s.id
		                  AND vx.type = $%[4]d AND vx.status = 'open') THEN '%[6]s'
		  WHEN EXISTS (SELECT 1 FROM assurance_exception vx
		                JOIN assurance_resolution vs ON vs.exception_id = vx.id
		                WHERE vx.subject_kind = 'deal' AND vx.subject_id = %[1]s.id
		                  AND vx.type = $%[4]d AND vx.status = 'resolved'
		                  AND vs.outcome = '%[9]s' AND vs.expires_at > now()) THEN '%[7]s'
		  WHEN EXISTS (SELECT 1 FROM assurance_deal_receipt vr
		                WHERE vr.deal_id = %[1]s.id AND vr.outcome = '%[10]s') THEN '%[8]s'
		  ELSE '%[5]s' END),
		EXISTS (SELECT 1 FROM assurance_deal_receipt vr
		         WHERE vr.deal_id = %[1]s.id AND vr.policy_version = $%[2]d
		           AND vr.outcome <> '%[11]s' AND vr.valid_until > now()
		           AND vr.input_fingerprint = %[3]s)`,
		deal, arg(viability.PolicyVersion), viabilityFingerprintSQL(deal), arg(assurance.TypeDealViability),
		forecasting.DispositionNone, forecasting.DispositionOpenReview, forecasting.DispositionOverride,
		forecasting.DispositionUnsupported, assurance.OutcomeValueCorrect, viability.OutcomeUnsupported,
		viability.OutcomeUnknown)
}

// noViabilityBasisSQL fills the four basis columns while enforcement is off.
const noViabilityBasisSQL = `NULL::text, NULL::timestamptz, NULL::text, NULL::boolean`

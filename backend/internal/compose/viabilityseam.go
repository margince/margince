// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a deal's viability assessment read. The input check stores it on the
// receipt, and the freshness check compares it, so "the evidence moved since"
// has one definition.

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// viabilityFingerprintSQL is what a viability assessment read about one deal.
// Each input that changes the answer changes it. Work the product filed itself
// is left out, so its own tasks never make a receipt stale.
//
// Participants and addresses are digested row by row. A tally or a maximum
// stays put when one row is swapped for another.
func viabilityFingerprintSQL(deal string) string {
	return `md5(concat_ws('|', ` + deal + `.owner_id, ` + deal + `.status, ` + deal + `.archived_at, ` +
		deal + `.expected_close_date, ` + deal + `.close_date_provisional, ` + deal + `.wait_until,
		(SELECT concat_ws('|', count(*), max(greatest(fa.updated_at, fl.created_at)),
		        count(*) FILTER (WHERE fa.reply_verdict = '` + activities.ReplyVerdictNegative + `'),
		        max(fa.reply_verdict_at),
		        (SELECT md5(string_agg(concat_ws(':', fp.contact_id, fp.user_id, fp.address, fp.channel_user_id, fp.role, fp.response),
		                               ',' ORDER BY fp.id))
		           FROM activity_participant fp WHERE fp.activity_id = ANY (array_agg(fa.id))),
		        (SELECT md5(string_agg(concat_ws(':', ce.id, ce.version), ',' ORDER BY ce.id)) FROM contact_email ce
		          WHERE ce.contact_id IN (SELECT fpc.contact_id FROM activity_participant fpc
		                                   WHERE fpc.activity_id = ANY (array_agg(fa.id))
		                                     AND fpc.contact_id IS NOT NULL)))
		   FROM activity_link fl JOIN activity fa ON fa.id = fl.activity_id
		  WHERE fl.deal_id = ` + deal + `.id AND fa.archived_at IS NULL AND ` + visibleToAllSQL("fa") + ` AND ` +
		relstrength.OriginIsEngagementSQL("fa") + `),
		array_to_string(` + dealMailboxesSQL(deal) + `, ',')))`
}

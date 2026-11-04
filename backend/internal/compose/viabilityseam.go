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
// The check stores it on the receipt, and the freshness check compares it with
// the deal's current one. A difference means the evidence moved since the check.
// A verdict that lands or changes after the message counts too, and so does a
// guest accepting or withdrawing, because each changes the answer. Work the
// product filed itself is left out, so its own tasks do not make a receipt
// stale. Archiving an activity drops it from the count, which changes the
// fingerprint too. So does a mail connection that delivers the deal's mail for
// the first time, because the deal's coverage is judged on those connections.
func viabilityFingerprintSQL(deal string) string {
	return `md5(concat_ws('|', ` + deal + `.owner_id, ` + deal + `.status, ` + deal + `.archived_at, ` +
		deal + `.expected_close_date, ` + deal + `.close_date_provisional, ` + deal + `.wait_until,
		(SELECT concat_ws('|', count(*), max(greatest(fa.updated_at, fl.created_at)),
		        count(*) FILTER (WHERE fa.reply_verdict = '` + activities.ReplyVerdictNegative + `'),
		        max(fa.reply_verdict_at),
		        (SELECT count(*) FROM activity_participant fp
		          WHERE fp.activity_id = ANY (array_agg(fa.id)) AND fp.response = 'accepted'))
		   FROM activity_link fl JOIN activity fa ON fa.id = fl.activity_id
		  WHERE fl.deal_id = ` + deal + `.id AND fa.archived_at IS NULL AND ` + visibleToAllSQL("fa") + ` AND ` +
		relstrength.OriginIsEngagementSQL("fa") + `),
		array_to_string(` + dealMailboxesSQL(deal) + `, ',')))`
}

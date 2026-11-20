// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealverdictsum holds two parts of the checksum that makes a deal due
// for a verdict again. Every part aliases the deal as d.
package dealverdictsum

import (
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// LedgerSQL is the deal's criterion ledger as a custom criterion reads
// it: each row, who wrote it, whether it met, and whether it was refuted. An
// erased snippet leaves it alone. It is NULL for a deal with no ledger, which
// concat_ws skips, so such a deal keeps its stored checksum.
const LedgerSQL = `(SELECT md5(string_agg(concat_ws(':', ev.id, ev.author_side, ev.met, ev.refuted_at), ',' ORDER BY ev.id))
		   FROM deal_stage_evidence ev WHERE ev.deal_id = d.id)`

// OwnSideSQL names the participants of the deal's mail and meetings
// that stand on a trusted own domain. A buyer milestone and a held meeting are
// judged against those domains. An own domain added or removed has to make the
// deal due again. The set read here holds every domain the judged set
// does, so it can only read a deal again for nothing, never miss one. NULL
// when no participant stands on one.
func OwnSideSQL() string {
	return `(SELECT md5(string_agg(op.id::text, ',' ORDER BY op.id))
		   FROM activity_link ol JOIN activity oa ON oa.id = ol.activity_id
		   JOIN activity_participant op ON op.activity_id = oa.id
		  WHERE ol.deal_id = d.id AND oa.archived_at IS NULL
		    AND EXISTS (SELECT 1 FROM (` + auth.TrustedOwnDomainsFoldedSQL + `) own(domain)
		                 WHERE ` + owndomain.CoversSQL("op.address_domain_folded", "own.domain") + `))`
}

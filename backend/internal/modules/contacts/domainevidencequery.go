// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// A single statement keeps projection readiness and its answer on one snapshot.
func domainEvidenceSQL() string {
	return `WITH projection AS MATERIALIZED (` + storekit.ParticipantEvidenceReadySQL() + `)
 SELECT CASE WHEN (SELECT ready FROM projection) THEN (` + domainEvidenceLatestSQL() + `)
 ELSE (` + domainEvidenceLegacySQL() + `) END`
}

func domainEvidenceLatestSQL() string {
	const horizon = "now()+@skew::interval"
	const reversed = "(reverse(ap.address_domain_folded) COLLATE \"C\")"
	exact := reversed + `=(reverse(@domain) COLLATE "C")`
	proper := owndomain.CoversIndexedSQL("ap.address_domain_folded", "@domain") +
		` AND ` + reversed + `>=((reverse(@domain)||'.') COLLATE "C")`
	arms := []string{`(SELECT a.occurred_at FROM activity a WHERE a.id=@activity
 AND a.restricted_at IS NULL AND a.archived_at IS NULL AND ` + occurredday.BegunSQL("a", horizon) + `)`}
	for _, bound := range []struct{ precision, ceiling string }{
		{`ap.domain_evidence_precision='instant'`, horizon},
		{`ap.domain_evidence_precision='day'`, `greatest(` + horizon + `,` + occurredday.TodayAnchorSQL(horizon) + `)`},
	} {
		for _, domain := range []string{exact, proper} {
			arms = append(arms, `(SELECT ap.domain_evidence_at AS occurred_at FROM activity_participant ap
 WHERE ap.domain_evidence_visible AND NOT ap.past_party_cap AND ap.address_domain_folded IS NOT NULL
 AND `+bound.precision+` AND `+domain+` AND ap.domain_evidence_at<=`+bound.ceiling+`
 ORDER BY ap.domain_evidence_at <-> (`+bound.ceiling+`) LIMIT 1)`)
		}
	}
	return `SELECT max(occurred_at) FROM (` + strings.Join(arms, ` UNION ALL `) + `) latest`
}

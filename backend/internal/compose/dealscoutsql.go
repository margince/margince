// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// dealScoutSQL is the scout's one read, in four steps:
//
//   - ev: every piece of evidence in the window that could count, from
//     companies that may be offered a suggestion;
//   - pairs: per company, the newest new_opportunity with the newest
//     commitment_made within signalPairWindow of it;
//   - chosen: the companies a rule fires for — a meeting, a document or a pair
//     — newest evidence first, capped;
//   - the evidence the chosen companies' suggestions cite, and for their
//     documents alone, the latest finished reading's amount and currency.
func dealScoutSQL(since, now time.Time, companyCap int) (string, []any) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sincePos, nowPos := arg(since), arg(now)
	in := func(at string) string {
		return fmt.Sprintf(`%[1]s > $%[2]d AND %[1]s <= $%[3]d`, at, sincePos, nowPos)
	}
	pairSeconds := arg(int64(signalPairWindow / time.Second))
	document := proposalDocumentSQL("at.filename", "at.content_type", arg)
	query := `
	WITH ev AS (
	  SELECT mc.company_id, 'meeting' AS kind, a.id AS ref, a.occurred_at, NULL::text AS signal_kind
	    FROM (` + activities.HeldMeetingCounterparties() + `) mc
	    JOIN activity a ON a.id = mc.activity_id
	   WHERE ` + workspaceEvidence("a") + ` AND ` + in("a.occurred_at") + `
	     AND ` + scoutCandidate("mc.company_id", "a.occurred_at") + `
	  UNION ALL
	  SELECT s.resolved_company_id, 'signal', s.id, s.detected_at, s.kind
	    FROM signal s
	   WHERE s.kind IN ('new_opportunity', 'commitment_made')
	     AND s.status = 'open' AND s.archived_at IS NULL AND s.visibility = 'workspace'
	     AND ` + in("s.detected_at") + `
	     AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(s.evidence) cite
	           WHERE cite->>'source_type' = 'activity' AND NOT EXISTS (SELECT 1 FROM activity ca
	             WHERE ca.id = ` + storekit.CitedActivityID("cite") + ` AND ` + workspaceEvidence("ca") + `))
	     AND ` + scoutCandidate("s.resolved_company_id", "s.detected_at") + `
	  UNION ALL
	  SELECT filed.company_id, 'attachment', at.id, a.occurred_at, NULL
	    FROM attachment at
	    JOIN activity a ON a.id = at.activity_id
	    JOIN LATERAL (
	      SELECT at.company_id WHERE at.company_id IS NOT NULL
	      UNION
	      SELECT reach.company_id FROM (` + activities.CompanyReachSet() + `) reach
	       WHERE reach.activity_id = a.id AND at.company_id IS NULL) filed ON true
	   WHERE at.archived_at IS NULL AND NOT at.bytes_withheld AND a.kind = 'email' AND a.direction = 'outbound'
	     AND ` + workspaceEvidence("a") + ` AND ` + in("a.occurred_at") + `
	     AND ` + document + `
	     AND ` + scoutCandidate("filed.company_id", "a.occurred_at") + `
	), pairs AS (
	  SELECT DISTINCT ON (o.company_id) o.company_id, o.ref AS opportunity, c.ref AS commitment,
	         greatest(o.occurred_at, c.occurred_at) AS occurred_at
	    FROM ev o
	    JOIN ev c ON c.company_id = o.company_id AND c.signal_kind = 'commitment_made'
	     AND abs(extract(epoch FROM c.occurred_at - o.occurred_at)) <= $` + fmt.Sprint(pairSeconds) + `
	   WHERE o.signal_kind = 'new_opportunity'
	   ORDER BY o.company_id, o.occurred_at DESC, c.occurred_at DESC
	), chosen AS (
	  SELECT company_id FROM (
	    SELECT company_id, occurred_at FROM ev WHERE kind <> 'signal'
	    UNION ALL
	    SELECT company_id, occurred_at FROM pairs) qualifying
	   GROUP BY company_id
	   ORDER BY max(occurred_at) DESC, company_id
	   LIMIT $` + fmt.Sprint(arg(companyCap)) + `
	), cited AS (
	  SELECT ev.*, row_number() OVER (PARTITION BY ev.company_id, ev.kind ORDER BY ev.occurred_at DESC, ev.ref) AS rank
	    FROM ev JOIN chosen USING (company_id)
	   WHERE ev.kind <> 'signal'
	      OR ev.ref IN (SELECT p.opportunity FROM pairs p WHERE p.company_id = ev.company_id
	                    UNION SELECT p.commitment FROM pairs p WHERE p.company_id = ev.company_id)
	)
	SELECT cited.company_id, cited.kind, cited.ref, cited.occurred_at, reading.amount, reading.currency
	  FROM cited
	  LEFT JOIN LATERAL (
	    SELECT (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
	             WHERE f->>'Field' = 'amount_minor' AND NOT coalesce((f->>'Omitted')::boolean, false)) AS amount,
	           (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
	             WHERE f->>'Field' = 'currency' AND NOT coalesce((f->>'Omitted')::boolean, false)) AS currency
	      FROM attachment_extraction x
	     WHERE cited.kind = 'attachment' AND x.attachment_id = cited.ref AND x.status = 'done'
	     ORDER BY x.finished_at DESC LIMIT 1) reading ON true
	 WHERE cited.kind = 'signal' OR cited.rank <= $` + fmt.Sprint(arg(dealScoutItemCap)) + `
	 ORDER BY cited.company_id, cited.occurred_at DESC, cited.ref`
	return query, args
}

// workspaceEvidence admits an activity the whole workspace may read: the
// workspace audience, live, and on a thread nobody holds.
func workspaceEvidence(alias string) string {
	return fmt.Sprintf(`(%[1]s.audience = 'workspace' AND %[1]s.archived_at IS NULL AND NOT %[2]s)`,
		alias, capture.CounterpartyHeldOn(alias))
}

// scoutCandidate narrows evidence to companies that may be offered a
// suggestion, and to evidence newer than the company's floor.
func scoutCandidate(company, occurred string) string {
	return deals.SuggestableCompanyClause(company) +
		` AND ` + occurred + ` > coalesce(` + deals.SuggestionFloorExpr(company) + `, '-infinity'::timestamptz)`
}

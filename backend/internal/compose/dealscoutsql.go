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
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Buyer intent selects candidates; meetings and documents only corroborate them.
func dealScoutSQL(since, now time.Time, companyCap int) (string, []any) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sincePos, nowPos := arg(since), arg(now)
	in := func(at string) string {
		return fmt.Sprintf(`%[1]s > $%[2]d AND %[1]s <= $%[3]d`, at, sincePos, nowPos)
	}
	pairSeconds := arg(int64(signalPairWindow / time.Second))
	document := proposalDocumentSQL("at.filename", "at.content_type", arg)
	query := `WITH ev AS (` + scoutEvidenceSQL(in, document) + `
	), deduped AS (
   SELECT DISTINCT ON (company_id, kind, source_identity) * FROM ev
   ORDER BY company_id, kind, source_identity, occurred_at, ref
 ), pairs AS (
	  SELECT DISTINCT ON (o.company_id) o.company_id, o.ref AS opportunity, c.ref AS commitment,
	         greatest(o.occurred_at, c.occurred_at) AS occurred_at
	    FROM deduped o
	    JOIN deduped c ON c.company_id = o.company_id AND c.signal_kind = 'commitment_made' AND c.matter = o.matter
	     AND abs(extract(epoch FROM c.occurred_at - o.occurred_at)) <= $` + fmt.Sprint(pairSeconds) + `
	   WHERE o.signal_kind = 'new_opportunity'
	   ORDER BY o.company_id, o.occurred_at DESC, c.occurred_at DESC
	), chosen AS (
	  SELECT company_id FROM (
	    SELECT company_id, occurred_at FROM pairs) qualifying
	   GROUP BY company_id
	   ORDER BY max(occurred_at) DESC, company_id
	   LIMIT $` + fmt.Sprint(arg(companyCap)) + `
	), cited AS (
	  SELECT ev.*, row_number() OVER (PARTITION BY ev.company_id, ev.kind ORDER BY ev.occurred_at DESC, ev.ref) AS rank
	    FROM deduped ev JOIN chosen USING (company_id)
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

// scoutEvidenceSQL places a meeting or a document by occurredday.StartSQL, so a
// day entry falls in the window and past the floor by its date.
func scoutEvidenceSQL(in func(string) string, document string) string {
	placed := occurredday.StartSQL("a")
	return `
	  SELECT mc.company_id, 'meeting' AS kind, a.id AS ref, a.occurred_at, NULL::text AS signal_kind, a.id::text AS source_identity, NULL::text AS matter
	    FROM (` + activities.HeldMeetingCounterparties() + `) mc
	    JOIN activity a ON a.id = mc.activity_id
	   WHERE ` + workspaceEvidence("a") + ` AND ` + in(placed) + `
	     AND ` + scoutCandidate("mc.company_id", placed) + `
	  UNION ALL
	  SELECT s.resolved_company_id, 'signal', s.id, source.occurred_at, s.kind, s.id::text, source.matter
	    FROM signal s
	    CROSS JOIN LATERAL (SELECT ` + storekit.CitedActivityTime("s.evidence") + ` AS occurred_at,
         (SELECT CASE WHEN count(DISTINCT coalesce(nullif(m.thread_key, ''), m.id::text)) = 1
            THEN min(coalesce(nullif(m.thread_key, ''), m.id::text)) END
          FROM jsonb_array_elements(s.evidence) matter_cite
          JOIN activity m ON m.id = ` + storekit.CitedActivityID("matter_cite") + `
          WHERE matter_cite->>'source_type' = 'activity') AS matter) source
	   WHERE s.kind IN ('new_opportunity', 'commitment_made')
	     AND (s.kind <> 'new_opportunity' OR (` + scoutBuyerSignal("s") + ` AND ` + unlinkedCommercialMatter("s") + `))
	     AND s.status = 'open' AND s.archived_at IS NULL AND s.visibility = 'workspace'
	     AND ` + in("source.occurred_at" /* occurred_at raw: the time a signal cites carries no precision */) + `
	     AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(s.evidence) cite
	           WHERE cite->>'source_type' = 'activity' AND NOT EXISTS (SELECT 1 FROM activity ca
	             WHERE ca.id = ` + storekit.CitedActivityID("cite") + ` AND ` + workspaceEvidence("ca") + `))
	     AND ` + scoutCandidate("s.resolved_company_id", "source.occurred_at" /* occurred_at raw: the time a signal cites carries no precision */) + `
	  UNION ALL
	  SELECT filed.company_id, 'attachment', at.id, a.occurred_at, NULL, coalesce(nullif(at.checksum, ''), at.id::text), NULL::text
	    FROM attachment at
	    JOIN activity a ON a.id = at.activity_id
	    JOIN (` + activities.CompanyReachSet() + `) filed ON filed.activity_id = a.id
	   WHERE at.archived_at IS NULL AND NOT at.bytes_withheld AND a.kind = 'email' AND a.direction = 'outbound'
	     AND ` + workspaceEvidence("a") + ` AND ` + in(placed) + `
	     AND ` + document + `
	     AND ` + scoutCandidate("filed.company_id", placed) + ``
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

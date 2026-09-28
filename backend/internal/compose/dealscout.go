// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Deal Scout: a pass over one workspace that finds companies with no open deal
// where the evidence says commercial motion is under way, and hands each one
// to deals as a suggestion.
//
// Deterministic on purpose, the posture signalscan.go argues for: a wrong
// suggestion is a card somebody dismisses, and no model decides that a deal
// exists. Three kinds of evidence count, each already on record:
//
//   - a held meeting with somebody outside at the company;
//   - both a new_opportunity and a commitment_made signal on it within thirty days;
//   - a proposal or contract sent to it as a document.
//
// Only evidence the whole workspace may read is used: an activity with the
// workspace audience on a thread nobody holds, and a signal visible to the
// workspace whose cited messages are the same. A suggestion built from private
// evidence would tell colleagues, by its mere existence, about mail they may
// not read.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// dealScoutWindow is how far back evidence counts. Older motion that never
	// became a deal is not a suggestion anybody wants today.
	dealScoutWindow = 90 * 24 * time.Hour
	// dealScoutCompanyCap bounds one pass. A company that is suggested leaves
	// the candidate set, so the next pass reaches the ones this one did not.
	dealScoutCompanyCap = 200
	// signalPairWindow is how close the two signals must be.
	signalPairWindow = 30 * 24 * time.Hour
	// dealScoutItemCap bounds the evidence one suggestion cites per kind.
	dealScoutItemCap = 5
)

// DealScoutPass is what one pass did, for the log line.
type DealScoutPass struct {
	Superseded, Considered, Raised int
}

// scoutItem is one piece of evidence about one company.
type scoutItem struct {
	company     ids.UUID
	companyName string
	evidence    deals.SuggestionEvidence
	signalKind  string
	filename    string
	amountMinor *int64
	currency    *string
}

// RunDealScout runs one pass inside the caller's transaction, as the system
// principal the job binds.
func RunDealScout(ctx context.Context, tx pgx.Tx, now time.Time) (DealScoutPass, error) {
	var pass DealScoutPass
	var err error
	if pass.Superseded, err = deals.SupersedeStaleSuggestionsTx(ctx, tx); err != nil {
		return pass, err
	}
	since := now.Add(-dealScoutWindow)
	var items []scoutItem
	for _, read := range []func(context.Context, pgx.Tx, time.Time, time.Time) ([]scoutItem, error){
		scoutMeetings, scoutSignals, scoutDocuments,
	} {
		found, err := read(ctx, tx, since, now)
		if err != nil {
			return pass, err
		}
		items = append(items, found...)
	}
	for _, company := range byNewestEvidence(items) {
		pass.Considered++
		draft, ok := draftSuggestion(company)
		if !ok {
			continue
		}
		raised, err := deals.RecordSuggestionTx(ctx, tx, draft)
		if err != nil {
			return pass, err
		}
		if raised {
			pass.Raised++
		}
	}
	return pass, nil
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

func scoutMeetings(ctx context.Context, tx pgx.Tx, since, now time.Time) ([]scoutItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT mc.company_id, co.display_name, a.id, a.occurred_at
		  FROM (`+activities.HeldMeetingCounterparties()+`) mc
		  JOIN activity a ON a.id = mc.activity_id
		  JOIN company co ON co.id = mc.company_id
		 WHERE `+workspaceEvidence("a")+`
		   AND a.occurred_at > $1 AND a.occurred_at <= $2
		   AND `+scoutCandidate("mc.company_id", "a.occurred_at"),
		since, now)
	if err != nil {
		return nil, fmt.Errorf("deal scout: reading held meetings: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (scoutItem, error) {
		var it scoutItem
		var activity ids.UUID
		err := row.Scan(&it.company, &it.companyName, &activity, &it.evidence.OccurredAt)
		it.evidence.Kind, it.evidence.ActivityID = deals.EvidenceMeeting, &activity
		return it, err
	})
}

func scoutSignals(ctx context.Context, tx pgx.Tx, since, now time.Time) ([]scoutItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.resolved_company_id, co.display_name, s.id, s.kind, s.detected_at
		  FROM signal s
		  JOIN company co ON co.id = s.resolved_company_id
		 WHERE s.kind IN ('new_opportunity', 'commitment_made')
		   AND s.status = 'open' AND s.archived_at IS NULL AND s.visibility = 'workspace'
		   AND s.detected_at > $1 AND s.detected_at <= $2
		   AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(s.evidence) cite
		         WHERE cite->>'source_type' = 'activity' AND NOT EXISTS (SELECT 1 FROM activity ca
		           WHERE ca.id::text = cite->>'source_id' AND `+workspaceEvidence("ca")+`))
		   AND `+scoutCandidate("s.resolved_company_id", "s.detected_at"),
		since, now)
	if err != nil {
		return nil, fmt.Errorf("deal scout: reading signals: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (scoutItem, error) {
		var it scoutItem
		var signal ids.UUID
		err := row.Scan(&it.company, &it.companyName, &signal, &it.signalKind, &it.evidence.OccurredAt)
		it.evidence.Kind, it.evidence.SignalID = deals.EvidenceSignal, &signal
		return it, err
	})
}

// scoutDocuments reads the documents sent to candidate companies, with the
// amount and currency of their latest finished reading when it stated both. A
// document belongs to the company capture filed it under, or, when capture
// filed it under none, to each company its message reaches through its links.
// The file name is judged in Go (isProposalDocument), where the words of a
// name can be told apart.
func scoutDocuments(ctx context.Context, tx pgx.Tx, since, now time.Time) ([]scoutItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT filed.company_id, co.display_name, at.id, a.occurred_at, at.filename, coalesce(at.content_type, ''),
		       reading.amount, reading.currency
		  FROM attachment at
		  JOIN activity a ON a.id = at.activity_id
		  JOIN LATERAL (
		    SELECT at.company_id WHERE at.company_id IS NOT NULL
		    UNION
		    SELECT reach.company_id FROM (`+activities.CompanyReachSet()+`) reach
		     WHERE reach.activity_id = a.id AND at.company_id IS NULL) filed ON true
		  JOIN company co ON co.id = filed.company_id
		  LEFT JOIN LATERAL (
		    SELECT (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
		             WHERE f->>'Field' = 'amount_minor' AND NOT coalesce((f->>'Omitted')::boolean, false)) AS amount,
		           (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
		             WHERE f->>'Field' = 'currency' AND NOT coalesce((f->>'Omitted')::boolean, false)) AS currency
		      FROM attachment_extraction x
		     WHERE x.attachment_id = at.id AND x.status = 'done'
		     ORDER BY x.finished_at DESC LIMIT 1) reading ON true
		 WHERE at.archived_at IS NULL AND a.kind = 'email' AND a.direction = 'outbound'
		   AND `+workspaceEvidence("a")+`
		   AND a.occurred_at > $1 AND a.occurred_at <= $2
		   AND `+scoutCandidate("filed.company_id", "a.occurred_at"),
		since, now)
	if err != nil {
		return nil, fmt.Errorf("deal scout: reading sent documents: %w", err)
	}
	found, err := pgx.CollectRows(rows, scanScoutDocument)
	if err != nil {
		return nil, fmt.Errorf("deal scout: scanning sent documents: %w", err)
	}
	var proposals []scoutItem
	for _, it := range found {
		if it.filename != "" {
			proposals = append(proposals, it)
		}
	}
	return proposals, nil
}

// scanScoutDocument reads one document row. A document whose name is not a
// proposal comes back with no file name, and the caller drops it.
func scanScoutDocument(row pgx.CollectableRow) (scoutItem, error) {
	var it scoutItem
	var attachment ids.UUID
	var contentType string
	var amount, currency *string
	if err := row.Scan(&it.company, &it.companyName, &attachment, &it.evidence.OccurredAt,
		&it.filename, &contentType, &amount, &currency); err != nil {
		return scoutItem{}, err
	}
	it.evidence.Kind, it.evidence.AttachmentID = deals.EvidenceAttachment, &attachment
	if !isProposalDocument(it.filename, contentType) {
		it.filename = ""
		return it, nil
	}
	if amount != nil && currency != nil {
		if minor, err := strconv.ParseInt(*amount, 10, 64); err == nil {
			it.amountMinor, it.currency = &minor, currency
		}
	}
	return it, nil
}

// byNewestEvidence groups the items by company, newest evidence first, and
// keeps the pass's cap of companies.
func byNewestEvidence(items []scoutItem) [][]scoutItem {
	grouped := map[ids.UUID][]scoutItem{}
	for _, it := range items {
		grouped[it.company] = append(grouped[it.company], it)
	}
	companies := make([][]scoutItem, 0, len(grouped))
	for _, group := range grouped {
		sort.Slice(group, func(i, j int) bool {
			return group[i].evidence.OccurredAt.After(group[j].evidence.OccurredAt)
		})
		companies = append(companies, group)
	}
	sort.Slice(companies, func(i, j int) bool {
		return companies[i][0].evidence.OccurredAt.After(companies[j][0].evidence.OccurredAt)
	})
	if len(companies) > dealScoutCompanyCap {
		companies = companies[:dealScoutCompanyCap]
	}
	return companies
}

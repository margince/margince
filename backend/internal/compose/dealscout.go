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
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// dealScoutWindow is how far back evidence counts. Older motion that never
	// became a deal is not a suggestion anybody wants today.
	dealScoutWindow = 90 * 24 * time.Hour
	// dealScoutCompanyCap bounds one pass. It chooses among companies whose
	// evidence already qualifies, so a flood of lone signals cannot crowd out
	// a company with a held meeting; a company that is suggested leaves the
	// candidate set, and the next pass reaches the ones this one did not.
	dealScoutCompanyCap = 200
	// signalPairWindow is how close the two signals must be.
	signalPairWindow = 30 * 24 * time.Hour
	// dealScoutItemCap bounds the meetings and documents one suggestion cites.
	dealScoutItemCap = 5
)

// DealScoutPass is what one pass did, for the log line.
type DealScoutPass struct {
	Superseded, Considered, Raised int
}

// scoutItem is one cited piece of evidence about one chosen company.
type scoutItem struct {
	company     ids.UUID
	evidence    deals.SuggestionEvidence
	amountMinor *int64
	currency    *string
}

// RunDealScout runs one pass inside the caller's transaction, as the system
// principal the job binds.
func RunDealScout(ctx context.Context, tx pgx.Tx, now time.Time) (DealScoutPass, error) {
	return scoutPass(ctx, tx, now, dealScoutCompanyCap)
}

func scoutPass(ctx context.Context, tx pgx.Tx, now time.Time, companyCap int) (DealScoutPass, error) {
	var pass DealScoutPass
	var err error
	if pass.Superseded, err = deals.SupersedeStaleSuggestionsTx(ctx, tx); err != nil {
		return pass, err
	}
	items, err := readScoutEvidence(ctx, tx, now.Add(-dealScoutWindow), now, companyCap)
	if err != nil {
		return pass, err
	}
	for _, company := range byCompany(items) {
		pass.Considered++
		draft := draftSuggestion(company)
		if draft.DuplicateOf, err = contacts.OpenDuplicateCompaniesTx(ctx, tx, draft.CompanyID); err != nil {
			return pass, err
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

// readScoutEvidence answers the evidence each suggestion will cite, for at most
// companyCap companies. One statement decides it all in the database: which
// evidence counts, which companies it qualifies, which of those the cap keeps,
// and — only for the documents of the companies kept — what their finished
// readings stated.
func readScoutEvidence(ctx context.Context, tx pgx.Tx, since, now time.Time, companyCap int) ([]scoutItem, error) {
	query, args := dealScoutSQL(since, now, companyCap)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("deal scout: reading the evidence: %w", err)
	}
	items, err := pgx.CollectRows(rows, scanScoutItem)
	if err != nil {
		return nil, fmt.Errorf("deal scout: scanning the evidence: %w", err)
	}
	return items, nil
}

func scanScoutItem(row pgx.CollectableRow) (scoutItem, error) {
	var it scoutItem
	var ref ids.UUID
	var amount, currency *string
	if err := row.Scan(&it.company, &it.evidence.Kind, &ref, &it.evidence.OccurredAt, &amount, &currency); err != nil {
		return scoutItem{}, err
	}
	switch it.evidence.Kind {
	case deals.EvidenceMeeting:
		it.evidence.ActivityID = &ref
	case deals.EvidenceSignal:
		it.evidence.SignalID = &ref
	default:
		it.evidence.AttachmentID = &ref
	}
	if amount != nil && currency != nil {
		if minor, err := strconv.ParseInt(*amount, 10, 64); err == nil {
			it.amountMinor, it.currency = &minor, currency
		}
	}
	return it, nil
}

// byCompany groups the rows, which arrive ordered by company.
func byCompany(items []scoutItem) [][]scoutItem {
	var out [][]scoutItem
	for _, it := range items {
		if n := len(out); n > 0 && out[n-1][0].company == it.company {
			out[n-1] = append(out[n-1], it)
			continue
		}
		out = append(out, []scoutItem{it})
	}
	return out
}

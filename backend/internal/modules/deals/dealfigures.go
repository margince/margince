// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The figures a card states about a deal, for several deals at once.
//
// A surface that lists deals somebody else ranked — the Worklist, reading the
// overnight brief — holds ids and needs each deal's money and dates to say
// anything useful about it. Reading them one at a time is a query per card;
// this is one query per page.
//
// Deliberately NOT a general deal read: it answers six columns and joins
// nothing, because a caller that needs the whole deal needs GetDeal.
//
// It does apply the ROLE MASK, which is not part of "the whole deal" — it is
// the gate that decides whether this reader may see a deal's money at all, and
// every other deal read applies it. Skipping it here would print on the
// Worklist the figure the record page withholds.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskFigures withholds the figures this reader's ROLE withholds.
//
// Row scope decides which deals answer; the field mask decides which of their
// columns do, and the two are different gates. A role can be authored to hide
// amount_minor on deals outside the reader's write authority, and every other
// deal read applies it — the list masks its page, the single get masks its row,
// the report engine drops masked rows out of its totals so a sum cannot
// disclose what a row would not. A read that skipped it would print on the
// Worklist the number the record page refuses to show.
//
// The money pair goes together. A withheld amount takes its currency with it,
// because a currency alone says a deal is priced and in what units, which is
// half of what the mask was hiding.
//
// Unlike a Deal on the wire, DealFigures carries no masked_fields list, so a
// withheld amount is indistinguishable from an unpriced deal. That is the same
// answer the card already gives for a deal with no amount, and it is the safe
// direction: it says less rather than claiming a figure is absent when it is
// merely withheld.
func maskFigures(ctx context.Context, tx pgx.Tx, figures map[ids.UUID]DealFigures) error {
	p, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	// Cheap exit for the common case — no mask on deals at all, which is how
	// every installation ships until an operator authors one.
	if len(auth.MaskedFields(p, "deal", false)) == 0 {
		return nil
	}
	dealIDs := make([]ids.UUID, 0, len(figures))
	for id := range figures {
		dealIDs = append(dealIDs, id)
	}
	writable, err := auth.WritableSubset(ctx, tx, dealTable, dealIDs)
	if err != nil {
		return err
	}
	for id, row := range figures {
		for _, field := range auth.MaskedFields(p, "deal", writable[id]) {
			if field == dealAmountField {
				row.AmountMinor = nil
				row.Currency = ""
			}
		}
		figures[id] = row
	}
	return nil
}

// dealAmountField is the masked field this read can actually withhold. The
// others in dealMaskableFields name columns it does not select.
const dealAmountField = "amount_minor"

// DealFigures is one deal's commercial face: what it is worth, when it was
// meant to land, and who answers for it.
type DealFigures struct {
	StageID           ids.UUID
	OwnerID           ids.UUID
	AmountMinor       *int64
	Currency          string
	ExpectedCloseDate *time.Time
	// CloseOverdue is CloseIsOverdue's own verdict for this deal — the ONE
	// place that comparison is made (closedate.go), called here rather than
	// re-spelled. Meaningless where ExpectedCloseDate is nil.
	CloseOverdue bool
}

// figuresScanCap bounds one read. A page names as many deals as it has rows,
// and a caller that hands over more than this is asking a different question
// than the one this answers.
const figuresScanCap = 200

// Figures answers the stated figures of the given deals, keyed by id.
//
// A deal this reader may not see is ABSENT from the answer rather than an
// error, which is the refusal shape the label resolver beside it uses: the
// caller keeps the row and states less about it. Absence and "no such deal" are
// deliberately the same answer here — both mean this reader has nothing to say
// about that id, and distinguishing them would tell them a deal exists.
func (s *Store) Figures(ctx context.Context, dealIDs []ids.UUID) (map[ids.UUID]DealFigures, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	out := map[ids.UUID]DealFigures{}
	if len(dealIDs) == 0 {
		return out, nil
	}
	if len(dealIDs) > figuresScanCap {
		// Refused rather than silently sliced. No caller can reach this today —
		// the brief that feeds it is capped far below — so a silent truncation
		// would be a short answer nothing would ever notice, waiting for the
		// first caller to grow past it. A caller with more deals than this is
		// asking a different question and should page.
		return nil, fmt.Errorf("deals: asked for %d deals' figures, and one read answers %d",
			len(dealIDs), figuresScanCap)
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// The installation's zone, read the same way installationToday and the
		// nightly corrector read it — CloseIsOverdue below is what actually
		// answers "is this deal overdue", asked once per row rather than
		// re-spelled in SQL.
		tzName, err := s.installation.Timezone(ctx, tx)
		if err != nil {
			return fmt.Errorf("read the installation's timezone: %w", err)
		}
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			return fmt.Errorf("the installation's timezone %q: %w", tzName, err)
		}
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		idsPos := arg(dealIDs)
		scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
		if err != nil {
			return err
		}
		query := storekit.SQLf(
			`SELECT d.id, d.stage_id, d.owner_id, d.amount_minor, d.currency, d.expected_close_date
			   FROM deal d
			  WHERE d.id = ANY($%d) AND d.archived_at IS NULL`, idsPos,
		)
		if scope != "" {
			query += storekit.SQLf(" AND %s", scope)
		}
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		now := s.clock()
		for rows.Next() {
			var (
				id     ids.UUID
				stage  *ids.UUID
				owner  *ids.UUID
				amount *int64
				code   *string
				closes *time.Time
			)
			if err := rows.Scan(&id, &stage, &owner, &amount, &code, &closes); err != nil {
				return err
			}
			figures := DealFigures{AmountMinor: amount, ExpectedCloseDate: closes}
			if closes != nil {
				figures.CloseOverdue = CloseIsOverdue(*closes, now, loc)
			}
			if stage != nil {
				figures.StageID = *stage
			}
			if owner != nil {
				figures.OwnerID = *owner
			}
			if code != nil {
				figures.Currency = *code
			}
			out[id] = figures
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return maskFigures(ctx, tx, out)
	})
	if err != nil {
		return nil, fmt.Errorf("deals: reading the figures behind a page of deals: %w", err)
	}
	return out, nil
}

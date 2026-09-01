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
// Deliberately NOT a general deal read. It answers four columns, masks nothing
// and joins nothing, because a caller that needs a deal needs GetDeal — which
// masks references the reader may not see. What is here is what a card states
// out loud, and every column of it is already on the row this reader may read.

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

// DealFigures is one deal's commercial face: what it is worth, when it was
// meant to land, and who answers for it.
type DealFigures struct {
	StageID           ids.UUID
	OwnerID           ids.UUID
	AmountMinor       *int64
	Currency          string
	ExpectedCloseDate *time.Time
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
		dealIDs = dealIDs[:figuresScanCap]
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
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
			  WHERE d.id = ANY($%d) AND d.archived_at IS NULL`, idsPos)
		if scope != "" {
			query += storekit.SQLf(" AND %s", scope)
		}
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
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
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("deals: reading the figures behind a page of deals: %w", err)
	}
	return out, nil
}

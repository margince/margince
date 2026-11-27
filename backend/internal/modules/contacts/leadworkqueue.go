// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// leadQueueCursor is the complete keyset behind the default leads queue:
// SLA band, score, then the oldest lead. Every term in ORDER BY is carried so
// page two starts strictly after page one rather than repeating or skipping a
// lead at a shared score.
type leadQueueCursor struct {
	AsOf      time.Time `json:"as_of"`
	Rank      int       `json:"rank"`
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
	ID        ids.UUID  `json:"id"`
}

const (
	leadQueueRankBreached = iota
	leadQueueRankAtRisk
	leadQueueRankWithinTarget
	leadQueueRankInactive
)

// encodeLeadQueueCursor renders the queue's position, and it can genuinely
// fail: the position carries two instants read off rows, and a timestamp
// outside year 0..9999 — which Postgres stores and this product never writes —
// has no JSON form. The caller answers the read rather than paging past a
// position it cannot name.
func encodeLeadQueueCursor(cursor leadQueueCursor) (string, error) {
	return storekit.EncodeOpaque(cursor)
}

func decodeLeadQueueCursor(token string) (leadQueueCursor, error) {
	cursor, err := storekit.DecodeOpaque[leadQueueCursor](token)
	if err != nil {
		return leadQueueCursor{}, err
	}
	// The envelope proves the token is ours; these prove it names a position in
	// THIS queue. A zero instant or id is `{}` unmarshalling cleanly, and a
	// rank or score outside its range is a token edited by hand — both would
	// otherwise page from a place no row occupies.
	if cursor.AsOf.IsZero() || cursor.CreatedAt.IsZero() || cursor.ID.IsZero() {
		return leadQueueCursor{}, &storekit.MalformedCursorError{}
	}
	if cursor.Rank < leadQueueRankBreached || cursor.Rank > leadQueueRankInactive || cursor.Score < 0 || cursor.Score > 100 {
		return leadQueueCursor{}, &storekit.MalformedCursorError{}
	}
	return cursor, nil
}

// listLeadWorkQueue serves the contract's default ordering: SLA band first,
// score inside the band, then oldest lead and id for deterministic ties.
func (s *Store) listLeadWorkQueue(ctx context.Context, in ListLeadsInput) ([]crmcontracts.Lead, storekit.Page, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	limit := storekit.ClampLimit(in.Limit)
	policy, err := s.slaPolicy(ctx)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var leads []crmcontracts.Lead
	var page storekit.Page
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		active, err := s.activeColumnsIn(ctx, tx, leadEntity)
		if err != nil {
			return err
		}
		prepared, err := withLeadTaskIdentities(ctx, tx)
		if err != nil {
			return err
		}
		leads, page, err = readLeadWorkQueue(prepared, tx, in, active, limit, policy)
		return err
	})
	return leads, page, err
}

func readLeadWorkQueue(ctx context.Context, tx pgx.Tx, in ListLeadsInput, active []fieldcatalog.Column, limit int, policy leadSLAPolicy) ([]crmcontracts.Lead, storekit.Page, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	asOf := slaNow(ctx)
	offset, err := storekit.ListOffset(in.Offset, in.Cursor)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var cursor *leadQueueCursor
	if in.Cursor != nil && *in.Cursor != "" {
		decoded, err := decodeLeadQueueCursor(*in.Cursor)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		cursor = &decoded
		asOf = decoded.AsOf
	}
	// The filters, the rank, the columns and the scanner all judge the page's
	// instant, the cursor's on a continuation page.
	ctx = withPinnedSLANow(ctx, asOf)
	where, args, arg, err := leadQueueWhere(ctx, in, active, policy)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	rank := leadQueueRank(policy, arg, asOf)
	if cursor != nil {
		where = append(where, storekit.SQLf("("+rank+", -score, created_at, id) > ($%d, $%d, $%d, $%d)",
			arg(cursor.Rank), arg(-cursor.Score), arg(cursor.CreatedAt), arg(cursor.ID)))
	}
	columns, err := leadColumns(ctx, arg)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	query := `SELECT ` + columns + storekit.SelectSuffix(active) + `, ` + rank +
		` FROM lead WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY ` + rank + `, score DESC, created_at, id` + storekit.PageWindow(limit, offset)
	return readLeadQueuePage(ctx, tx, leadQueueRead{
		sql: query, args: *args, active: active, asOf: asOf,
		total: storekit.PageCount{Cursor: in.Cursor, Offset: offset, Limit: limit, Count: func() (storekit.ListCount, error) {
			return storekit.CountList(ctx, leadEntity, leadEntity, func(arg func(any) int) ([]string, error) {
				return leadListWhere(ctx, in, active, policy, arg)
			})
		}},
	}, policy)
}

func leadQueueWhere(ctx context.Context, in ListLeadsInput, active []fieldcatalog.Column, policy leadSLAPolicy) ([]string, *[]any, func(any) int, error) {
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	where := []string{whereAlways}
	scope, err := auth.ScopeClauseFor(ctx, leadEntity, "", arg)
	if err != nil {
		return nil, nil, nil, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	filters, err := leadListWhere(ctx, in, active, policy, arg)
	if err != nil {
		return nil, nil, nil, err
	}
	return append(where, filters...), &args, arg, nil
}

// leadQueueRank renders the SLA band as a CASE over the lead's own columns.
// With the target switched off every lead ranks the same and the queue
// falls through to score, then age.
func leadQueueRank(policy leadSLAPolicy, arg func(any) int, asOf time.Time) string {
	if !policy.enabled {
		// No target, so no lead is LATE — but a lead still owes a first reply,
		// and the queue's job is to put the ones that do in front of the ones
		// that do not. Ranking every row the same made an answered lead with a
		// high score outrank an unanswered one, which is the queue's whole
		// question answered backwards. within_target is the honest band for
		// "owed, with nothing measuring by when".
		return fmt.Sprintf(`CASE
			WHEN archived_at IS NOT NULL OR first_response_at IS NOT NULL THEN %d
			ELSE %d END`, leadQueueRankInactive, leadQueueRankWithinTarget)
	}
	minutes := policy.targetMinutes()
	risk := int(policy.atRisk() / time.Minute)
	deadline := storekit.SQLf("COALESCE(routed_at, created_at) + $%d * interval '1 minute'", arg(minutes))
	return fmt.Sprintf(`CASE
		WHEN archived_at IS NOT NULL OR first_response_at IS NOT NULL THEN %d
		WHEN %s < $%d THEN %d
		WHEN %s - $%d * interval '1 minute' <= $%d THEN %d
		ELSE %d END`, leadQueueRankInactive, deadline, arg(asOf), leadQueueRankBreached,
		deadline, arg(risk), arg(asOf), leadQueueRankAtRisk, leadQueueRankWithinTarget)
}

// leadQueueRead is one queue page's statement, the instant it judges the SLA
// at, and the count behind its total.
type leadQueueRead struct {
	sql    string
	args   []any
	active []fieldcatalog.Column
	asOf   time.Time
	total  storekit.PageCount
}

func readLeadQueuePage(ctx context.Context, tx pgx.Tx, read leadQueueRead, policy leadSLAPolicy) ([]crmcontracts.Lead, storekit.Page, error) {
	limit, asOf := read.total.Limit, read.asOf
	var leads []crmcontracts.Lead
	var ranks []int

	rows, err := tx.Query(ctx, read.sql, read.args...)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var rank int
		lead, err := scanLead(rows, read.active, policy, asOf, systemtext.Locale(ctx), &rank)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		leads = append(leads, lead)
		ranks = append(ranks, rank)
	}
	if err := rows.Err(); err != nil {
		return nil, storekit.Page{}, err
	}
	matching, err := read.total.Total(ctx, tx, len(leads))
	if err != nil {
		return nil, storekit.Page{}, fmt.Errorf("count lead: %w", err)
	}
	// This page path must stamp writability before offering edits to colleagues’ rows.
	if err := attachLeadRows(ctx, tx, leads); err != nil {
		return nil, storekit.Page{}, err
	}
	page := storekit.Page{Total: matching}
	if len(leads) > limit {
		leads = leads[:limit]
		last := leads[limit-1]
		next, err := encodeLeadQueueCursor(leadQueueCursor{
			AsOf: asOf, Rank: ranks[limit-1], Score: last.Score, CreatedAt: last.CreatedAt, ID: ids.UUID(last.Id),
		})
		if err != nil {
			return nil, storekit.Page{}, err
		}
		page = storekit.Page{HasMore: true, NextCursor: next, Total: matching}
	}
	if leads == nil {
		leads = []crmcontracts.Lead{}
	}
	return leads, page, nil
}

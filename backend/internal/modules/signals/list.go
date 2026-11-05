// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ListSignalsInput scopes the visible signal page.
type ListSignalsInput struct {
	Cursor          *string
	Limit           *int
	Status          *string
	Kind            *string
	ResolutionState *string
	// The company key also matches the polymorphic signal subject.
	CompanyID       *ids.UUID
	IncludeArchived bool
}

// OfCompanyWhere includes direct company subjects and resolved contacts.
// Deal-subject signals stay with their deal, even when resolved onto this company.
func OfCompanyWhere(companyPos int) string {
	return storekit.SQLf(
		`((s.entity_type IS DISTINCT FROM 'deal' AND s.resolved_company_id = $%[1]d)
		  OR (s.entity_type = 'company' AND s.entity_id = $%[1]d))`, companyPos)
}

// ListSignals applies row visibility before returning evidence.
func (s *Store) ListSignals(ctx context.Context, in ListSignalsInput) ([]crmcontracts.Signal, storekit.Page, error) {
	if err := auth.Require(ctx, "signal", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	var records []crmcontracts.Signal
	var page storekit.Page
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		records, page, err = ListSignalsTx(ctx, tx, in, s.words)
		return err
	})
	return records, page, err
}

// ListSignalsTx shares the visible signal reader with transactional callers.
// Nil words keep the stored summaries.
func ListSignalsTx(ctx context.Context, tx pgx.Tx, in ListSignalsInput, words SummaryWords) ([]crmcontracts.Signal, storekit.Page, error) {
	if err := auth.Require(ctx, "signal", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	limit := storekit.ClampLimit(in.Limit)

	where, args, err := signalListFilters(ctx, in)
	if err != nil {
		return nil, storekit.Page{}, err
	}

	var signals []crmcontracts.Signal
	var page storekit.Page
	rows, err := tx.Query(ctx,
		`SELECT `+signalColumns("s")+` FROM signal s`+signalResolutionJoin+` WHERE `+strings.Join(where, " AND ")+
			storekit.SQLf(` ORDER BY s.created_at DESC, s.id DESC LIMIT %d`, limit+1),
		args...)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		sig, params, err := scanSignal(rows)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		signals = append(signals, words.reword(ctx, sig, params))
	}
	if err := rows.Err(); err != nil {
		return nil, storekit.Page{}, err
	}
	if len(signals) > limit {
		signals = signals[:limit]
		last := signals[len(signals)-1]
		next, err := storekit.EncodeCursor(last.CreatedAt, ids.UUID(last.Id))
		if err != nil {
			return nil, storekit.Page{}, err
		}
		page = storekit.Page{HasMore: true, NextCursor: next}
	}
	if signals == nil {
		signals = []crmcontracts.Signal{}
	}
	return signals, page, nil
}

// LockCompanySignalsTx holds existing signals against reopening or changing kind.
func LockCompanySignalsTx(ctx context.Context, tx pgx.Tx, company ids.CompanyID) error {
	if err := auth.Require(ctx, "signal", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureLinkTarget(ctx, tx, "company", company.UUID); err != nil {
		return err
	}
	args := []any{company}
	_, err := tx.Exec(ctx, `SELECT s.id FROM signal s WHERE `+OfCompanyWhere(len(args))+` ORDER BY s.id FOR UPDATE`, args...)
	return err
}

// CompanySignalsCompleteTx prevents partial visibility from proving an account-wide claim.
// CompanySignalsCompleteTx withholds certainty when a visible account contains private signals.
func CompanySignalsCompleteTx(ctx context.Context, tx pgx.Tx, company ids.CompanyID, kind string) (bool, error) {
	if err := auth.Require(ctx, "signal", principal.ActionRead); err != nil {
		return false, err
	}
	if err := auth.EnsureLinkTarget(ctx, tx, "company", company.UUID); err != nil {
		return false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos, kindPos := arg(company), arg(kind)
	scope, err := auth.SignalScopeClause(ctx, "s", arg)
	if err != nil {
		return false, err
	}
	if scope == "" {
		return true, nil
	}
	query := storekit.SQLf(`SELECT coalesce(bool_and(coalesce((%s), false)), true) FROM signal s
 WHERE %s AND s.kind = $%d AND s.status = 'open' AND s.archived_at IS NULL`, scope, OfCompanyWhere(companyPos), kindPos)
	var complete bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&complete); err != nil {
		return false, err
	}
	return complete, nil
}

func signalListFilters(ctx context.Context, in ListSignalsInput) ([]string, []any, error) {
	where := []string{"1=1"}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	if !in.IncludeArchived {
		where = append(where, "s.archived_at IS NULL")
	}
	if in.Status != nil {
		where = append(where, storekit.SQLf("s.status = $%d", arg(*in.Status)))
	}
	if in.Kind != nil {
		where = append(where, storekit.SQLf("s.kind = $%d", arg(*in.Kind)))
	}
	if in.ResolutionState != nil {
		where = append(where, storekit.SQLf("s.resolution_state = $%d", arg(*in.ResolutionState)))
	}
	if in.CompanyID != nil {
		where = append(where, OfCompanyWhere(arg(*in.CompanyID)))
	}
	scope, err := auth.SignalScopeClause(ctx, "s", arg)
	if err != nil {
		return nil, nil, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	if in.Cursor != nil && *in.Cursor != "" {
		c, err := storekit.DecodeCursor(*in.Cursor)
		if err != nil {
			return nil, nil, err
		}
		where = append(where, storekit.SQLf("(s.created_at, s.id) < ($%d, $%d)", arg(c.CreatedAt), arg(c.ID)))
	}

	return where, args, nil
}

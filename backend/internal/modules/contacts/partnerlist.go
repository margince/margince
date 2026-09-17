// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The partner list: what it may be narrowed by, what it may be ordered by, and
// the page that continues in that order.
//
// Its own file because the ORDER is a vocabulary now rather than one column.
// The list declared a `sort` parameter and ordered by company_id regardless, so
// a client offering the control showed an order the server never applied; what
// replaced that is a field set, a keyset that carries the sort key, and a
// refusal for a field this list cannot order by — three things a reader asking
// "what can I sort partners by?" should find together rather than inside the
// upsert's file.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// ListPartnersInput narrows and orders the partner list.
type ListPartnersInput struct {
	PartnerRole *string
	CertStatus  *string
	Limit       *int
	Cursor      string
	// Sort is the caller's ordering, in the shared Sort component's spelling.
	// Nil or the default spelling means the house order.
	Sort *string
}

// partnerSortFields is what the partner list may be ordered by: the columns it
// publishes, which is what a reader can see and therefore what they can order
// by.
//
// The company's NAME is not here, and its absence is the Sort component's own
// example of what this server cannot express yet — a partner ordered by its
// company is ordered by a joined value, and the keyset cursor has no key for
// one. Leaving it out is what makes the 422 an honest answer rather than a
// silent fall back to some other order.
// columnCertifiedStaff is the one sortable partner column no filter already
// names, so it carries its own spelling rather than a third raw literal.
const columnCertifiedStaff = "certified_staff"

var partnerSortFields = map[string]storekit.SortField{
	filterCertStatus:      storekit.Column(fieldcatalog.TypeText),
	filterPartnerRole:     storekit.Column(fieldcatalog.TypeText),
	"margin_tier":         storekit.Column(fieldcatalog.TypeText),
	"relationship_stage":  storekit.Column(fieldcatalog.TypeText),
	columnCertifiedStaff:  storekit.Column(fieldcatalog.TypeNumber),
	"retention_rate":      storekit.Column(fieldcatalog.TypeNumber),
	"partner_fit_score":   storekit.Column(fieldcatalog.TypeNumber),
	"relationship_health": storekit.Column(fieldcatalog.TypeNumber),
	"last_contact_at":     storekit.Column(storekit.KindTimestamp),
	"next_step_due_at":    storekit.Column(fieldcatalog.TypeDate),
	"created_at":          storekit.Column(storekit.KindTimestamp),
	"updated_at":          storekit.Column(storekit.KindTimestamp),
}

// ListPartners reads one page of the partner list, in the caller's order or the
// house default. A partner row is a read of its company, so both grants are
// asked for before anything is selected.
func (s *Store) ListPartners(ctx context.Context, in ListPartnersInput) ([]partnerRow, storekit.Page, error) {
	if err := auth.Require(ctx, "partner", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	limit := storekit.ClampLimit(in.Limit)
	var out []partnerRow
	var page storekit.Page
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, page, err = listPartnersTx(ctx, tx, in, limit)
		return err
	})
	return out, page, err
}

// listPartnersTx runs the keyset-paged partner list inside the caller's
// transaction: filters + company-derived row scope, one page + lookahead.
func listPartnersTx(ctx context.Context, tx pgx.Tx, in ListPartnersInput, limit int) ([]partnerRow, storekit.Page, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The sort FIRST: it may bind parameters of its own, and the cursor clause
	// below is rendered from it, so both have to count through one counter.
	sorted, err := storekit.ParseListSort(ctx, in.Sort, partnerSortFields, arg)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	where, err := partnerListWhere(ctx, in, sorted, arg)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	// p.id beside the row: the house cursor is (created_at, id) and this list
	// publishes the COMPANY id as its identity, which is not the partner row's.
	// Ordering on a column the tuple does not name is how a page boundary
	// repeats or skips a row.
	// The company is an EXISTS rather than a JOIN, and that is what lets the
	// house keyset name its columns. `company` carries a `created_at` and an
	// `id` of its own, so with it in the FROM list the cursor tuple — which is
	// alias-free by design, since every other list it serves reads one table —
	// is ambiguous and Postgres refuses the statement. Nothing outside selects
	// a company column; the join was only ever a filter and a scope.
	sql := storekit.SQLf(`
		SELECT %s, p.id%s FROM partner p
		WHERE %s%s LIMIT $%d`,
		aliased(partnerColumns, "p"), sorted.CursorKeySuffix(), strings.Join(where, " AND "),
		sorted.OrderBy(), arg(limit+1))
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	defer rows.Close()
	var out []partnerRow
	var rowIDs []ids.UUID
	var cursorKeys []*string
	for rows.Next() {
		p, id, key, err := scanPartnerPage(rows, sorted)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		out = append(out, p)
		rowIDs = append(rowIDs, id)
		cursorKeys = append(cursorKeys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, storekit.Page{}, err
	}
	var page storekit.Page
	if len(out) > limit {
		out = out[:limit]
		next, err := sorted.EncodePageCursor(cursorKeys[limit-1], out[limit-1].CreatedAt, rowIDs[limit-1])
		if err != nil {
			return nil, storekit.Page{}, err
		}
		page = storekit.Page{HasMore: true, NextCursor: next}
	}
	return out, page, nil
}

// scanPartnerPage scans one row of a sorted page: the published columns, the
// partner row's own id for the cursor tuple, and the sort's cursor key where
// the sort appends one.
func scanPartnerPage(row pgx.Row, sorted *storekit.ListSort) (partnerRow, ids.UUID, *string, error) {
	var id ids.UUID
	var key *string
	dest := []any{&id}
	if sorted.CursorKeySuffix() != "" {
		dest = append(dest, &key)
	}
	out, err := scanPartner(trailingColumns{row: row, dest: dest})
	return out, id, key, err
}

// trailingColumns scans the columns a paged SELECT appends after the published
// ones, so the row scanner every other caller uses does not have to know they
// are there.
type trailingColumns struct {
	row  pgx.Row
	dest []any
}

func (t trailingColumns) Scan(dest ...any) error { return t.row.Scan(append(dest, t.dest...)...) }

// partnerListWhere builds the WHERE fragments for the partner list: the
// role/cert-status filters, the keyset cursor, and the company's own row scope
// (a partner row is a read of its company, so the company scope bounds
// the list).
func partnerListWhere(ctx context.Context, in ListPartnersInput, sorted *storekit.ListSort, arg func(any) int) ([]string, error) {
	where := []string{"p.archived_at IS NULL"}
	if in.PartnerRole != nil {
		where = append(where, storekit.SQLf("p.partner_role = $%d", arg(*in.PartnerRole)))
	}
	if in.CertStatus != nil {
		where = append(where, storekit.SQLf("p.cert_status = $%d", arg(*in.CertStatus)))
	}
	if in.Cursor != "" {
		// Through the sort's own keyset, which refuses a token minted under a
		// different ordering rather than resuming on an axis this page is not
		// ordered by.
		keyset, err := sorted.KeysetClause(in.Cursor, arg)
		if err != nil {
			return nil, err
		}
		where = append(where, keyset)
	}
	scope, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
	if err != nil {
		return nil, err
	}
	// The company's existence, its archival and its row scope in one clause:
	// a partner row is a read of its company, so the company scope bounds the
	// list, and the alias stays inside where it cannot collide with the
	// partner's own columns.
	company := "EXISTS (SELECT 1 FROM company o WHERE o.id = p.company_id AND o.archived_at IS NULL"
	if scope != "" {
		company += " AND " + scope
	}
	where = append(where, company+")")
	return where, nil
}

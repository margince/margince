// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The prelude every keyset list read shares: validate the sort
// against the record's vocabulary plus its live custom columns, clamp the
// page size, and build the WHERE terms that are the same for every record
// type — the caller's row scope, its custom-field equality filters, and the
// keyset cursor. What differs per record (its columns, its own filters, its
// scanner) stays with the record.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// ListWhereSeed opens a list/read WHERE chain so every optional narrowing
// below it can be appended unconditionally with " AND ".
const ListWhereSeed = "1=1"

// ListPrelude is one list read's validated, scope-bounded starting point.
// It is passed by POINTER throughout: `arg` appends to args, and callers
// keep registering arguments (their own filters) after it is built — a
// value copy would leave those arguments on a dead struct and the query
// short of placeholders.
type ListPrelude struct {
	sorted *ListSort
	limit  int
	where  []string
	args   []any
	arg    func(any) int
}

// Where is the scope-bounded, cursor-bounded WHERE the prelude has assembled.
// A caller appends its own record-specific terms to the returned slice.
func (p *ListPrelude) Where() []string { return p.where }

// Arg registers one query argument and answers its placeholder number, so a
// caller never hand-numbers a $N. The prelude owns the argument slice because
// its own clauses are already in it.
//
//craft:ignore naked-any v is one bind value on its way to pgx, which accepts any Go type its driver can encode (a typed id, a string, a bool, a time.Time, a slice for an ANY(...)); the encoding contract is pgx's, so there is no narrower type to give it here
func (p *ListPrelude) Arg(v any) int { return p.arg(v) }

// BuildListPrelude assembles it, or returns the first refusal — an
// out-of-vocabulary sort field, an unknown cf_ filter, an unreadable cursor.
func BuildListPrelude(
	ctx context.Context,
	object string,
	fields map[string]string,
	active []fieldcatalog.Column,
	sort *string,
	limit *int,
	cursor *string,
	customFilters map[string]string,
) (*ListPrelude, error) {
	sorted, err := ParseListSort(sort, SortVocabulary(fields, active))
	if err != nil {
		return nil, err
	}

	p := &ListPrelude{sorted: sorted, limit: ClampLimit(limit), where: []string{ListWhereSeed}}
	p.arg = func(v any) int { p.args = append(p.args, v); return len(p.args) }

	// A row-scoped record narrows to what this reader may see. The
	// workspace-shared catalogues (products, offer templates) have no such
	// boundary — every seat sees the same rows — and asking for one is an
	// error rather than an empty clause, so they say so by passing "".
	if object != "" {
		scope, err := auth.ScopeClauseFor(ctx, object, "", p.arg)
		if err != nil {
			return nil, err
		}
		if scope != "" {
			p.where = append(p.where, scope)
		}
	}

	cfClauses, err := CustomFilterClauses(active, customFilters, p.arg)
	if err != nil {
		return nil, err
	}
	p.where = append(p.where, cfClauses...)

	if cursor != nil && *cursor != "" {
		clause, err := sorted.KeysetClause(*cursor, p.arg)
		if err != nil {
			return nil, err
		}
		p.where = append(p.where, clause)
	}
	return p, nil
}

// TxRunner opens the transaction a list read runs inside. A module's store
// satisfies it with the pool it is already bound to, so this helper never holds
// a database handle of its own and the workspace binding stays with the caller.
type TxRunner interface {
	Tx(ctx context.Context, fn func(pgx.Tx) error) error
}

// RunListPage executes one prepared list read and turns it into a page:
// fetch limit+1 rows to learn whether another page exists, trim to the
// page, and encode the next cursor from the last row kept. Generic over
// the record type because the shape — not the record — is what repeats;
// `scan` and `key` are the only genuinely per-record parts.
func RunListPage[T any](
	ctx context.Context,
	s TxRunner,
	pre *ListPrelude,
	table, columns string,
	active []fieldcatalog.Column,
	where []string,
	scan func(pgx.Rows, []fieldcatalog.Column, *ListSort) ([]T, []*string, error),
	key func(T) (time.Time, ids.UUID),
	// finish runs over the trimmed page inside the same transaction — the
	// field-mask pass, which needs one more statement over the page's ids.
	finish ...func(pgx.Tx, []T) error,
) ([]T, Page, error) {
	var out []T
	var page Page
	err := s.Tx(ctx, func(tx pgx.Tx) (err error) {
		out, page, err = RunListPageTx(ctx, tx, pre, table, columns, active, where, scan, key, finish...)
		return err
	})
	return out, page, err
}

// RunListPageTx is RunListPage inside a caller-opened transaction — the
// composite record reads, whose every section must describe one instant.
// Same statement, same trim, same finish pass; only the transaction is
// borrowed.
func RunListPageTx[T any](
	ctx context.Context,
	tx pgx.Tx,
	pre *ListPrelude,
	table, columns string,
	active []fieldcatalog.Column,
	where []string,
	scan func(pgx.Rows, []fieldcatalog.Column, *ListSort) ([]T, []*string, error),
	key func(T) (time.Time, ids.UUID),
	finish ...func(pgx.Tx, []T) error,
) ([]T, Page, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+columns+SelectSuffix(active)+pre.sorted.CursorKeySuffix()+
			` FROM `+table+` WHERE `+strings.Join(where, " AND ")+
			pre.sorted.OrderBy()+SQLf(` LIMIT %d`, pre.limit+1),
		pre.args...)
	if err != nil {
		return nil, Page{}, err
	}
	defer rows.Close()
	out, cursorKeys, err := scan(rows, active, pre.sorted)
	if err != nil {
		return nil, Page{}, err
	}
	var page Page
	if len(out) > pre.limit {
		out = out[:pre.limit]
		createdAt, id := key(out[len(out)-1])
		if page, err = nextPage(pre.sorted, cursorKeys[pre.limit-1], createdAt, id); err != nil {
			return nil, Page{}, err
		}
	}
	for _, f := range finish {
		if err := f(tx, out); err != nil {
			return nil, Page{}, err
		}
	}
	if out == nil {
		out = []T{}
	}
	return out, page, nil
}

// nextPage is the page that continues after this row: the flag and the token
// TOGETHER, because either without the other is a lie.
//
// A token that will not mint abandons the read rather than shipping an empty
// one beside HasMore: true, which would tell a client there is another page and
// hand them nothing to fetch it with — a page they can ask for and never
// receive, silent on the server and permanent for that list.
//
// The failure is reachable: time.Time refuses an instant outside years
// 0000-9999 and Postgres timestamptz reaches year 294276, so one absurd row's
// created_at is enough.
func nextPage(sorted *ListSort, sortKey *string, createdAt time.Time, id ids.UUID) (Page, error) {
	next, err := sorted.EncodePageCursor(sortKey, createdAt, id)
	if err != nil {
		return Page{}, err
	}
	return Page{HasMore: true, NextCursor: next}, nil
}

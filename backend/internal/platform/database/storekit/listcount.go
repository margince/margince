// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// MaxListOffset is how many rows a list read may skip. A deeper page is
// reached through the cursor, because OFFSET reads every row it skips.
const MaxListOffset = 10000

// OffsetError refuses a list read's offset, as a 422 on the offset field.
type OffsetError struct {
	Code    string
	Message string
}

func (e *OffsetError) Error() string { return "store: " + e.Message }

// FieldFault names the offset as the input to change.
func (e *OffsetError) FieldFault() (field, code, message string) { return "offset", e.Code, e.Message }

// ListOffset is how many rows a list read skips, 0 when none was asked for.
// A cursor already names where the page starts, so the two are refused together.
func ListOffset(offset *int, cursor *string) (int, error) {
	if offset == nil {
		return 0, nil
	}
	if cursor != nil && *cursor != "" {
		return 0, &OffsetError{Code: "offset_with_cursor", Message: "offset cannot be sent with cursor; send one of the two"}
	}
	if *offset < 0 || *offset > MaxListOffset {
		return 0, &OffsetError{
			Code:    "out_of_range",
			Message: fmt.Sprintf("offset must be between 0 and %d; read deeper pages through the cursor", MaxListOffset),
		}
	}
	return *offset, nil
}

// PageWindow is the LIMIT and OFFSET of one list page. It asks for one row past
// the page, which tells the read whether another page follows.
func PageWindow(limit, offset int) string {
	if offset == 0 {
		return SQLf(" LIMIT %d", limit+1)
	}
	return SQLf(" LIMIT %d OFFSET %d", limit+1, offset)
}

// ListCount is the COUNT(*) behind a page's total: its statement and its bind values.
type ListCount struct {
	sql  string
	args []any
}

// CountList counts a list's rows under the reader's scope on object and terms, on a fresh counter.
// Not the page's clauses: a reference sort binds values a COUNT never reads, and Postgres refuses them.
func CountList(ctx context.Context, object, table string, terms func(arg func(any) int) ([]string, error)) (ListCount, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := []string{ListWhereSeed}
	scope, err := auth.ScopeClauseFor(ctx, object, "", arg)
	if err != nil {
		return ListCount{}, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	own, err := terms(arg)
	if err != nil {
		return ListCount{}, err
	}
	where = append(where, own...)
	return ListCount{sql: "SELECT count(*) FROM " + table + " WHERE " + strings.Join(where, " AND "), args: args}, nil
}

// PageCount is what one page's total needs: where the page starts, and the
// COUNT to build when its rows cannot say.
type PageCount struct {
	Cursor        *string
	Offset, Limit int
	Count         func() (ListCount, error)
}

// Total is how many rows match, nil on a page read through a cursor; fetched
// counts the probe row. Only a full page, or an empty one past an offset, runs the COUNT.
func (c PageCount) Total(ctx context.Context, tx pgx.Tx, fetched int) (*int, error) {
	if c.Cursor != nil && *c.Cursor != "" {
		return nil, nil //nolint:nilnil // a nil total is a read that does not count, as Page.Total says
	}
	if fetched <= c.Limit && (fetched > 0 || c.Offset == 0) {
		n := c.Offset + fetched
		return &n, nil
	}
	count, err := c.Count()
	if err != nil {
		return nil, err
	}
	var matching int
	if err := tx.QueryRow(ctx, count.sql, count.args...).Scan(&matching); err != nil {
		return nil, err
	}
	return &matching, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// HiddenRows is what row scope withheld from one list: how many rows, and the
// colleagues to ask about them. It never carries anything a hidden row holds.
type HiddenRows struct {
	Count   int
	Holders []HiddenHolder
}

// HiddenHolder is one live colleague who owns at least one hidden row.
type HiddenHolder struct {
	UserID      ids.UUID
	DisplayName string
}

// HiddenSection is the one statement a list and its hidden count both read.
// They share the tables and conditions, and differ only by the visibility
// predicate. Two separately written statements would drift, and the count
// would then name rows the list never had.
type HiddenSection struct {
	From    string // tables and aliases, as after `FROM`
	Where   string // every condition but visibility
	Owner   string // the column holding each row's owner
	Visible string // the caller's visibility predicate; "" when nothing is bounded
}

// ListSQL is the section's list statement up to its `ORDER BY`: base rows the
// caller may see. join goes between `FROM` and `WHERE`, for columns only the
// list selects.
func (s HiddenSection) ListSQL(columns, join string) string {
	sql := "SELECT " + columns + " FROM " + s.From + " " + join + " WHERE " + s.Where
	if s.Visible != "" {
		sql += " AND (" + s.Visible + ")"
	}
	return sql
}

// ReadHiddenRows counts the base rows the visibility predicate rejects and
// names their owners. An owner who left, or an agent seat, still counts in
// Count and is never offered as someone to ask. The negation is NULL-safe: a
// predicate that yields NULL for a row did not admit it.
func ReadHiddenRows(ctx context.Context, tx pgx.Tx, s HiddenSection, args []any) (HiddenRows, error) {
	out := HiddenRows{Holders: []HiddenHolder{}}
	if s.Visible == "" {
		return out, nil
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT h.owner_id, u.display_name, count(*)::int
		  FROM (SELECT %s AS owner_id FROM %s WHERE %s AND NOT COALESCE((%s), false)) h
		  LEFT JOIN app_user u ON u.id = h.owner_id AND %s
		 GROUP BY h.owner_id, u.display_name
		 ORDER BY u.display_name NULLS LAST, h.owner_id`,
		s.Owner, s.From, s.Where, s.Visible, workforce.LiveHumanSQL("u")), args...)
	if err != nil {
		return HiddenRows{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var owner *ids.UUID
		var name *string
		var count int
		if err := rows.Scan(&owner, &name, &count); err != nil {
			return HiddenRows{}, err
		}
		out.Count += count
		if owner != nil && name != nil {
			out.Holders = append(out.Holders, HiddenHolder{UserID: *owner, DisplayName: *name})
		}
	}
	return out, rows.Err()
}

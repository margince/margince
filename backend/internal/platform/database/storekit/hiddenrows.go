// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	Entry   string // the column naming one listed entry, counted once however many rows join it
	Owner   string // the column holding each entry's owner
	Visible string // the caller's visibility predicate; "" when nothing is bounded
}

// BoundedBy is the conjunction of the visibility bounds that are set, as a
// HiddenSection's Visible: "" when no bound is set.
func BoundedBy(bounds ...string) string {
	set := make([]string, 0, len(bounds))
	for _, bound := range bounds {
		if bound != "" {
			set = append(set, "("+bound+")")
		}
	}
	return strings.Join(set, " AND ")
}

// VisibleFrom is the section's `FROM` and `WHERE` for the rows the caller may
// see, for a statement that selects its own columns. join goes between `FROM`
// and `WHERE`, for tables only that statement reads.
func (s HiddenSection) VisibleFrom(join string) string {
	sql := " FROM " + s.From + " " + join + " WHERE " + s.Where
	if s.Visible != "" {
		sql += " AND (" + s.Visible + ")"
	}
	return sql
}

// ListSQL is the section's list statement up to its `ORDER BY`.
func (s HiddenSection) ListSQL(columns, join string) string {
	return "SELECT " + columns + s.VisibleFrom(join)
}

// ReadHiddenRows counts the base entries the caller may not see and names
// their owners. An entry the list shows through any one row is not hidden. An
// entry has one owner, so the per-owner counts add up. An owner who left, or
// an agent seat, still counts and is never offered as someone to ask.
func ReadHiddenRows(ctx context.Context, tx pgx.Tx, s HiddenSection, args []any) (HiddenRows, error) {
	out := HiddenRows{Holders: []HiddenHolder{}}
	if s.Visible == "" {
		return out, nil
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT h.owner_id, u.display_name, count(DISTINCT h.entry)::int
		  FROM (SELECT %[1]s AS entry, %[2]s AS owner_id FROM %[3]s WHERE %[4]s
		           AND %[1]s NOT IN (SELECT %[1]s FROM %[3]s WHERE %[4]s AND (%[5]s))) h
		  LEFT JOIN app_user u ON u.id = h.owner_id AND %[6]s
		 GROUP BY h.owner_id, u.display_name
		 ORDER BY u.display_name NULLS LAST, h.owner_id`,
		s.Entry, s.Owner, s.From, s.Where, s.Visible, workforce.LiveHumanSQL("u")), args...)
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

// Wire is the contract shape of what was withheld: a count and who to ask.
func (h HiddenRows) Wire() crmcontracts.HiddenRecords {
	holders := make([]crmcontracts.HiddenRecordHolder, 0, len(h.Holders))
	for _, holder := range h.Holders {
		holders = append(holders, crmcontracts.HiddenRecordHolder{
			UserId:      openapi_types.UUID(holder.UserID),
			DisplayName: holder.DisplayName,
		})
	}
	return crmcontracts.HiddenRecords{Count: h.Count, Holders: holders}
}

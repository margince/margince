// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"

	"github.com/jackc/pgx/v5"
)

func carriedRootSQL(table, alias string, columns ...string) string {
	selected := make([]string, 0, len(columns))
	current := make([]string, 0, len(columns))
	parent := make([]string, 0, len(columns))
	for _, column := range columns {
		name := pgx.Identifier{column}.Sanitize()
		selected = append(selected, name)
		current = append(current, "c."+name)
		parent = append(parent, "p."+name)
	}
	name := pgx.Identifier{table}.Sanitize()
	return `(WITH RECURSIVE chain AS (
	    SELECT c.carried_from, ` + strings.Join(current, ", ") + `, 0 AS depth
	      FROM ` + name + ` c WHERE c.id = ` + pgx.Identifier{alias}.Sanitize() + `.id
	    UNION ALL
	    SELECT p.carried_from, ` + strings.Join(parent, ", ") + `, chain.depth + 1
	      FROM ` + name + ` p JOIN chain ON p.id = chain.carried_from)
	  SELECT ` + strings.Join(selected, ", ") + ` FROM chain ORDER BY depth DESC LIMIT 1)`
}

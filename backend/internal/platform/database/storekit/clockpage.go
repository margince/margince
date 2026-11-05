// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ClockRule names which stored clocks a repair refolds.
type ClockRule int

// The clock rules a repair can apply.
const (
	// ClockInFuture is a stored clock later than now.
	ClockInFuture ClockRule = iota
	// ClockNotFolded is a stored clock that is not what its fold function
	// now gives, whatever older rule left it.
	ClockNotFolded
)

// clockRuleSQL renders rule over the record aliased t. Both arms are
// compile-time text and a constant table name.
func clockRuleSQL(rule ClockRule, table ClockTable) string {
	if rule == ClockNotFolded {
		return `t.last_activity_at IS DISTINCT FROM last_activity_of_` + clockTableIdent(table) + `(t.id)`
	}
	return `coalesce(t.last_activity_at > now(), false)`
}

// ClockPage reads the next limit records of table after `after`, in id order.
// It answers every id it inspected, for the cursor, and the ones rule selects.
// The page is bounded by limit whatever share of the table rule selects.
func ClockPage(ctx context.Context, q ClockQuerier, table ClockTable, rule ClockRule, after ids.UUID, limit int) (inspected, selected []ids.UUID, err error) {
	rows, err := q.Query(ctx, `SELECT t.id, `+clockRuleSQL(rule, table)+` FROM `+clockTableIdent(table)+` t
		 WHERE t.id > $1 ORDER BY t.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("reading a page of %s clocks: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var wrong bool
		if err := rows.Scan(&id, &wrong); err != nil {
			return nil, nil, fmt.Errorf("reading a page of %s clocks: %w", table, err)
		}
		inspected = append(inspected, id)
		if wrong {
			selected = append(selected, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading a page of %s clocks: %w", table, err)
	}
	return inspected, selected, nil
}

// CountClockRecords counts up to maximum+1 records of table: the rows a repair
// inspects, read without testing any of them.
func CountClockRecords(ctx context.Context, q ClockQuerier, table ClockTable, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+clockTableIdent(table)+` LIMIT $1) inspected`,
		maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting %s clocks: %w", table, err)
	}
	return count, nil
}

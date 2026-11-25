// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// arrivedOriginSQL is the origin each lead in $1 arrived with. That is the
// value the first audited change of its origin replaced. A lead whose origin
// never changed arrived with the one it has now. A first change away from no
// origin yields NULL, because the lead arrived without one.
const arrivedOriginSQL = `
	SELECT p.id, CASE WHEN first_change.found THEN first_change.origin ELSE p.origin END AS origin
	  FROM lead p
	  LEFT JOIN LATERAL (
	        SELECT true AS found, a.before ->> '` + leadOriginField + `' AS origin
	          FROM audit_log a
	         WHERE a.entity_type = '` + leadEntity + `' AND a.entity_id = p.id AND a.action = 'update'
	           AND a.before ? '` + leadOriginField + `'
	         ORDER BY a.occurred_at, a.id
	         LIMIT 1) first_change ON true
	 WHERE p.id = ANY($1)`

// BackfillArrivalOriginTx records the arrival origin of leads stored before it
// was kept. It takes up to limit leads after the cursor and returns those it
// inspected.
// The value comes from the audit trail or the lead itself. So the write is not
// audited, the same as the insert that fills it. A lead that arrived with
// no origin stays NULL, so a replay inspects it again and changes nothing.
func BackfillArrivalOriginTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := requireLeadRepair(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM lead WHERE `+arrivalOriginColumn+` IS NULL AND id > $1
		 ORDER BY id LIMIT $2 FOR UPDATE`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading leads with no arrival origin: %w", err)
	}
	inspected, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading leads with no arrival origin: %w", err)
	}
	if len(inspected) == 0 {
		return inspected, 0, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE lead l SET `+arrivalOriginColumn+` = arrived.origin
		  FROM (`+arrivedOriginSQL+`) arrived
		 WHERE l.id = arrived.id AND l.`+arrivalOriginColumn+` IS NULL AND arrived.origin IS NOT NULL`, inspected)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: storing the arrival origin of leads: %w", err)
	}
	return inspected, int(tag.RowsAffected()), nil
}

// CountArrivalOriginBackfills counts what BackfillArrivalOriginTx has yet to
// inspect, reading at most maximum+1.
func CountArrivalOriginBackfills(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM lead WHERE `+arrivalOriginColumn+` IS NULL LIMIT $1) pending`,
		maximum+1).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contacts: counting leads with no arrival origin: %w", err)
	}
	return n, nil
}

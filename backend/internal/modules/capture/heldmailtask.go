// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StampHeldMailTx keeps provider arrival evidence unchanged when its original cannot prove it.
func StampHeldMailTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM capture_import WHERE id > @after ORDER BY id LIMIT @limit FOR UPDATE`, pgx.NamedArgs{"after": after, "limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	tag, err := tx.Exec(ctx, stampHeldMailSQL, pgx.NamedArgs{"ids": scanned})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, int(tag.RowsAffected()))
}

// EstimateHeldMail bounds the rows inspected, not the number that will be parseable.
func EstimateHeldMail(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM capture_import ORDER BY id LIMIT @cap) AS candidates`, pgx.NamedArgs{"cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

const stampHeldMailSQL = `WITH held AS (
    SELECT ci.id,
           CASE
             WHEN jsonb_typeof(rc.payload) = 'string' THEN left(rc.payload #>> '{}', 65536)
             WHEN left(rc.payload ->> 'data', 87380) ~ '^([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$'
             THEN encode(decode(left(rc.payload ->> 'data', 87380), 'base64'), 'escape')
           END AS text_head
      FROM capture_import ci
      JOIN activity a ON a.id = ci.activity_id
      JOIN raw_capture rc ON rc.id = a.raw_capture_id
     WHERE ci.id = ANY(@ids) AND ci.provider_received_at IS NULL
       AND a.kind = 'email' AND a.direction = 'inbound'
       AND a.captured_by IN ('connector:gmail:' || ci.user_id::text, 'connector:graph:' || ci.user_id::text)
       AND (jsonb_typeof(rc.payload) = 'string'
            OR (jsonb_typeof(rc.payload) = 'object' AND rc.payload ->> 'encoding' = 'base64'
                AND jsonb_typeof(rc.payload -> 'data') = 'string'))
       AND NOT EXISTS (SELECT 1 FROM capture_import o
                        WHERE o.activity_id = ci.activity_id AND o.user_id <> ci.user_id)
),
top_received AS (
    SELECT id,
           (regexp_match(substring(text_head FROM '^(.*?)\r?\n\r?\n'),
                         '(?:^|\n)received:((?:[^\n]|\n[ \t])*)', 'i'))[1] AS line
      FROM held
),
stamp AS (
    SELECT id, regexp_match(
             btrim(regexp_replace(regexp_replace((regexp_match(line, ';([^;]*)$'))[1],
                                                 '\([^)]*\)', '', 'g'), '\s+', ' ', 'g')),
             '^(?:[A-Za-z]{3}, ?)?(\d{1,2}) ([A-Za-z]{3}) (\d{4}) (\d{2}):(\d{2})(?::(\d{2}))? ([+-])(\d{2})(\d{2})$') AS m
      FROM top_received
),
parts AS (
    SELECT id, m[1]::int AS d,
           array_position(ARRAY['jan', 'feb', 'mar', 'apr', 'may', 'jun',
                                'jul', 'aug', 'sep', 'oct', 'nov', 'dec'], lower(m[2])) AS mo,
           m[3]::int AS y, m[4]::int AS hh, m[5]::int AS mi, coalesce(m[6], '00')::int AS ss,
           m[7] AS sign, m[8] AS zh, m[9] AS zm
      FROM stamp
     WHERE m IS NOT NULL
),
stamped AS (
    SELECT id, format('%s-%s-%s %s:%s:%s%s%s:%s', y, mo, d, hh, mi, ss, sign, zh, zm)::timestamptz AS at
      FROM parts
     WHERE mo IS NOT NULL AND y BETWEEN 1970 AND 9999 AND hh <= 23 AND mi <= 59 AND ss <= 59
       AND zh::int <= 14 AND zm::int <= 59
       AND d BETWEEN 1 AND extract(day FROM make_date(y, mo, 1) + interval '1 month - 1 day')::int
)
UPDATE capture_import ci
   SET provider_received_at = stamped.at
  FROM stamped
 WHERE ci.id = stamped.id AND ci.provider_received_at IS NULL`

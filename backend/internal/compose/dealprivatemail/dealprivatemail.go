// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealprivatemail reads the dates of a deal's mail that only its
// participants may read, for the deal verdict. Nothing but a date leaves a row:
// no direction, subject, body or address.
package dealprivatemail

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// ChecksumSQL hashes the id, date and update time of the newest limit private
// mails of the deal aliased d, the rows Read hands the verdict. It is NULL for
// a deal with none, so such a deal keeps its stored checksum.
func ChecksumSQL(limit int) string {
	return fmt.Sprintf(`(SELECT md5(string_agg(concat_ws(':', r.id, r.occurred_at, r.updated_at), ',' ORDER BY r.occurred_at DESC, r.id DESC))
		   FROM (SELECT pa.id, pa.occurred_at, pa.updated_at
		           FROM activity_link pl JOIN activity pa ON pa.id = pl.activity_id
		          WHERE pl.deal_id = d.id AND pa.archived_at IS NULL AND pa.kind = 'email'
		            AND %s AND %s
		          ORDER BY pa.occurred_at DESC, pa.id DESC LIMIT %d) r)`,
		auth.PrivateAudienceSQL("pa"), relstrength.OriginIsEngagementSQL("pa"), limit)
}

// Read puts the newest limit private mails of the deal into in, oldest first.
// It moves newest to when the latest of them reached the deal.
func Read(ctx context.Context, tx pgx.Tx, deal ids.UUID, limit int, in *dealverdict.Input, newest *time.Time) error {
	rows, err := tx.Query(ctx, `
		SELECT a.occurred_at, greatest(a.created_at, l.created_at)
		  FROM activity_link l JOIN activity a ON a.id = l.activity_id
		 WHERE l.deal_id = $1 AND a.archived_at IS NULL AND a.kind = 'email'
		   AND `+auth.PrivateAudienceSQL("a")+` AND `+relstrength.OriginIsEngagementSQL("a")+`
		 ORDER BY a.occurred_at DESC, a.id DESC LIMIT $2`, deal, limit)
	if err != nil {
		return fmt.Errorf("compose: reading the deal's private mail dates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var mail dealverdict.PrivateMail
		var arrived time.Time
		if err := rows.Scan(&mail.OccurredAt, &arrived); err != nil {
			return err
		}
		in.PrivateMail = append(in.PrivateMail, mail)
		if arrived.After(*newest) {
			*newest = arrived
		}
	}
	slices.Reverse(in.PrivateMail)
	return rows.Err()
}

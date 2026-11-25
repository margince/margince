// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeriesSplit is a meeting a split series replaced, and the meeting of the
// series that replaced it at the same slot.
type SeriesSplit struct{ Replaced, Survivor ids.ActivityID }

// seriesSplitSQL holds when child belongs to the series Google split off
// parent's series at parent's slot. Both are on seat's calendar. A "this and
// following events" edit names the new series after the old UID's local part.
// Google adds "_R" and the split's start, as in abc_R20261124T090000@google.com.
// Likeness alone never holds: two meetings of one organizer at one slot can
// be two meetings.
func seriesSplitSQL(parent, child, seat string) string {
	p, c := pgx.Identifier{parent}.Sanitize(), pgx.Identifier{child}.Sanitize()
	return p + `.kind = 'meeting' AND ` + c + `.kind = 'meeting' AND ` + p + `.id <> ` + c + `.id
	   AND ` + p + `.source_system = ` + c + `.source_system AND ` + p + `.occurred_at = ` + c + `.occurred_at
	   AND ` + p + `.archived_at IS NULL AND ` + p + `.restricted_at IS NULL
	   AND ` + c + `.archived_at IS NULL AND ` + c + `.restricted_at IS NULL
	   AND ` + p + `.meeting_status IS DISTINCT FROM 'canceled' AND ` + c + `.meeting_status IS DISTINCT FROM 'canceled'
	   AND ` + auth.ConnectorSeatSQL(parent) + ` = ` + seat + `::uuid::text
	   AND ` + auth.ConnectorSeatSQL(child) + ` = ` + seat + `::uuid::text
	   AND EXISTS (SELECT 1 FROM activity_participant po
	                 JOIN activity_participant co ON co.activity_id = ` + c + `.id AND co.role = 'organizer'
	                  AND lower(co.address) = lower(po.address)
	                WHERE po.activity_id = ` + p + `.id AND po.role = 'organizer')
	   AND EXISTS (SELECT 1 FROM activity_identity pi
	                 JOIN activity_identity ci ON ci.activity_id = ` + c + `.id AND ci.identity_kind = 'meeting'
	                WHERE pi.activity_id = ` + p + `.id AND pi.identity_kind = 'meeting'
	                  AND ` + meetingUIDOfKeySQL("ci.identity_key") + ` <> ` + meetingUIDOfKeySQL("pi.identity_key") + `
	                  AND regexp_replace(` + meetingUIDOfKeySQL("ci.identity_key") + `, '_R[0-9]{8}T[0-9]{6}Z?(?=@|$)', '')
	                      = ` + meetingUIDOfKeySQL("pi.identity_key") + `)`
}

// SeriesSplitsTx answers each split that meeting is either side of on seat's
// calendar.
func SeriesSplitsTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID) ([]SeriesSplit, error) {
	rows, err := tx.Query(ctx, `
		SELECT parent.id, child.id FROM activity m
		  JOIN activity other ON other.source_system = m.source_system AND other.occurred_at = m.occurred_at AND other.id <> m.id
		 CROSS JOIN LATERAL (VALUES (m.id, other.id), (other.id, m.id)) pair(parent_id, child_id)
		  JOIN activity parent ON parent.id = pair.parent_id
		  JOIN activity child ON child.id = pair.child_id
		 WHERE m.id = $1 AND `+seriesSplitSQL("parent", "child", "$2")+`
		 ORDER BY parent.id, child.id`, meeting, seat)
	if err != nil {
		return nil, fmt.Errorf("activities: finding the meetings a split series replaced: %w", err)
	}
	splits, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SeriesSplit, error) {
		var s SeriesSplit
		return s, row.Scan(&s.Replaced, &s.Survivor)
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the meetings a split series replaced: %w", err)
	}
	return splits, nil
}

// SeriesSplitStandsTx asks the rule again of one pair, which the caller has
// locked. A replay may have moved or re-keyed either row since it was found.
func SeriesSplitStandsTx(ctx context.Context, tx pgx.Tx, split SeriesSplit, seat ids.UUID) (bool, error) {
	var stands bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity parent, activity child
		                WHERE parent.id = $1 AND child.id = $2 AND `+seriesSplitSQL("parent", "child", "$3")+`)`,
		split.Replaced, split.Survivor, seat).Scan(&stands)
	if err != nil {
		return false, fmt.Errorf("activities: reading whether a split series still replaces a meeting: %w", err)
	}
	return stands, nil
}

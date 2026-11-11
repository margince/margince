// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var errErasureGraphMissing = errors.New("privacy: erasing a pending activity update requires the graph invalidator")

func (e *Eraser) pendingTimelineUpdates(ctx context.Context, tx pgx.Tx, subject ids.ContactID, keys subjectKeys) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE `+
		strings.ReplaceAll(subjectTimelineIDs, "$6", "$3")+` AND EXISTS(
 SELECT 1 FROM event_outbox pending WHERE pending.published_at IS NULL
 AND pending.envelope->>'type'='activity.updated' AND pending.envelope->'entity'->>'type'='activity'
 AND pending.envelope->'entity'->>'id'=a.id::text) ORDER BY a.id`, subject, keys.emails, channelActivityKeys(keys.identities))
	if err != nil {
		return nil, err
	}
	activities, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	if len(activities) > 0 && e.invalidateEdges == nil {
		return nil, errErasureGraphMissing
	}
	return activities, nil
}

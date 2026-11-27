// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A calendar event this seat's calendar matched onto a row another door filed
// is stored under no key of this calendar. A tombstone names only the event id,
// and a moved one-off states a new identity, so neither finds that row. The
// seat's import row records each id its own connection delivered the meeting under.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// meetingOfOwnDelivery finds the meeting this seat's own calendar connection
// delivered under the key's event id, locked, and whether the seat holds it.
// Event ids are only unique within one mailbox, so an id another mailbox
// delivered reaches nothing here.
func (s *Sink) meetingOfOwnDelivery(ctx context.Context, tx pgx.Tx, key connector.NaturalKey) (ids.ActivityID, bool, error) {
	if may, err := s.mayLookPastNaturalKey(ctx, tx, key); err != nil || !may {
		return ids.ActivityID{}, false, err
	}
	keys := storekit.ProviderMessageKeysSQL("$1", "$2", "$4", "$3")
	var own ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT a.id FROM activity a JOIN capture_import i ON i.activity_id=a.id AND i.user_id=$1
 WHERE i.provider_message_ids @> `+keys+` AND cardinality(`+keys+`) > 0
 AND a.kind='meeting' AND a.archived_at IS NULL AND a.restricted_at IS NULL
 ORDER BY a.id LIMIT 1`, actorUserID(ctx), key.SourceSystem, key.SourceID, runConnection(ctx)).Scan(&own)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("capture: finding the meeting a calendar event was delivered as: %w", err)
	}
	return lockedMeetingForSeat(ctx, tx, own)
}

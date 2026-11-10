// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A provider tombstone can name nothing but the mailbox's own event id. When a
// colleague's calendar filed the meeting first, no row carries that id. The
// tombstone holds no iCal UID to find the shared meeting by either. The seat's
// import row records each id its own connection delivered the meeting under.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// meetingOfOwnEventID finds the meeting this seat's own calendar connection
// delivered under the tombstone's event id. Event ids are only unique within
// one mailbox, so an id another mailbox delivered reaches nothing here.
func (s *Sink) meetingOfOwnEventID(ctx context.Context, tx pgx.Tx, key connector.NaturalKey) (ids.ActivityID, bool, error) {
	if may, err := s.identityMayCancel(ctx, tx, key); err != nil || !may {
		return ids.ActivityID{}, false, err
	}
	keys := storekit.ProviderMessageKeysSQL("$1", "$2", "$3")
	var own ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT a.id FROM activity a JOIN capture_import i ON i.activity_id=a.id AND i.user_id=$1
 WHERE i.provider_message_ids @> `+keys+` AND cardinality(`+keys+`) > 0
 AND a.kind='meeting' AND a.archived_at IS NULL AND a.restricted_at IS NULL
 ORDER BY a.id LIMIT 1 FOR UPDATE OF a`, actorUserID(ctx), key.SourceSystem, key.SourceID).Scan(&own)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("capture: finding the meeting a removed calendar event was delivered as: %w", err)
	}
	return own, true, nil
}

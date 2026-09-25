// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a provider-side deletion may destroy.
//
// Its own file rather than a third selector beside the other two, because the
// question is a different one. An exclusion rule and the personal-mail window
// both ask "which of this seat's mail matches a standing decision"; this asks
// about ONE message the owner has just acted on, by the key it was captured
// under. The buckets it sorts into are the same, which is the point — a
// deletion answers to a colleague's claim and the statutory floor exactly as
// every other purge does.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SelectRemovedPurgeTx answers what a provider-side deletion may destroy: the
// one message this seat imported under a natural key, sorted into the same
// three buckets every other purge uses.
//
// The mailbox owner deleting a message in Gmail or Outlook is the clearest
// signal available about mail nobody else has seen, and the narrowest: it says
// something about THAT message and about no other. So the selection is keyed on
// the natural key rather than on a rule, and scoped to the seat whose
// connection reported the removal — a colleague who imported the same message
// keeps it, which falls out of SharedImports without a special case.
//
// A removal reaching a message under the statutory floor lands in Restricted
// and is kept, for the reason the floor exists: an owner's inbox housekeeping
// does not outrank a commercial-records duty, and a Handelsbrief that vanished
// because somebody tidied up would be a records failure wearing a privacy
// control's clothes.
func SelectRemovedPurgeTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID, sourceSystem, sourceID string, floor StatutoryFloor,
) (PurgeSubject, error) {
	var subject PurgeSubject
	if user == ids.Nil || sourceSystem == "" || sourceID == "" {
		return subject, nil
	}
	args := []any{user, sourceSystem, sourceID}
	shielded, args := floor.column(len(args), args)
	rows, err := tx.Query(ctx, `
		SELECT a.id,
		       (a.restricted_at IS NOT NULL OR (`+shielded+`)
		        OR (`+underAnOpenRequest+`)) AS withheld,
		       (SELECT count(*) FROM capture_import o WHERE o.activity_id = a.id) AS importers
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $1
		 WHERE a.source_system = $2 AND a.source_id = $3
		 ORDER BY a.id`, args...)
	if err != nil {
		return subject, fmt.Errorf("capture: selecting what a mailbox-side deletion would destroy: %w", err)
	}
	if err := collectPurgeRows(rows, &subject, "selecting what a mailbox-side deletion would destroy"); err != nil {
		return subject, err
	}
	return subject, nil
}

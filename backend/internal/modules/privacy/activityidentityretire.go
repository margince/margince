// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// retireActivityIdentities drops the external identities a message answered to
// — its Message-ID, its calendar occurrence.
//
// Erasure ARCHIVES the activity rather than deleting it, so the foreign key's
// cascade never fires. Left standing, the identity row is a durable record that
// a message with that Message-ID was here, outliving the erasure that was
// supposed to remove it. A later arrival of the same message also resolves onto
// the emptied row, which both answers wrongly and discloses that the message
// was erased.
//
// Both acts call this, the way the reply verdicts and the request settlements
// beside them are shared. Contact erasure had no such statement at all: the
// activity-content arm deleted these and the Art. 17 cascade never did, so a
// subject's Message-IDs survived their own erasure. Two lists is how that
// happened.
//
// By ID SET rather than by a link walk, because the contact arm reaches mail
// that is linked to nobody — a deferred or still-unsure sender produces
// activities with no contact link — and those are exactly the rows a walk over
// activity_link would leave holding their identities.
func retireActivityIdentities(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_identity WHERE activity_id = ANY($1)`, activityIDs); err != nil {
		return fmt.Errorf("privacy: retiring the erased messages' external identities: %w", err)
	}
	return nil
}

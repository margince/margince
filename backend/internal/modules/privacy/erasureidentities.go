// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The external identities an erased message answered to, retired in ONE place.
//
// Both destructive arms empty an activity's text and both ARCHIVE the row
// rather than deleting it, so the identity table's foreign key never cascades
// and the claim outlives the content it points at. The per-activity arm knew
// that and deleted the rows itself; the subject-scoped Art. 17 cascade kept
// its own list of derived rows and that list went short by exactly this table,
// which is what a second list does.
//
// Here rather than in the table's owner, and that is the architecture rather
// than a preference: a module never imports a sibling, so privacy cannot call
// into activities and the statement has to stand somewhere on this side of the
// line. It is therefore ratified cross-store SQL, declared as such in the table
// ownership waivers — which is also why activities must not grow a second
// spelling of it for nobody to call.
//
// Its own file rather than a helper beside either caller, and the reason is the
// census. erasureCascadeFiles (backend/gates/piisqlreader_test.go) names the
// FILES the PII censuses parse, so a scrub that lives in a named file stays
// legible to them and one folded into a neighbour is a table that reads as
// covered the moment it moves.
//
// Scoped to a SLICE because the two callers hold different sets — one activity
// for the arm that erases a single record's content, the whole redacted
// timeline for the arm that erases a subject — and one statement over ANY($1)
// is the shape that serves both without either keeping a copy.
//
// It deliberately does NOT reach the rows the statutory floor shielded. Those
// keep their content, so they keep the identity that names it: retiring an
// identity whose message still stands would send the next arrival of that
// message to a second row while the first one is right there.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// retireActivityIdentities drops the external identities of activities whose
// content has just been destroyed.
func retireActivityIdentities(ctx context.Context, tx pgx.Tx, activities []ids.UUID) error {
	if len(activities) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM activity_identity WHERE activity_id = ANY($1)`, activities); err != nil {
		return fmt.Errorf("privacy: retiring an erased message's identities: %w", err)
	}
	return nil
}

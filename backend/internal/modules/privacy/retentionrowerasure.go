// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The two executors that DELETE a row rather than scrub one, and why that is
// the same act twice rather than two.
//
// Every other erase in this engine leaves a record standing and empties it: an
// activity keeps its source key and loses its text, a contact keeps its row and
// loses its name. These two tables have no metadata half to keep — the row IS
// the content — so emptying one would leave an empty row saying nothing, and
// the delete is the whole of the action.
//
// What survives each is the record the row hangs off: ai_call keeps its routing
// and spend after its payload goes, and the activity keeps the source key that
// tombstones a replay after its stored original goes. Neither delete is allowed
// to reach that half, which is what makes both of them one-line statements.
//
// They live beside each other rather than in the table file because that file
// is the dispatch and the authorable set; these are the only executors whose
// shape is shared, and the shape is the point.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasePayload deletes the row outright rather than scrubbing it in place —
// unlike activity/erase there is no metadata half of this record left to keep:
// ai_call_payload IS the special-category-adjacent content, and ai_call (the
// metadata row it FK-cascades from) survives untouched. The retention audit entry
// carries no payload bytes, only policy metadata.
func (*RetentionService) erasePayload(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM ai_call_payload WHERE id = $1`, id)
	return err
}

// eraseRawCapture deletes the provider original outright, like erasePayload and
// for the same reason: the row IS the content, and there is no metadata half of
// it to keep. What survives is the activity the selector joined it to, which
// carries the source key the capture natural key tombstones a replay against —
// so what goes is the stored copy, not the fact that the message arrived.
//
// Only erase is registered. Archiving an original would mean keeping it under
// another name, which is what the activity's own archive action already does to
// the record it belongs to.
func (*RetentionService) eraseRawCapture(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM raw_capture WHERE id = $1`, id)
	return err
}

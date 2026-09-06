// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// Whether an overlay-assembled record is this caller's to change.
//
// `writable` rides on the wire so a client draws its edit affordances from the
// server's answer instead of guessing, and absent means NOT writable — the
// fail-closed default a client needs when it cannot tell. The overlay
// assemblers did not set it, so every mirrored record reached the client as
// read-only and a user fully entitled to edit one saw no way to. They do not
// conclude "the flag is absent"; they conclude the product cannot edit the
// incumbent's records.
//
// The answer is a DIFFERENT computation from the native one, not the same one
// applied to another table. Natively the question is ownership, teams and
// record grants; a mirrored row has none of those — it carries no local
// owner_id — and Update gates on the write-back contract plus the seat.
//
// The row-scope term that completes the native answer is already true of any
// record an assembler is filling: the mirror read that produced it is
// visibility-joined, so a row this caller cannot see never reaches here at
// all. Update's third gate says so itself — a row the actor cannot see is
// ErrNotFound there, exactly as on read.
//
// It answers "may this caller change this row", never "will this succeed". A
// write can still lose the incumbent's drift check and come back as version
// skew, which is a fact about the moment rather than about authority — the
// native flag makes the same distinction.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Writable reports whether this caller may update an overlay record of this
// type: the write-back contract supports it AND the seat holds the grant.
//
// ONE computation with two callers — the REST assembler in compose and the
// provider's own. A second spelling is how the two would answer differently
// for one record, and the surface a client sees is whichever it asked.
//
// It deliberately says nothing about CREATE. SupportsWrite answers false for
// every type on that verb today, and a client reading this flag as permission
// to create would offer a button for a write the overlay refuses.
func Writable(ctx context.Context, et datasource.EntityType) bool {
	if !SupportsWrite(WriteUpdate, et) {
		return false
	}
	return auth.Require(ctx, string(et), principal.ActionUpdate) == nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lockPersonForAttach takes the person's row lock before an edge is hung off
// them, so an archive in flight cannot be outrun.
//
// archivePersonRows archives the person AND sweeps their relationships, in one
// transaction. A writer that inserted a relationship without holding the person
// row could commit between the archive's own probe and its sweep, leaving a
// LIVE relationship pointing at an ARCHIVED person — which is the orphan the
// "block rather than orphan" rule exists to prevent (issue #1625).
//
// The lock closes the window rather than narrowing it, and it does so in
// whichever order the two arrive:
//
//   - the archive commits first, and this returns ErrNotFound because LiveOnly
//     does not resolve an archived row. The attach fails, which is the right
//     answer: there is nothing live left to attach to.
//   - the attach commits first, and the archive's sweep — which runs after its
//     own lock on the same row — sees the new relationship and archives it with
//     everything else.
//
// The statement is written here rather than taken from storekit.LockRow, and
// the reason is a rule this must not quietly decide.
//
// LockRow is how a MUTATION of a row begins in this tree — LockRow then
// ApplyLocked — and the write-authority census reads it as exactly that: reach
// it on a shareable record and the function must also take a row-level
// write-authority probe. That census also records, in its own header, the one
// question it deliberately does not settle: auth.EnsureLinkTarget asks whether
// a caller may REFERENCE a record, and "whether `add` needs write authority on
// the thing added TO is a product question UC-E11-08 E2 raises rather than
// settles."
//
// Attaching an edge to a person is precisely that question. This lock does not
// change the person — it holds the row so an archive cannot slip past — and
// borrowing the mutation door would have answered a product question as a side
// effect of a concurrency fix, on eleven call sites, inside a PR about a race.
//
// So it says what it is. LiveOnly's two jobs are kept: the row is locked and
// its liveness is the same read, because a caller that locked and then asked
// separately whether the row was live would have written the same race one
// level down.
func lockPersonForAttach(ctx context.Context, tx pgx.Tx, personID ids.PersonID) error {
	var got ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM person WHERE id = $1 AND archived_at IS NULL FOR UPDATE`,
		personID.UUID).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		// The same refusal storekit.LockRow gives for a row its filter cannot
		// resolve, so an attach onto an archived person reads the way an attach
		// onto a missing one does — which is what it is.
		return apperrors.ErrNotFound
	}
	return err
}

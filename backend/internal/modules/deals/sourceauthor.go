// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Recording who created a deal in the system it was imported from.
//
// The same repair that names the author of an imported activity names the
// author of an imported deal, and for the same reason: an import runs as one
// administrator, so `captured_by` names that one seat on every row it wrote.
// True, and useless as a statement about who did the work.
//
// THE OBJECT AND THE TABLE ARE SPELLED APART. `dealTable` names the table and
// reaches LockRow and EnsureWritable; the RBAC object is the literal "deal" and
// reaches Require, Audit and the storekit helpers. They are the same word today
// and the compiler cannot tell them apart, so passing one where the other
// belongs reads correctly right up until one of the names moves — at which
// point a call starts asking about a permission nobody granted, or files an
// audit row under a table (gates/moduletablespelling_test.go).
//
// SIMPLER THAN THE ACTIVITY STORE. An activity can be under a statutory
// retention hold and outside the caller's audience, so its version of this
// needs a savepoint and a visibility refusal indistinguishable from "no such
// row". A deal has neither.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetDealSourceAuthorTx records the author on one deal, inside the caller's
// transaction, and answers what it did.
//
// IN THE CALLER'S TRANSACTION, deliberately: the repair writes its own ledger
// row for the same record in the same commit, and a ledger that said
// "attributed" about a write that was rolled back would tell the next run to
// skip the record forever.
//
// THROUGH THE MODULE'S OWN PATCH SEAM. applyDealPatchLocked records the
// forecast movement a deal write may have caused, for every writer. This patch
// moves no forecast field, and that is exactly why it goes through the seam
// rather than around it: the seam is what makes the recording true of every
// writer rather than of the ones that remembered
// (gates/dealforecastmovement_test.go).
//
// The refusals are skips with a reason rather than errors, because a batch of
// five hundred walked from another system will always contain a few this
// installation cannot attribute, and failing the request on the first one would
// discard the rest and do it again on every resumed run.
func (s *Store) SetDealSourceAuthorTx(
	ctx context.Context, tx pgx.Tx, id ids.DealID, in storekit.SourceAuthorInput,
) (storekit.SourceAuthorOutcome, string, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	if !in.Named() {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorUnnamedReason, nil
	}
	lock, err := storekit.LockRow(ctx, tx, dealTable, id.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("deal"), nil
	}
	if err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	// The row probe, after the object grant and before the write. The grant says
	// this caller may edit deals at all; this says they may edit THIS one.
	//
	// ITS ErrNotFound IS A SKIP, in the same words the missing row gets above.
	// EnsureWritable asks EnsureVisible first, and a row the caller's scope hides
	// answers "not found" — returned as an error it would abort the batch
	// transaction and discard every row attributed before it.
	if err := auth.EnsureWritable(ctx, tx, dealTable, id.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason("deal"), nil
		}
		return storekit.SourceAuthorSkipped, "", err
	}
	before, reason, err := storekit.AttributableNow(ctx, tx, "deal", id.UUID, in)
	if err != nil || reason != "" {
		return storekit.SourceAuthorSkipped, reason, err
	}
	// DID ANYTHING ACTUALLY CHANGE? Asked here, under the lock taken above,
	// rather than by the caller from a digest beside the row. The repair is
	// resumed and re-run across tens of thousands of records, so a clean second
	// pass offers answers that already stand — and rewriting them would restamp
	// `updated_at` and bump `version` on a whole pipeline for no change, while
	// reporting `applied` for work not done.
	if before.Same(in) {
		return storekit.SourceAuthorUnchanged, "", nil
	}
	p := storekit.NewPatch()
	p.Set("source_author_id", before.ID, in.AuthorID)
	p.Set("source_author_name", before.Name, in.AuthorName)
	// Applied through the lock rather than a version comparison: the wire takes
	// no If-Match, because the repair walks records the caller enumerated in
	// another system and holds no version for any of them. The row is already
	// held FOR UPDATE above, and the lock is the guarantee — a nil pinned
	// version would instead read as a caller who HAD one and declined it.
	if err := applyDealPatchLocked(ctx, tx, p, lock); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("deals: attributing the deal: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "import", "deal", id.UUID, p.Before(), p.After()); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("deals: auditing the attribution: %w", err)
	}
	return storekit.SourceAuthorApplied, "", nil
}

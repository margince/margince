// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Recording who created a project in the system it was imported from.
//
// The sibling of the deal and contact stores of the same name, and the same
// shape for the same reason: an import runs as one administrator, so
// `captured_by` names that seat on every row it wrote, and the author column is
// the only thing on the row that knows who actually did the work.
//
// A project carries no retention hold and no per-row audience, so this is the
// ordinary lock and the ordinary patch — the savepoint and the visibility
// refusal that activities.SetSourceAuthorTx needs have no subject here.

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

// SetProjectSourceAuthorTx records the author on one project, inside the
// caller's transaction, and answers what it did.
//
// In the caller's transaction so the repair's ledger row and this write stand
// or fall together: a ledger that said "attributed" about a rolled-back write
// would tell the next run to skip the record forever.
func (s *Store) SetProjectSourceAuthorTx(
	ctx context.Context, tx pgx.Tx, id ids.ProjectID, in storekit.SourceAuthorInput,
) (storekit.SourceAuthorOutcome, string, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionUpdate); err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	if !in.Named() {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorUnnamedReason, nil
	}
	lock, err := storekit.LockRow(ctx, tx, projectObject, id.UUID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return storekit.SourceAuthorSkipped, storekit.SourceAuthorMissingReason(projectObject), nil
	}
	if err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	// The row probe, after the object grant and before the write. The grant says
	// this caller may edit projects at all; this says they may edit THIS one.
	if err := auth.EnsureWritable(ctx, tx, projectObject, id.UUID); err != nil {
		return storekit.SourceAuthorSkipped, "", err
	}
	before, reason, err := storekit.AttributableNow(ctx, tx, projectObject, id.UUID, in)
	if err != nil || reason != "" {
		return storekit.SourceAuthorSkipped, reason, err
	}
	if before.Same(in) {
		return storekit.SourceAuthorUnchanged, "", nil
	}
	p := storekit.NewPatch()
	p.Set("source_author_id", before.ID, in.AuthorID)
	p.Set("source_author_name", before.Name, in.AuthorName)
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("projects: attributing the project: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "import", projectObject, id.UUID, p.Before(), p.After()); err != nil {
		return storekit.SourceAuthorSkipped, "", fmt.Errorf("projects: auditing the attribution: %w", err)
	}
	return storekit.SourceAuthorApplied, "", nil
}

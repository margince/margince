// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The gate every write to the tag VOCABULARY passes.
//
// A vocabulary write is workspace-wide by construction: renaming, retiring,
// restoring or folding a word changes what every record carrying it is filed
// under, including records the caller cannot see or write through any other
// route. So the object grant is not the whole question — the caller also has
// to be unbounded.
//
// It is not reachable through the seed: every role holding `tag.update` today
// (admin, ops) is row_scope=all. But object grants are editable per role, and
// the contract documents that as intentional — so an admin granting
// `tag: {update: true}` to a team-scoped role would mint exactly the caller
// this refuses. The cheap moment to make an invariant true is before somebody
// relies on it being false.
//
// ONE gate rather than a check at each of the four writes: four call sites is
// how the fifth vocabulary write ships without it. Per-RECORD tag writes are a
// different question and keep their own answer — ApplyTag and RemoveTag gate
// on write authority over the record being tagged (#3755), which is a bounded
// caller's honest reach and is deliberately not narrowed here.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func requireVocabularyAuthority(ctx context.Context, action principal.Action) error {
	if err := auth.Require(ctx, "tag", action); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	if !auth.Unbounded(actor) {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

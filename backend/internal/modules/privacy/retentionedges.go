// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EdgeInvalidator re-folds the relationship aggregates an activity fed, inside
// the caller's transaction.
//
// It is a seam because the fold lives in the search module, and a module never
// imports a sibling. Compose injects the real one. A second copy of the fold
// here would drift: one such copy deleted the pairs that lost all evidence. It
// left every surviving pair counting the removed interaction.
type EdgeInvalidator func(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error

// WithEdgeInvalidator returns a service that keeps the relationship graph true
// as retention removes the interactions under it, in the same transaction.
//
// The consumer of `retention.applied` recomputes too, and a recompute is
// idempotent. That consumer is the backstop: a bus can fall behind, and the
// correction is a deletion obligation, so this call is the guarantee.
func (s *RetentionService) WithEdgeInvalidator(fn EdgeInvalidator) *RetentionService {
	out := *s
	out.invalidateEdges = fn
	out.eraser = out.eraser.WithEdgeInvalidator(fn)
	return &out
}

// invalidateGraph runs the injected invalidator, if the role wired one. A role
// that did not is not broken: the consumer still corrects the aggregate on the
// next delivery.
func (s *RetentionService) invalidateGraph(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if s.invalidateEdges == nil {
		return nil
	}
	return s.invalidateEdges(ctx, tx, []ids.UUID{id})
}

// WithEdgeInvalidator keeps activity certificates and their graph corrections atomic.
func (e *Eraser) WithEdgeInvalidator(fn EdgeInvalidator) *Eraser {
	clone := *e
	clone.invalidateEdges = fn
	return &clone
}

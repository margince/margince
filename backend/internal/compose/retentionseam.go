// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The nightly retention service and the seams it owes, assembled in one place.
//
// privacy.NewRetentionService builds a service that can evaluate policies; it
// cannot, on its own, finish what its destructive actions promise. Two of those
// obligations live in other modules — the relationship fold in search, the
// provider-original purge in capture — and a module never imports a sibling, so
// each arrives as a function this file injects.
//
// It is one assembler rather than a builder each caller runs because the two
// seams are not equivalent, and the difference is the reason this file exists.
// A missing edge invalidator degrades: the bus consumer handling
// retention.applied corrects the aggregate late. A missing raw-capture purger
// does not degrade, it silently keeps the verbatim provider original of a
// message whose retention window closed — which is what the service did until a
// review of the erasure beside it went looking. privacy refuses that
// configuration now, and this is the one place that satisfies it.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/capture"
	"github.com/gradionhq/margince/backend/internal/modules/privacy"
	"github.com/gradionhq/margince/backend/internal/modules/search"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// NewRetentionServiceFor assembles the retention service for one database
// binding, with every seam its actions depend on.
//
// blob may be nil in a deployment with no object store, where no attachment
// object can exist for the erase action to purge. The other two seams have no
// such "there is nothing to do" case and are always wired.
func NewRetentionServiceFor(db *database.DB, blob blobstore.Store, log *slog.Logger) *privacy.RetentionService {
	pending := capture.NewPendingStore(db)
	return privacy.NewRetentionService(db, blob, log).
		WithEdgeInvalidator(func(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
			return search.RecomputeEdgesForActivities(ctx, tx, []ids.UUID{activityID})
		}).
		WithRawCapturePurger(pending.PurgeRawCaptureTx)
}

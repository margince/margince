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
// It is one assembler rather than a builder each caller runs so that adding a
// seam is one edit rather than a hunt: six integration tests each wired their
// own service before this, and one of them claimed to be "wired exactly as the
// worker wires it" — a claim that had to be re-earned by hand every time and
// would have gone false without failing.
//
// Why the two seams are not equivalent is on privacy.RawCapturePurger, beside
// the type that carries the obligation.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NewRetentionServiceFor assembles the retention service for one database
// binding, with every seam its actions depend on.
//
// blob may be nil in a deployment with no object store, where no attachment
// object can exist for the erase action to purge. The other two seams have no
// such "there is nothing to do" case and are always wired.
func NewRetentionServiceFor(db *database.DB, blob blobstore.Store, log *slog.Logger) *privacy.RetentionService {
	return privacy.NewRetentionService(db, blob, log, RawCapturePurgerFor(db)).
		WithEdgeInvalidator(func(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
			return search.RecomputeEdgesForActivities(ctx, tx, []ids.UUID{activityID})
		})
}

// RawCapturePurgerFor is capture's purge, as the seam privacy erases through.
//
// THREE paths destroy an activity's content and every one of them needs it: the
// nightly erase action, a statutory floor expiring, and a controller's release. Each is reached without an Art. 17 request, so none can
// lean on the person-scoped purge the erasure cascade does — and a path wired
// without it destroys the parsed text while the verbatim original stands,
// joined on the pair the erasure deliberately keeps, and an Art. 15 export
// serves it back.
//
// The join those rows are found by belongs to capture, which writes them. A
// copy of it in privacy would be a second answer to the same question, and the
// half that drifted would leave originals behind while reporting success.
func RawCapturePurgerFor(db *database.DB) privacy.RawCapturePurger {
	return capture.NewPendingStore(db).PurgeRawCaptureTx
}

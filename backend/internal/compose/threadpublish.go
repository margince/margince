// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const threadRepairActor = capture.ThreadRepairActor

func threadAttributionReconciler(pool *pgxpool.Pool) func(context.Context, pgx.Tx, ids.ActivityID, ids.UUID) error {
	sink := capture.NewSink(InstallationDB(pool)).WithProjectAttribution(capture.ProjectAttribution{
		Keys: ProjectsStore(pool), Stamp: activities.StampCorrespondenceForProject,
	}).WithDealAttribution(capture.DealAttribution{
		Candidates: deals.NewStore(InstallationDB(pool), DealsInstallation()), Filer: ActivityDealFiler{},
	})
	authority := identity.NewService(pool)
	return func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID) error {
		ctx = threadRepairContext(ctx)
		return sink.ReconcileThreadAttributionSeatTx(ctx, tx, id, seat, authority)
	}
}

func threadRepairContext(ctx context.Context) context.Context {
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: threadRepairActor})
}

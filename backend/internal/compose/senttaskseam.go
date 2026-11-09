// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func sentPartySettlement(pool *pgxpool.Pool) func(context.Context, ids.UUID) error {
	db := InstallationDB(pool)
	tasks := activities.NewStore(db)
	return comms.NewStore(db, time.Now, tasks, comms.WithPartySettlement(func(ctx context.Context, tx pgx.Tx, email ids.ActivityID) error {
		_, err := tasks.CompleteContactTasksAnsweredByTx(principal.SystemActing(ctx, "system:task-completion"), tx, email)
		return err
	})).SettlePartyAdmission
}

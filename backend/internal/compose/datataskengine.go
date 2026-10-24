// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

func newDataTaskEngine(pool *pgxpool.Pool, release string, now func() time.Time) (*datatask.Engine, error) {
	return datatask.New(pool, dataTaskCatalog(pool, now), release, now, dataTaskFailureNotice(pool))
}

func dataTaskFailureNotice(pool *pgxpool.Pool) func(context.Context, pgx.Tx, datatask.Progress) error {
	return func(ctx context.Context, tx pgx.Tx, p datatask.Progress) error {
		ctx = datatask.Acting(ctx, p.TaskID)
		recipients, err := identity.DataTaskOperatorsTx(ctx, tx)
		if err != nil {
			return err
		}
		store := notices.NewStore(InstallationDB(pool))
		for _, recipient := range recipients {
			_, err := store.CreateTx(ctx, tx, notices.NewNotice{
				Recipient: recipient, Kind: "data_task_failed",
				Subject: "A background correction needs attention", Body: "A correction stopped after repeated failures. Open System health to review its status and retry it.",
				DedupeKey: fmt.Sprintf("%s:%s:%d", p.RunID, p.Step, p.Version),
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func dataTaskCatalog(pool *pgxpool.Pool, now func() time.Time) []datatask.Task {
	return []datatask.Task{
		heldMailDataTask(now), homeRepairDataTask(pool, now), mailRecipientsDataTask(), machineAddressDataTask(), pastActivityClocksDataTask(),
		unaskedCommitmentsDataTask(), colleagueSenderDataTask(), deliveryReportsDataTask(pool, now), liftedAgentCallsDataTask(), mailDealsDataTask(pool), mailBodyCompletenessDataTask(),
		machineSenderDataTask(), signatureTitleHistoryDataTask(pool), manualSignalNoEvidenceDataTask(), companyIndustryDataTask(), leadScoresFollowOriginDataTask(now),
	}
}

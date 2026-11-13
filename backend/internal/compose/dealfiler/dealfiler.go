// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealfiler files captured mail on a deal through the activities
// module, for capture and its repairs.
package dealfiler

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Activities is capture.DealFiler over the activities module, which
// owns activity_link and the relink event. Capture and its repair write
// through it.
type Activities struct{}

// FileOnDealIfUnfiled is activities.FileOnDealIfUnfiledTx.
func (Activities) FileOnDealIfUnfiled(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any,
) (landed, filed bool, err error) {
	outcome, err := activities.FileOnDealIfUnfiledTx(ctx, tx, id, deal, evidence)
	return outcome == activities.DealFilingLanded, outcome == activities.DealFilingAlreadyFiled, err
}

// StampDealTopic is activities.StampDealTopicCheckedTx. A held or archived
// mail keeps no memo, and needs none: no pass files it.
func (Activities) StampDealTopic(ctx context.Context, tx pgx.Tx, id ids.ActivityID, digest string) error {
	_, err := activities.StampDealTopicCheckedTx(ctx, tx, id, digest)
	return err
}

// RetractMachineDeal is activities.RetractMachineDealTx.
func (Activities) RetractMachineDeal(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any,
) (bool, error) {
	return activities.RetractMachineDealTx(ctx, tx, id, deal, evidence)
}

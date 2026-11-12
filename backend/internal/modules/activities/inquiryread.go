// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// WebsiteInquiryTask recognizes both writers across later changes to the linked record.
func (s *Store) WebsiteInquiryTask(ctx context.Context, submission string) (crmcontracts.Activity, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	var task crmcontracts.Activity
	var found bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		leadSystem, leadKey := provenance.WebsiteInquiryTaskSource(submission, false)
		contactSystem, contactKey := provenance.WebsiteInquiryTaskSource(submission, true)
		rows, err := tx.Query(ctx, `SELECT id FROM activity WHERE kind='task' AND ((source_system=@lead_system AND source_id=@lead_key) OR (source_system=@contact_system AND source_id=@contact_key)) LIMIT 2`, pgx.NamedArgs{"lead_system": leadSystem, "lead_key": leadKey, "contact_system": contactSystem, "contact_key": contactKey})
		if err != nil {
			return err
		}
		matches, err := pgx.CollectRows(rows, pgx.RowTo[ids.ActivityID])
		if err != nil {
			return err
		}
		found = len(matches) != 0
		if len(matches) > 1 {
			return apperrors.ErrConflict
		}
		if !found {
			return nil
		}
		task, err = readActivityContent(ctx, tx, matches[0], storekit.IncludeArchived)
		return err
	})
	if err == nil && found {
		err = s.attachCustomValues(ctx, &task)
	}
	return task, found, err
}

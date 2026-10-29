// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func signaturePhonesDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793267775_signature_phones_without_evidence", Version: 1,
		Title: "Taking back signature numbers without evidence", Requires: []string{introductionValidationMigration},
		Steps: []datatask.Step{{
			Name: "signature_phones", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateSignaturePhones, Batch: retractUnsupportedSignaturePhonesBatch,
		}},
	}
}

func retractUnsupportedSignaturePhonesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.RetractUnsupportedSignaturePhonesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
}

func estimateSignaturePhones(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountSignaturePhones(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

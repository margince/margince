// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storedMailCredentialScope covers the capture path and the send path alike,
// since both wrote mail text before either of them masked it.
const storedMailCredentialScope = `FROM activity a WHERE a.kind = 'email'`

// storedCredentialsMaskedDataTask masks the credentials stored in mail text.
//
// Mail written before the capture-side mask holds passwords and access keys in
// clear, where the record shows them and search finds them. The rows are
// repaired in place, and the activity.updated each one emits rebuilds what was
// derived from the old text.
//
// The audit images and the stored provider payload hold the same credentials,
// and neither is in scope here.
func storedCredentialsMaskedDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793640000_stored_credentials_masked", Version: 1,
		Title: "Masking credentials already stored in mail",
		Steps: []datatask.Step{{
			Name: "mask_mail_text", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateStoredMailCredentials,
			Batch:    maskStoredMailCredentials,
		}},
	}
}

func estimateStoredMailCredentials(
	ctx context.Context, q datatask.Querier, maximum int64,
) (int64, bool, error) {
	return countStoredMail(ctx, q, storedMailCredentialScope, maximum)
}

// maskStoredMailCredentials walks the mail by id and masks a page of it.
//
// The cursor advances over every row read, not only the changed ones. Mail
// holding no credential is then not read again on the next pass.
func maskStoredMailCredentials(
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn,
) (datatask.BatchOut, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionUpdate); err != nil {
		return datatask.BatchOut{}, err
	}
	var after ids.UUID
	if len(in.Cursor) > 0 {
		if err := json.Unmarshal(in.Cursor, &after); err != nil {
			return datatask.BatchOut{}, fmt.Errorf("compose: reading the mail mask cursor: %w", err)
		}
	}
	page, err := activities.MaskMailCredentialsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	cursor, err := json.Marshal(page.Last)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: writing the mail mask cursor: %w", err)
	}
	return datatask.BatchOut{
		Cursor: cursor, Scanned: page.Scanned, Changed: page.Changed,
		Done: page.Scanned < in.Limit,
	}, nil
}

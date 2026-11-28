// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func resetNativeLinkCoverage(ctx context.Context, tx pgx.Tx, link Link) error {
	var cursor *string
	var started, coverage, refreshed *time.Time
	var at time.Time
	if err := tx.QueryRow(ctx, `SELECT native_cursor,native_scan_started_at,native_coverage_at,native_refresh_at,now() FROM chat_link WHERE id=@id`, pgx.StrictNamedArgs{"id": link.ID}).Scan(&cursor, &started, &coverage, &refreshed, &at); err != nil {
		return err
	}
	if changed, err := tx.Exec(ctx, `UPDATE chat_link SET native_cursor=NULL,native_scan_started_at=NULL,native_coverage_at=NULL,native_refresh_at=@at WHERE id=@id AND archived_at IS NULL AND version=@version`, pgx.StrictNamedArgs{"id": link.ID, "at": at, versionKey: link.Version}); err != nil {
		return err
	} else if changed.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	_, err := storekit.Audit(ctx, tx, "update", chatLinkEntity, link.ID, map[string]any{"native_cursor": cursor, "native_scan_started_at": started, "native_coverage_at": coverage, "native_refresh_at": refreshed}, map[string]any{"native_cursor": nil, "native_scan_started_at": nil, "native_coverage_at": nil, "native_refresh_at": at})
	return err
}

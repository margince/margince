// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RootScan binds durable discovery progress to one link and scan window.
type RootScan struct {
	Cursor   string
	Started  time.Time
	Coverage *time.Time
}

// RootState retains the source revision represented by a filed recap.
type RootState struct {
	Error    *string
	Retry    *time.Time
	ID       ids.UUID
	Root     string
	Revision string
	Applied  *string
	Digest   *string
	Activity *ids.UUID
	Deleted  bool
	Notice   *string
}

// StartRootScan opens a durable scan window before source pages are consumed.
func (s *Store) StartRootScan(ctx context.Context, link Link, at time.Time) (RootScan, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return RootScan{}, err
	}
	var out RootScan
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockScanLinkTx(ctx, tx, link.ID); err != nil {
			return err
		}
		if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `UPDATE chat_link SET native_scan_started_at=coalesce(native_scan_started_at,@at)
   WHERE id=@id AND archived_at IS NULL AND version=@version RETURNING coalesce(native_cursor,''),native_scan_started_at,native_coverage_at`, pgx.StrictNamedArgs{"at": at, "id": link.ID, versionKey: link.Version}).Scan(&out.Cursor, &out.Started, &out.Coverage)
	})
	return out, err
}

// RecordRootPage persists discovered roots together with the scan checkpoint.
func (s *Store) RecordRootPage(ctx context.Context, link Link, scan RootScan, page chatwire.RootPage) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if page.Next != "" && page.Next == scan.Cursor {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockScanLinkTx(ctx, tx, link.ID); err != nil {
			return err
		}
		if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		for _, root := range page.Roots {
			if err := stageRootTx(ctx, tx, link, root); err != nil {
				return err
			}
		}

		tag, err := tx.Exec(ctx, `UPDATE chat_link SET native_cursor=nullif(@cursor,''),
   native_coverage_at=CASE WHEN @next='' THEN native_scan_started_at ELSE native_coverage_at END,
   native_scan_started_at=CASE WHEN @next='' THEN NULL ELSE native_scan_started_at END,
   backfill_requested=CASE WHEN @next='' THEN false ELSE backfill_requested END
   WHERE id=@id AND archived_at IS NULL AND version=@version AND coalesce(native_cursor,'')=@before AND native_scan_started_at=@started`, pgx.StrictNamedArgs{"next": page.Next, "cursor": rootPageCursor(page), "id": link.ID, "before": scan.Cursor, "started": scan.Started, versionKey: link.Version})
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrConflict
		}
		return nil
	})
}

func rootPageCursor(page chatwire.RootPage) string {
	if page.Next != "" {
		return page.Next
	}
	return page.Checkpoint
}

// RootsToRefresh finds source roots whose recap coverage needs revalidation.
func (s *Store) RootsToRefresh(ctx context.Context, link Link, before time.Time) ([]RootState, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var out []RootState
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+rootColumns+`
   FROM chat_root WHERE link_id=@link AND (retry_at IS NULL OR retry_at<=@at) AND (notice_revision IS NOT NULL OR applied_revision IS DISTINCT FROM revision OR updated_at<=greatest(@before,(SELECT native_refresh_at FROM chat_link WHERE id=@link)))
   ORDER BY (notice_revision IS NOT NULL OR applied_revision IS DISTINCT FROM revision) DESC,updated_at,id LIMIT 25`, pgx.StrictNamedArgs{linkKey: link.ID, "before": before, "at": before.Add(time.Hour)})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanRootState)
		return err
	})
	return out, err
}

// CompleteRootTx commits recap coverage with the source state it represents.
func (s *Store) CompleteRootTx(ctx context.Context, tx pgx.Tx, link Link, root RootState, digest string, activity *ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
		return err
	}
	args := rootExpected(root, link.ID)
	args["digest"], args["activity"] = digest, activity
	tag, err := tx.Exec(ctx, `UPDATE chat_root SET applied_revision=revision,content_digest=@digest,
  activity_id=coalesce(@activity,activity_id),updated_at=now(),notice_revision=NULL,last_error=NULL,retry_at=NULL
  WHERE id=@id AND link_id=@link AND `+rootUnchangedSQL, args)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	after := root
	after.Applied = &root.Revision
	after.Notice = nil
	after.Error, after.Retry = nil, nil
	after.Digest = &digest
	if activity != nil {
		after.Activity = activity
	}
	_, err = storekit.Audit(ctx, tx, "update", "chat_root", root.ID, rootImage(root), rootImage(after))
	return err
}

func lockScanLinkTx(ctx context.Context, tx pgx.Tx, link ids.UUID) error {
	var id ids.UUID
	return tx.QueryRow(ctx, `SELECT id FROM chat_link WHERE id=@id FOR UPDATE`, pgx.StrictNamedArgs{"id": link}).Scan(&id)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOutstandingApprovalScanRequiresSystemAndBoundedPages(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.OutstandingApprovalNotices(e.asUser(e.recipient), nil, ids.Nil, 200); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("human-wide notice scan returned %v", err)
	}
	for _, limit := range []int{-1, 0, 1001} {
		if _, err := e.store.OutstandingApprovalNotices(e.engineCtx(), nil, ids.Nil, limit); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("page limit %d returned %v", limit, err)
		}
	}
	tx, err := e.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := e.store.OutstandingApprovalNotices(e.engineCtx(), tx, ids.Nil, 200)
	if !errors.Is(err, pgx.ErrTxClosed) || !page.AfterID.IsZero() || len(page.ApprovalIDs) != 0 {
		t.Fatalf("failed scan advanced its cursor: page=%+v err=%v", page, err)
	}
}

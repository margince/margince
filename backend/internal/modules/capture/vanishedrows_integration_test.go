// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// vanishedRowsTx runs body in a workspace transaction it rolls back, so each
// spec sees only the rows it seeds.
func vanishedRowsTx(t *testing.T, body func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()
	ctx, pool := meetingTenureEnv(t)
	settled := errors.New("rolled back by the spec")
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		body(ctx, tx)
		return settled
	})
	if !errors.Is(err, settled) {
		t.Fatal(err)
	}
}

// A connection disconnected or erased between scheduling and running has no
// row left to read; the run answers not-found rather than reading nothing.
func TestARunOnAConnectionThatIsGoneAnswersNotFound(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	ws, _ := principal.WorkspaceID(ctx)
	r := NewRegistry(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)), nil, nil, nil)
	gone := ids.NewV7()
	if err := r.syncConnection(ctx, gone); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a sync answered %v, want not-found", err)
	}
	if err := r.RenewWatch(ctx, gone, "projects/demo/topics/mail"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a watch renewal answered %v, want not-found", err)
	}
	done, _, _, err := r.executeBackfillPage(ctx, gone)
	if !errors.Is(err, apperrors.ErrNotFound) || !done {
		t.Errorf("a history page answered done=%v, %v; want a finished run and not-found", done, err)
	}
}

// The watermark and its anchor are one act. When the row moved under the
// open transaction, the release fails the whole write rather than leaving a
// stale anchor behind a stored cursor.
func TestAnAnchorThatCannotBeReleasedFailsTheWatermark(t *testing.T) {
	vanishedRowsTx(t, func(ctx context.Context, tx pgx.Tx) {
		if err := releaseSyncAnchor(ctx, tx, ids.NewV7(), 1); !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("releasing the anchor of a moved row answered %v, want a conflict", err)
		}
	})
}

// A shared mailbox run has no member behind it, so the row itself is the
// authority; without it the write is refused.
func TestASharedMailboxRunWithoutItsRowMayNotWrite(t *testing.T) {
	vanishedRowsTx(t, func(ctx context.Context, tx pgx.Tx) {
		if err := sharedMailboxAllowedTx(ctx, tx, ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a run of a missing shared mailbox answered %v, want a permission refusal", err)
		}
	})
}

// An admin edit is checked against the mailbox it read. If the row changed in
// between, the edit is a conflict and not a silent write over the newer state.
func TestASharedMailboxEditRacedByAnotherIsAConflict(t *testing.T) {
	vanishedRowsTx(t, func(ctx context.Context, tx pgx.Tx) {
		team, reader := ids.NewV7(), ids.NewV7()
		seed := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Reader')`, []any{reader, "reader-" + reader.String() + "@seller.example"}},
			{`INSERT INTO team (id, name) VALUES ($1, $2)`, []any{team, "Support " + team.String()}},
			{`INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, []any{team, reader}},
		}
		for _, s := range seed {
			if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
				t.Fatalf("seeding: %v", err)
			}
		}
		read := SharedMailboxView{ID: ids.NewV7(), ReaderTeam: ids.NewV7(), BackupOwner: &reader}
		if _, err := moveSharedReadersTx(ctx, tx, read, team); !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("moving the readers of a changed mailbox answered %v, want a conflict", err)
		}
		if _, err := changeBackupOwnerTx(ctx, tx, read, SharedChange{ClearBackupOwner: true}); !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("clearing the backup owner of a changed mailbox answered %v, want a conflict", err)
		}
	})
}

// Re-reading stored mail as sent mail writes on the seat's behalf. It needs a
// pass to narrow to that seat, and with none it refuses before any write.
func TestReclaimingSentMailWithoutABoundPassIsRefused(t *testing.T) {
	vanishedRowsTx(t, func(ctx context.Context, tx pgx.Tx) {
		seat := ids.NewV7()
		address := "seat-" + seat.String() + "@seller.example"
		if _, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Seat')`,
			seat, "login-"+seat.String()+"@seller.example"); err != nil {
			t.Fatalf("seeding the seat: %v", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO capture_connection (provider, user_id, status, account_label)
			VALUES ('gmail', $1, 'connected', $2)`, seat, address); err != nil {
			t.Fatalf("seeding the seat's mailbox: %v", err)
		}
		ws, _ := principal.WorkspaceID(ctx)
		passless := principal.WithWorkspaceID(context.Background(), ws)
		row := StoredOwnSentMail{Activity: ids.From[ids.ActivityKind](ids.NewV7()), Seat: seat, Sender: address}
		original := []byte("From: " + address + "\r\nTo: buyer@buyer.example\r\n\r\nHello.\r\n")
		if _, err := ReclaimStoredOwnSentMailTx(passless, tx, nil, nil, row, original); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("reclaiming with no pass answered %v, want a permission refusal", err)
		}
	})
}

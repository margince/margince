// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A notice with an unknown kind, no subject, or no acting principal is refused
// before anything is written.
func TestANoticeThatIsNotWellFormedIsRefused(t *testing.T) {
	e := setupNotices(t)
	if err := e.store.db.Tx(e.engineCtx(), func(tx pgx.Tx) error {
		if _, _, err := e.store.CreateTxNew(e.engineCtx(), tx, NewNotice{Kind: "no-such-kind", Subject: "s", Recipient: e.recipient}); err == nil {
			t.Error("an unknown kind was accepted")
		}
		if _, _, err := e.store.insertNoticeTx(e.engineCtx(), tx, NewNotice{Recipient: e.recipient}, nil); err == nil {
			t.Error("a notice with no kind or subject was accepted")
		}
		bare := context.Background()
		if _, _, err := e.store.insertNoticeTx(bare, tx, NewNotice{Kind: "x", Subject: "s", Recipient: ids.New[ids.UserKind]()}, nil); err == nil {
			t.Error("a notice with no acting principal was accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

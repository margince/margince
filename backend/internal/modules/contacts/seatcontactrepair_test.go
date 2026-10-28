// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The contact step archives on the system's authority alone, and its count
// needs the contact read grant. Both refuse before they read anything.
func TestTheSeatContactRepairRefusesAnyoneButTheSystem(t *testing.T) {
	if _, err := ArchiveSeatContactsTx(context.Background(), nil, ids.Nil, 10); err == nil {
		t.Error("a caller with no principal archived seat contacts")
	}
	if _, err := CountLiveContacts(context.Background(), nil, 10); err == nil {
		t.Error("a caller with no principal counted the live contacts")
	}
}

// unreachableTx is a database that answers no statement.
type unreachableTx struct {
	pgx.Tx
	err error
}

func (tx unreachableTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, tx.err
}

func seatRepairActor(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:seat_repair_test",
		Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
	})
}

// A page the database cannot read stops the step and says what it was reading.
func TestTheSeatContactRepairStopsOnAPageItCannotRead(t *testing.T) {
	ctx := seatRepairActor(map[string]principal.ObjectGrant{entityContact: {Read: true, Delete: true}})
	down := errors.New("connection refused")
	_, err := ArchiveSeatContactsTx(ctx, unreachableTx{err: down}, ids.Nil, 10)
	if !errors.Is(err, down) || !strings.Contains(err.Error(), "paging contacts for the seat repair") {
		t.Errorf("a failed page read = %v, want the database's error, named as the page read", err)
	}
}

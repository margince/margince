// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unreachableTx is a database that answers no query.
type unreachableTx struct {
	pgx.Tx
	err error
}

func (tx unreachableTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, tx.err
}

// A seat whose teams cannot be read is not answered as a seat in no team,
// which would hide every record shared with those teams.
func TestASeatWhoseTeamsCannotBeReadIsNoSeat(t *testing.T) {
	down := errors.New("connection reset")
	if _, err := SeatPrincipal(context.Background(), unreachableTx{err: down}, ids.NewV7()); !errors.Is(err, down) {
		t.Fatalf("SeatPrincipal = %v, want the failed team read", err)
	}
}

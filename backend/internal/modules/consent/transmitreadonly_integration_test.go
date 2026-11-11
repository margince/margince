// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestTheFinalSendReaderCannotTakeDomainRowLocksOrWrite(t *testing.T) {
	e := setupResolve(t)
	for _, statement := range []string{
		"SELECT 1 FROM contact FOR UPDATE",
		"UPDATE contact SET full_name=full_name WHERE false",
	} {
		err := e.gate.readTransmitTx(e.ctx, func(tx pgx.Tx) error { _, err := tx.Exec(e.ctx, statement); return err })
		denied, ok := errors.AsType[*pgconn.PgError](err)
		if !ok || denied.Code != "25006" {
			t.Fatalf("final reader admitted a locking/write statement: %v", err)
		}
	}
}

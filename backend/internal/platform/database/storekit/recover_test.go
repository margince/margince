// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRecoveryRefusesANonAdvancingCursorBeforeResuming(t *testing.T) {
	err := RecoverPages(context.Background(), func(_ context.Context, visit func(pgx.Tx) error) error { return visit(nil) },
		func(pgx.Tx, ids.UUID) ([]ids.UUID, error) { return []ids.UUID{ids.Nil}, nil },
		func(id ids.UUID) ids.UUID { return id },
		func(pgx.Tx, ids.UUID) error { t.Fatal("invalid cursor reached recovery"); return nil })
	if err == nil {
		t.Fatal("non-advancing page accepted")
	}
}

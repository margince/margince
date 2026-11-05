// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A purge that cannot run reports it, so an erasure never claims what it did not delete.
func TestPurgingProvidedValuesReportsAFailedStatement(t *testing.T) {
	e := setupSARIdentifiers(t)
	tx, err := e.owner.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(e.ctx); err != nil {
		t.Fatal(err)
	}
	if err := purgeProvidedValues(e.ctx, tx, []ids.UUID{ids.NewV7()}); err == nil {
		t.Error("purgeProvidedValues on a closed transaction succeeded")
	}
}

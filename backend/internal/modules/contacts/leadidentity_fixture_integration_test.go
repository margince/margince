// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedHistoricalLeadWithEmail(ctx context.Context, t *testing.T, store *Store, in CreateLeadInput, email string) ids.LeadID {
	t.Helper()
	in.Email = nil
	lead, _, err := store.CreateLead(ctx, in)
	if err != nil {
		t.Fatalf("create historical lead: %v", err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE lead SET email = lower($2) WHERE id = $1`, id, email)
		return err
	}); err != nil {
		t.Fatalf("restore historical lead address: %v", err)
	}
	return id
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The field-diff rail names the tool a delegated change came through, and
// names it from the row's own passport.
//
// The chip resolved a passport id against the reader's current passports, and
// that list carries only the newest token per connection.
//
// Every refresh revokes the connection's passport and mints another. So a
// change made under an earlier token matched nothing and read as "An agent".

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestFieldHistoryNamesTheClientOfARotatedPassport(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Rotation Subject", nil)
	human := seedWorkspaceUser(t, e, "Ada Authority")
	passport := seedGrantedPassport(t, e, human, "Claude")
	revokePassport(t, e, passport)

	// Dated forward: SeedContact's own create row is stamped at real now and
	// the read is newest first.
	at := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	seedDelegatedDiffRow(t, e, contact, human, passport,
		map[string]any{"title": "Head of Ops"},
		map[string]any{"title": "COO"}, at)

	page, err := privacy.ListFieldHistory(e.Admin(), e.DB(), privacy.FieldHistoryFilter{
		EntityType: "contact", EntityID: contact,
	})
	if err != nil {
		t.Fatalf("reading the field history: %v", err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("the field history is empty; the seeded diff should be in it")
	}
	newest := page.Entries[0]
	if newest.AgentClient == nil {
		t.Fatal("agent_client is nil for a revoked passport, so the chip still reads \"An agent\"")
	}
	if *newest.AgentClient != "Claude" {
		t.Errorf("agent_client = %q, want Claude", *newest.AgentClient)
	}
}

// revokePassport is the state a token refresh leaves behind.
//
// The connection's previous passport is revoked, so it leaves the reader's
// list while the rows it authorized stay.
func revokePassport(t *testing.T, e *Env, passport ids.UUID) {
	t.Helper()
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE passport SET revoked_at = now() WHERE id = $1`, passport)
		return err
	})
	if err != nil {
		t.Fatalf("revoking the passport: %v", err)
	}
}

// seedDelegatedDiffRow writes one field-diff row an agent authored, which the
// shared helper cannot: it stamps a human actor and carries no passport.
func seedDelegatedDiffRow(t *testing.T, e *Env, contact, human, passport ids.UUID,
	before, after map[string]any, at time.Time,
) {
	t.Helper()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("encoding the before image: %v", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("encoding the after image: %v", err)
	}
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO audit_log (id, actor_type, actor_id, on_behalf_of, passport_id,
			                        action, entity_type, entity_id, before, after, occurred_at)
			 VALUES ($1, 'agent', $2, $3, $4, 'update', 'contact', $5, $6, $7, $8)`,
			ids.NewV7(), "agent:"+passport.String(), human, passport, contact,
			beforeJSON, afterJSON, at)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the delegated diff row: %v", err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A dead-lettered delivery whose body was cleared has nothing to send, so a
// replay is refused before anything about the row is reset.
func TestAReplayOfAClearedBodyIsRefusedBeforeTheRowIsReset(t *testing.T) {
	e := setupContractVis(t)
	sub, deliveries := e.seedDeliveries(t, 1)
	cleared := deliveries[0]
	if _, err := e.owner.Exec(context.Background(), `UPDATE webhook_delivery
		SET status = 'dead_lettered', dead_lettered_at = now(), last_error = 'endpoint down'
		WHERE id = $1`, cleared); err != nil {
		t.Fatal(err)
	}
	ctx := e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true, Update: true}})
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error { return ClearDeliveryBodiesTx(ctx, tx, deliveries) }); err != nil {
		t.Fatal(err)
	}

	err := e.store.requireReplay(ctx, sub, cleared)
	var refused *BadInputError
	if !errors.As(err, &refused) {
		t.Fatalf("replaying a cleared body = %v, want a 422 refusal", err)
	}
	var status, lastError string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status, last_error FROM webhook_delivery WHERE id = $1`, cleared).Scan(&status, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "dead_lettered" || lastError != "endpoint down" {
		t.Fatalf("the refused replay changed the row to %s / %q", status, lastError)
	}
}

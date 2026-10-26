// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func mailSeatIdentities(t *testing.T, e *integration.Env) map[string][]string {
	t.Helper()
	var out map[string][]string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = storekit.UnambiguousMailSeatIdentities(e.Admin(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// A seat whose login names its domain in Unicode is matched by mail that
// spells it in punycode, and the reverse. The identity carries both
// spellings. The SQL side compares stored header addresses, and Postgres
// cannot fold IDNA.
func TestAUnicodeLoginOwnsItsPunycodeSpellingToo(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE app_user SET email = 'anna@bücher.example' WHERE id = $1`, e.Rep1)

	got := mailSeatIdentities(t, e)[e.Rep1.String()]
	for _, want := range []string{"anna@xn--bcher-kva.example", "anna@bücher.example"} {
		if !slices.Contains(got, want) {
			t.Errorf("the seat's identities %v lack %q", got, want)
		}
	}
}

// Two seats claiming one mailbox in two spellings collide, so neither is the
// unambiguous holder. Folded only by case, they used to look like two
// different addresses, each owned by one seat.
func TestTwoSpellingsOfOneMailboxOnTwoSeatsBelongToNeither(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE app_user SET email = 'anna@bücher.example' WHERE id = $1`, e.Rep1)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
		VALUES ($1, 'gmail', 'connected', 'vault:test', 'anna@xn--bcher-kva.example')`, e.Rep2); err != nil {
		t.Fatal(err)
	}
	identities := mailSeatIdentities(t, e)
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
		for _, address := range identities[seat.String()] {
			if address == "anna@xn--bcher-kva.example" || address == "anna@bücher.example" {
				t.Errorf("seat %s holds %q, which another seat claims in another spelling", seat, address)
			}
		}
	}
}

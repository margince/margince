// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
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

// The restoring read carries every spelling too, so stored mail a seat received
// in either spelling is still the seat's.
func TestTheRestoringReadCarriesBothSpellingsOfAUnicodeLogin(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE app_user SET email = 'anna@bücher.example' WHERE id = $1`, e.Rep1)
	var got map[string][]string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		got, err = storekit.MailSeatIdentities(e.Admin(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"anna@xn--bcher-kva.example", "anna@bücher.example"} {
		if !slices.Contains(got[e.Rep1.String()], want) {
			t.Errorf("the seat's restoring identities %v lack %q", got[e.Rep1.String()], want)
		}
	}
}

// provedIdentities is the seat-to-addresses map ProvedMailSeatIdentitiesSQL binds.
func provedIdentities(t *testing.T, e *integration.Env) map[string][]string {
	t.Helper()
	var bound []any
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.ProvedMailSeatIdentitiesSQL(e.Admin(), tx, func(v any) int {
			bound = append(bound, v)
			return len(bound)
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(bound) != 1 {
		t.Fatalf("bound %d values, want the one identity map", len(bound))
	}
	var encoded []byte
	switch value := bound[0].(type) {
	case string:
		encoded = []byte(value)
	case []byte:
		encoded = value
	default:
		t.Fatalf("bound a %T, want the encoded identity map", bound[0])
	}
	out := map[string][]string{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// A proved mailbox is the seat's in both spellings. Proved by two seats in two
// spellings, it is one mailbox two seats claim, and proves neither.
func TestAProvedMailboxIsOneMailboxInEitherSpelling(t *testing.T) {
	e := integration.Setup(t)
	connectMailbox(t, e, e.Rep1, "gmail", "anna@bücher.example")
	for _, want := range []string{"anna@xn--bcher-kva.example", "anna@bücher.example"} {
		if got := provedIdentities(t, e)[e.Rep1.String()]; !slices.Contains(got, want) {
			t.Errorf("the seat's proved identities %v lack %q", got, want)
		}
	}

	connectMailbox(t, e, e.Rep2, "gmail", "anna@xn--bcher-kva.example")
	identities := provedIdentities(t, e)
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
		if got := identities[seat.String()]; len(got) != 0 {
			t.Errorf("seat %s proves %v, a mailbox another seat proves in another spelling", seat, got)
		}
	}
}

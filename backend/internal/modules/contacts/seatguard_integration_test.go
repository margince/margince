// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func (e *dedupeEnv) declareSeatAddress(ctx context.Context, t *testing.T, address string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_owner_identity (user_id, kind, value, created_by)
			VALUES ($1, 'address', $2, 'human:test')`, e.otherRep, address)
		return err
	}); err != nil {
		t.Fatalf("declaring a seat address: %v", err)
	}
}

func (e *dedupeEnv) countContactsWithEmail(ctx context.Context, t *testing.T, address string) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM contact_email WHERE lower(email) = $1`, address).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEnsureCounterpartyRefusesAnotherSeatsLoginAndDeclaredAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.declareSeatAddress(ctx, t, "colleague.private@gmail.test")
	login := "OTHER-" + e.otherRep.String() + "@dd.test"
	for _, address := range []string{login, "Colleague.Private@gmail.test"} {
		_, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, address, "Colleague", "dd.test"))
		if !errors.Is(err, ErrCounterpartyIsSeat) {
			t.Fatalf("ensure %s = %v, want ErrCounterpartyIsSeat", address, err)
		}
	}
	if n := e.countContactsWithEmail(ctx, t, "colleague.private@gmail.test"); n != 0 {
		t.Fatalf("%d contacts minted for a seat address, want 0", n)
	}
	if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "carol@stranger.test", "Carol", "stranger.test")); err != nil {
		t.Fatalf("a real counterparty must still be created: %v", err)
	}
}

func TestDomainTriageDoesNotEmploySeatContacts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, address := range []string{"hans@seatdom.test", "colleague.private@seatdom.test"} {
		if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, address, "Person", "seatdom.test")); err != nil {
			t.Fatalf("ensure %s: %v", address, err)
		}
	}
	e.declareSeatAddress(ctx, t, "colleague.private@seatdom.test")

	readID := e.startTriageRead(ctx, t, "seatdom.test")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "seatdom.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Seatdom GmbH", SeedURL: "https://seatdom.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.EdgesPlanted != 1 {
		t.Fatalf("EdgesPlanted = %d, want 1 (the stranger only)", res.EdgesPlanted)
	}
	var seatEdges int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM relationship r JOIN contact_email ce ON ce.contact_id = r.contact_id
			 WHERE r.kind = 'employment' AND ce.email = 'colleague.private@seatdom.test'`).Scan(&seatEdges)
	}); err != nil {
		t.Fatal(err)
	}
	if seatEdges != 0 {
		t.Fatalf("seat contact has %d employment edges, want 0", seatEdges)
	}
}

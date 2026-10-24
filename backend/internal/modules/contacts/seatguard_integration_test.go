// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// declareSeatAddress stores an alias of seat. Source "user" is a typed alias.
// No product path writes "provider" yet, so this insert is its only route.
func (e *dedupeEnv) declareSeatAddress(ctx context.Context, t *testing.T, seat ids.UUID, address, source string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
			VALUES ($1, 'address', $2, $3, 'human:test')`, seat, address, source)
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

func TestEnsureCounterpartyRefusesAProvenSeatAddressFromEveryMailbox(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.declareSeatAddress(ctx, t, e.otherRep, "colleague.attested@gmail.test", "provider")
	login := "OTHER-" + e.otherRep.String() + "@dd.test"
	for _, address := range []string{login, "Colleague.Attested@gmail.test"} {
		_, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, address, "Colleague", "dd.test"))
		if !errors.Is(err, ErrCounterpartyIsSeat) {
			t.Fatalf("ensure %s from another seat's mailbox = %v, want ErrCounterpartyIsSeat", address, err)
		}
	}
	if n := e.countContactsWithEmail(ctx, t, "colleague.attested@gmail.test"); n != 0 {
		t.Fatalf("%d contacts minted for a proven seat address, want 0", n)
	}
	if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "carol@stranger.test", "Carol", "stranger.test")); err != nil {
		t.Fatalf("a real counterparty must still be created: %v", err)
	}
}

// Any seat can type any address as its own, a client's included. The typed
// alias stops capture from that seat's mailbox and from no one else's.
func TestATypedAliasStopsOnlyItsOwnSeatsCapture(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.declareSeatAddress(ctx, t, e.otherRep, "client@bigcustomer.test", "user")
	e.declareSeatAddress(ctx, t, e.rep, "rep.home@gmail.test", "user")

	if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "rep.home@gmail.test", "Rep", "gmail.test")); !errors.Is(err, ErrCounterpartyIsSeat) {
		t.Fatalf("the seat's own typed alias from its own mailbox = %v, want ErrCounterpartyIsSeat", err)
	}
	if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "client@bigcustomer.test", "Client", "bigcustomer.test")); err != nil {
		t.Fatalf("another seat's typed alias must not stop this seat's capture: %v", err)
	}
	if n := e.countContactsWithEmail(ctx, t, "client@bigcustomer.test"); n != 1 {
		t.Fatalf("%d contacts for the address another seat typed, want 1", n)
	}
}

func TestDomainTriageDoesNotEmploySeatContacts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, address := range []string{"hans@seatdom.test", "colleague.private@seatdom.test", "typed@seatdom.test"} {
		if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, address, "Someone", "seatdom.test")); err != nil {
			t.Fatalf("ensure %s: %v", address, err)
		}
	}
	e.declareSeatAddress(ctx, t, e.otherRep, "colleague.private@seatdom.test", "provider")
	e.declareSeatAddress(ctx, t, e.otherRep, "typed@seatdom.test", "user")

	readID := e.startTriageRead(ctx, t, "seatdom.test")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "seatdom.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Seatdom GmbH", SeedURL: "https://seatdom.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.EdgesPlanted != 2 {
		t.Fatalf("EdgesPlanted = %d, want 2 (the stranger and the typed alias)", res.EdgesPlanted)
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

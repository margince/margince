// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func runContactVerdict(t *testing.T, e *integration.Env, email, domain string) ids.UUID {
	t.Helper()
	activityID := seedCapturedMail(t, e, email, "hello")
	dispositionID := seedPendingDisposition(t, e, email, domain, activityID)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	return dispositionID
}

func contactsAt(t *testing.T, e *integration.Env, email string) int {
	t.Helper()
	return countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		 WHERE pe.email = $1 AND p.archived_at IS NULL`, email)
}

// A seat's address is a fact about the seats today. The ledger keeps `real`, so
// no noise sweep or erasure label reaches the colleague's mail. The address
// becomes a contact once the seat lets go of it.
func TestASeatAddressVerdictStaysRealAndCapturesOnceTheSeatLetsGo(t *testing.T) {
	e := integration.Setup(t)
	const address = "rep1.home@private.example"
	owned := capture.NewOwnerIdentityStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	alias, err := owned.Add(rep1, capture.IdentityKindAddress, address)
	if err != nil {
		t.Fatalf("adding the alias: %v", err)
	}

	first := runContactVerdict(t, e, address, "private.example")
	if got := dispositionStatus(t, e, first); got != capture.PendingStatusReal {
		t.Fatalf("disposition = %q, want real", got)
	}
	if n := contactsAt(t, e, address); n != 0 {
		t.Fatalf("%d contacts for the seat's own alias, want 0", n)
	}

	if err := owned.Remove(rep1, alias.ID); err != nil {
		t.Fatalf("removing the alias: %v", err)
	}
	runContactVerdict(t, e, address, "private.example")
	if n := contactsAt(t, e, address); n != 1 {
		t.Fatalf("%d contacts once the seat let go of the address, want 1", n)
	}
}

// Another seat's typed alias does not refuse this seat's verdict: typing an
// address proves nothing about who holds it.
func TestAnotherSeatsTypedAliasDoesNotStopAVerdict(t *testing.T) {
	e := integration.Setup(t)
	const address = "buyer@bigcustomer.example"
	rep2 := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(rep2, capture.IdentityKindAddress, address); err != nil {
		t.Fatalf("adding the alias: %v", err)
	}

	runContactVerdict(t, e, address, "bigcustomer.example")
	if n := contactsAt(t, e, address); n != 1 {
		t.Fatalf("%d contacts for an address only another seat typed, want 1", n)
	}
}

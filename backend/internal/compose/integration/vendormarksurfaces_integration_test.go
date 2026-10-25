// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A contact marked vendor_marketing drives no account picture. Each surface
// below shows the contact while unmarked and drops it once marked, so the
// assertion cannot pass because the setup was empty.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"

	"github.com/jackc/pgx/v5"
)

func markAsVendor(t *testing.T, contact ids.UUID) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(context.Background(), `UPDATE contact SET relationship_mark = 'vendor_marketing',
		relationship_mark_set_by = 'agent:capture_verdict' WHERE id = $1`, contact); err != nil {
		t.Fatalf("marking the contact: %v", err)
	}
}

func TestAMarkedContactsMailDoesNotReachTheirEmployersAccount(t *testing.T) {
	e := Setup(t)
	vendorCo := seedEmployerOnDomain(t, e, "Vendorco", "vendorco.test")
	mail := seedMailFrom(t, "val@vendorco.test", mailAtTheNewJob)
	val := captureSender(t, e, mail, "val@vendorco.test", "vendorco.test")
	if !reaches(t, e, mail, vendorCo) {
		t.Fatal("setup: the unmarked contact's mail does not reach the employer")
	}

	markAsVendor(t, val)

	if reaches(t, e, mail, vendorCo) {
		t.Error("a marked contact's mail still reaches their employer's account")
	}
}

func TestAMarkedCoworkerIsNotDrawnOnTheAccountArm(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	company := e.SeedCompany(t, "ScaleCommerce", &e.Rep1)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	vendor := e.SeedContact(t, "Marked Coworker", &e.Rep1)
	for _, p := range []ids.UUID{mine, vendor} {
		SeedIDRow(t, owner, `INSERT INTO relationship
			(id, kind, contact_id, company_id, source, captured_by)
			VALUES ($1, 'employment', '`+p.String()+`', '`+company.String()+`', 'manual', 'human:x')`)
	}
	drawn := func() bool {
		rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
		code, graph := readGraph(rep, t, e, mine)
		if code != http.StatusOK {
			t.Fatalf("graph → %d, want 200", code)
		}
		for _, n := range graph.Nodes {
			if n.Label == "Marked Coworker" {
				return true
			}
		}
		return false
	}
	if !drawn() {
		t.Fatal("setup: the unmarked coworker is not drawn")
	}

	markAsVendor(t, vendor)

	if drawn() {
		t.Error("the account arm still draws a coworker marked vendor_marketing")
	}
}

func TestAMarkedPeerIsNotObservedWithTheAnchor(t *testing.T) {
	e := Setup(t)
	anchor := e.SeedContact(t, "Anna Weber", &e.Rep1)
	peer := e.SeedContact(t, "Marked Peer", &e.Rep1)
	seedSharedThread(t, e, e.Rep1, anchor, peer, "the shared thread", "workspace")
	peers := func() int {
		rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
		var got []search.PeerEdge
		if err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
			var err error
			got, err = search.ContactEdgesForContact(rep, tx, anchor, 10)
			return err
		}); err != nil {
			t.Fatalf("reading the anchor's peers: %v", err)
		}
		return len(got)
	}
	if peers() != 1 {
		t.Fatal("setup: the unmarked peer is not observed")
	}

	markAsVendor(t, peer)

	if peers() != 0 {
		t.Error("a peer marked vendor_marketing is still observed with the anchor")
	}
}

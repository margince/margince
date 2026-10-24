// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedContactAt(t *testing.T, e *Env, owner *pgx.Conn, name, address string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, &e.Rep1)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'human:x')`, contact, address); err != nil {
		t.Fatalf("seeding %s: %v", address, err)
	}
	return contact
}

func coworkerLabels(t *testing.T, e *Env, viewer, contact ids.UUID) map[string]bool {
	t.Helper()
	code, graph := readGraph(e.As(viewer, []ids.UUID{e.Team1}, graphPerms), t, e, contact)
	if code != http.StatusOK {
		t.Fatalf("graph → %d, want 200", code)
	}
	labels := map[string]bool{}
	for _, n := range graph.Nodes {
		labels[n.Label] = true
	}
	return labels
}

// A seat is never drawn in "Their company". A proven seat address hides the
// contact from every viewer. An alias a seat only typed hides it from that
// seat alone, because any seat can type a client's address as its own.
func TestContactGraphKeepsSeatsOutOfTheirCompanyLane(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	var rep3Login string
	if err := owner.QueryRow(context.Background(),
		`SELECT email FROM app_user WHERE id = $1`, e.Rep3).Scan(&rep3Login); err != nil {
		t.Fatal(err)
	}
	company := e.SeedCompany(t, "ScaleCommerce", &e.Rep1)
	anchor := e.SeedContact(t, "Anna Weber", &e.Rep1)
	typed := seedContactAt(t, e, owner, "Typed Alias", "rep2.home@private.example")
	login := seedContactAt(t, e, owner, "Seat Login", rep3Login)
	for _, p := range []ids.UUID{anchor, typed, login} {
		SeedIDRow(t, owner, `INSERT INTO relationship
			(id, kind, contact_id, company_id, source, captured_by)
			VALUES ($1, 'employment', '`+p.String()+`', '`+company.String()+`', 'manual', 'human:x')`)
	}
	rep2 := e.As(e.Rep2, []ids.UUID{e.Team1}, AdminPerms)
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(rep2, capture.IdentityKindAddress, "rep2.home@private.example"); err != nil {
		t.Fatalf("adding the alias: %v", err)
	}

	own := coworkerLabels(t, e, e.Rep2, anchor)
	if own["Typed Alias"] || own["Seat Login"] {
		t.Fatalf("the seat who typed the alias sees %v, want neither seat drawn", own)
	}
	other := coworkerLabels(t, e, e.Rep1, anchor)
	if !other["Typed Alias"] {
		t.Fatal("another seat's typed alias hid the contact from this viewer")
	}
	if other["Seat Login"] {
		t.Fatal("a seat's login was drawn as a coworker at another company")
	}
}

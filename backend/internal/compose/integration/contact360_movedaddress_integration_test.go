// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bareInbound writes received mail linked to the contact whose sender row
// carries only the address, as capture writes it before anyone resolved it.
func bareInbound(t *testing.T, owner *pgx.Conn, contact ids.UUID, address string, at time.Time) {
	t.Helper()
	mail := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'hello', $2, 'inbound', 'manual', 'human:x')`, at)
	LinkActivity(t, owner, mail, "contact", contact)
	if _, err := owner.Exec(t.Context(),
		`INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'from')`, mail, address); err != nil {
		t.Fatal(err)
	}
}

// The inbound clock counts mail from a bare address only where the address
// proves the contact. Mail from an address another contact once held is not
// theirs to count.
func TestTheInboundClockSkipsMailFromABareMovedAddress(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	holder := e.SeedContact(t, "New Holder", &e.Rep1)
	former := e.SeedContact(t, "Former Holder", &e.Rep1)
	for _, row := range []struct {
		contact  ids.UUID
		address  string
		archived bool
	}{{former, "moved@customer.test", true}, {holder, "moved@customer.test", false}, {holder, "own@customer.test", false}} {
		if _, err := owner.Exec(t.Context(), `INSERT INTO contact_email (contact_id, email, source, captured_by, archived_at)
			VALUES ($1, $2, 'manual', 'human:seed', CASE WHEN $3 THEN now() END)`, row.contact, row.address, row.archived); err != nil {
			t.Fatal(err)
		}
	}
	// The page reads by its own clock, so the seeds are dated from it.
	own := roomFixedNow.Add(-30 * 24 * time.Hour)
	bareInbound(t, owner, holder, "own@customer.test", own)
	bareInbound(t, owner, holder, "moved@customer.test", roomFixedNow.Add(-10*24*time.Hour))

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	page, err := contactRoomService(e).Assemble(rep, ids.From[ids.ContactKind](holder))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	if page.LastInboundAt == nil || !page.LastInboundAt.Equal(own) {
		t.Errorf("last_inbound_at = %v, want the mail from their own address at %v", page.LastInboundAt, own)
	}
}

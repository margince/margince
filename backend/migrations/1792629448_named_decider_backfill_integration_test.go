// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// The backfill names a decider for every pending request it can, and closes the
// rest. A promise is put only to the colleague who made it. One staged for the
// reader, or private mail its owner did not promise, is closed instead.
func TestTheDeciderBackfillPutsAPromiseOnlyToItsPromiser(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1792641896"), 2); err != nil {
		t.Fatalf("rolling the decider migrations back: %v", err)
	}

	reader := seedAppUserForConnection(t, conn, "reader@backfill.example")
	promiser := seedAppUserForConnection(t, conn, "promiser@backfill.example")
	mailboxOwner := seedAppUserForConnection(t, conn, "mailbox@backfill.example")
	stage := func(kind, change string, onBehalfOf *string) string {
		t.Helper()
		var id string
		if err := conn.QueryRow(ctx, `
			INSERT INTO approval (kind, proposed_by, on_behalf_of, proposed_change, diff_hash, expires_at)
			VALUES ($1, 'agent:legacy', $2, $3::jsonb, gen_random_uuid()::text, now() + interval '1 day')
			RETURNING id`, kind, onBehalfOf, change).Scan(&id); err != nil {
			t.Fatalf("seeding a legacy %s: %v", kind, err)
		}
		return id
	}
	forReader := stage("commitment_task", `{"summary":"Send the figures"}`, &reader)
	byPromiser := stage("commitment_task", `{"summary":"Send the figures","promiser_seat_id":"`+promiser+`"}`, &reader)
	privateElsewhere := stage("commitment_task",
		`{"summary":"Send the figures","promiser_seat_id":"`+promiser+`","private_to":"`+mailboxOwner+`","seat_id":"`+mailboxOwner+`"}`, &mailboxOwner)
	legacy := stage("commitment_task", `{"summary":"Send the figures","seat_id":"`+promiser+`"}`, &promiser)
	legacyPrivate := stage("commitment_task", `{"summary":"Send the figures","seat_id":"`+mailboxOwner+`","private_to":"`+mailboxOwner+`"}`, &mailboxOwner)
	ordinary := stage("advance_deal", `{"stage":"lost"}`, &reader)

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}

	read := func(id string) (status string, decider, onBehalfOf *string) {
		t.Helper()
		if err := conn.QueryRow(ctx,
			`SELECT status, requested_decider_id::text, on_behalf_of::text FROM approval WHERE id = $1`,
			id).Scan(&status, &decider, &onBehalfOf); err != nil {
			t.Fatalf("reading %s: %v", id, err)
		}
		return status, decider, onBehalfOf
	}
	for name, id := range map[string]string{
		"staged for the reader":                  forReader,
		"private mail its owner did not promise": privateElsewhere,
		"in older private mail with no promiser": legacyPrivate,
	} {
		if status, decider, _ := read(id); status != "expired" || decider != nil {
			t.Errorf("a promise %s is %s for %v, want closed for nobody", name, status, orNobody(decider))
		}
	}
	if status, decider, onBehalfOf := read(byPromiser); status != "pending" || orNobody(decider) != promiser || orNobody(onBehalfOf) != promiser {
		t.Errorf("a promise with a promiser is %s for %s on behalf of %s, want pending for the promiser %s",
			status, orNobody(decider), orNobody(onBehalfOf), promiser)
	}
	if status, decider, _ := read(legacy); status != "pending" || orNobody(decider) != promiser {
		t.Errorf("an older proposal naming its promiser in seat_id is %s for %s, want pending for %s", status, orNobody(decider), promiser)
	}
	if status, decider, _ := read(ordinary); status != "pending" || orNobody(decider) != reader {
		t.Errorf("an ordinary request is %s for %s, want pending for the colleague it was staged for", status, orNobody(decider))
	}
	var audits int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'approval' AND action = 'expire'
		AND entity_id = ANY($1)`, []string{forReader, privateElsewhere, legacyPrivate}).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 3 {
		t.Errorf("%d audit rows for the three closed promises, want one each", audits)
	}
}

func orNobody(s *string) string {
	if s == nil {
		return "nobody"
	}
	return *s
}

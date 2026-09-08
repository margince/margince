// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The person page's last-touch fold against a real database: which message
// moves "they wrote last", and which one must not.
//
// Its own file rather than a section of the relationship room's, because it is
// its own question. That suite is about what a caller is REFUSED — the
// composite read's per-section denials, the correction ledger, the local
// graph's row scope. This one is about attribution, and the fixtures it needs
// are participant rows no refusal test has any use for.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// "They wrote last" is a claim about who AUTHORED the message, and the page
// folds it over three shapes of record that answer that question differently.
//
// A thread is linked to everybody it concerns, so a mail from Marcus to Judith
// reaches Judith's page: read off reachability, it told Judith's reader that
// Judith wrote last. Read off the sender alone, it went the other way — only
// capture writes participant rows, so a message a rep LOGGED BY HAND has none,
// and every hand-logged reply stopped counting as a reply at all.
//
// Both directions are asserted here because each is the other's regression: a
// fix for either one alone reintroduces the other, and neither shows up in a
// test that seeds only the shape it is about.
func TestLastInboundNamesTheAuthorAndNotEverybodyOnTheThread(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	mine := e.SeedPerson(t, "Judith Vogel", &e.Rep1)
	SeedIDRow(t, owner, `INSERT INTO person_email (id, person_id, email, source, captured_by)
		VALUES ($1, $2, 'judith@example.com', 'manual', 'human:x')`, mine)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	svc := personRoomService(e)
	personID := ids.From[ids.PersonKind](mine)

	// A rep logs their call by hand: nobody recorded a sender, and the link the
	// rep made is the whole of what the record says about who this was with.
	handLogged := roomAgo(72 * time.Hour)
	logged := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by)
		VALUES ($1, 'call', 'They rang back', 'body', $2, 'inbound', 'manual', 'human:x')`, handLogged)
	LinkActivity(t, owner, logged, "person", mine)

	page, err := svc.Assemble(rep, personID)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if page.LastInboundAt == nil || !page.LastInboundAt.Equal(handLogged) {
		t.Fatalf("last inbound = %v after a hand-logged inbound call, want %v — only capture writes participants, so a rep's own link is the only author this record has",
			page.LastInboundAt, handLogged)
	}

	// Marcus writes into the same conversation. It is NEWER than the call and it
	// reaches Judith's page, so a reachability read would move her clock to it.
	foreign := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'Re: the retrofit', 'body', $2, 'inbound', 'gmail', 'human:x')`, roomAgo(48*time.Hour))
	LinkActivity(t, owner, foreign, "person", mine)
	seedSender(t, owner, foreign, "marcus@example.com")

	page, err = svc.Assemble(rep, personID)
	if err != nil {
		t.Fatalf("Assemble after the colleague's message: %v", err)
	}
	if page.LastInboundAt == nil || !page.LastInboundAt.Equal(handLogged) {
		t.Fatalf("last inbound = %v after a message somebody ELSE sent into the thread, want the hand-logged call at %v — the page must not tell a reader that Judith wrote Marcus's mail",
			page.LastInboundAt, handLogged)
	}

	// Judith answers, captured through the mailbox: her address on the 'from'
	// row is the proof the strict predicate wants, and the clock moves.
	hers := roomAgo(time.Hour)
	reply := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'Re: the retrofit', 'body', $2, 'inbound', 'gmail', 'human:x')`, hers)
	LinkActivity(t, owner, reply, "person", mine)
	seedSender(t, owner, reply, "judith@example.com")

	page, err = svc.Assemble(rep, personID)
	if err != nil {
		t.Fatalf("Assemble after her reply: %v", err)
	}
	if page.LastInboundAt == nil || !page.LastInboundAt.Equal(hers) {
		t.Fatalf("last inbound = %v after the contact's own captured reply, want %v", page.LastInboundAt, hers)
	}
}

// seedSender stamps the 'from' participant capture writes for a message it
// ingested — the row that names an author, and the one a hand-logged activity
// never has.
func seedSender(t *testing.T, owner *pgx.Conn, activity ids.UUID, address string) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'from', $2)`,
		activity, address); err != nil {
		t.Fatalf("seeding the sender %s: %v", address, err)
	}
}

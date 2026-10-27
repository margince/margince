// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A lead who writes to us stays a lead. The mail files on the lead, no contact
// is made, and the lead's owner is asked to qualify them, once at a time.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ownedLead creates a live lead holding the address, owned by owner.
func (l *leadMailEnv) ownedLead(t *testing.T, address string, owner ids.UUID) ids.LeadID {
	t.Helper()
	ownerID := ids.From[ids.UserKind](owner)
	created, _, err := l.e.Contacts.CreateLead(l.e.Admin(), contacts.CreateLeadInput{
		FullName: new("Lena Prospect"), Email: new(address), Source: "manual", OwnerID: &ownerID,
	})
	if err != nil {
		t.Fatalf("creating the lead for %s: %v", address, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(created.Id))
}

// qualifyAsks counts the open asks to qualify the lead assigned to owner.
func (l *leadMailEnv) qualifyAsks(t *testing.T, lead ids.LeadID, owner ids.UUID) int {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id
		WHERE al.lead_id = $1 AND a.kind = 'task' AND a.source_system = $2
		  AND NOT a.is_done AND a.assignee_id = $3`, lead, provenance.LeadRepliedSource, owner)
}

func TestAReplyFromALeadAsksItsOwnerToQualifyThemOnce(t *testing.T) {
	l := setupLeadMail(t)
	const address = "lena@prospect.example"
	lead := l.ownedLead(t, address, l.e.Rep2)

	offer := "offer-" + ids.NewV7().String() + "@mail.example"
	outbound := l.mail(t, owedMail{from: l.seat, to: address, subject: "Our offer", messageID: offer}, true)
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 0 {
		t.Fatalf("mail we sent asked to qualify the lead %d times, want none: they have not written", n)
	}
	first := l.mail(t, owedMail{
		from: address, to: l.seat, subject: "Re: Our offer",
		inReplyTo: offer, at: l.at.Add(time.Minute),
	}, true)
	second := l.mail(t, owedMail{
		from: address, to: l.seat, subject: "Re: Our offer",
		inReplyTo: offer, at: l.at.Add(2 * time.Minute),
	}, true)

	for name, activity := range map[string]ids.UUID{"outbound": outbound, "first reply": first, "second reply": second} {
		if !l.filedUnderLead(t, activity, lead) {
			t.Errorf("the %s mail is not filed under the lead", name)
		}
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM contact_email WHERE email = $1 AND archived_at IS NULL`, address); n != 0 {
		t.Errorf("%d contact addresses were made for a live lead's address, want none", n)
	}
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
		t.Errorf("two replies left %d open asks to qualify the lead with its owner, want exactly one", n)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND archived_at IS NULL AND status <> 'promoted'`, lead); n != 1 {
		t.Error("a reply converted the lead; only somebody working the lead converts it")
	}

	// The owner works the ask. Mail we send next asks nothing; their next
	// message asks again.
	var asked ids.UUID
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT a.id FROM activity a
			JOIN activity_link al ON al.activity_id = a.id
			WHERE al.lead_id = $1 AND a.source_system = $2`, lead, provenance.LeadRepliedSource).Scan(&asked)
	}); err != nil {
		t.Fatalf("reading the ask: %v", err)
	}
	if _, err := l.e.Activities.CompleteTask(l.e.Admin(), ids.From[ids.ActivityKind](asked), l.at); err != nil {
		t.Fatalf("completing the ask: %v", err)
	}
	l.mail(t, owedMail{from: l.seat, to: address, subject: "Re: Our offer", inReplyTo: offer, at: l.at.Add(3 * time.Minute)}, true)
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 0 {
		t.Errorf("our own mail to a lead who wrote before opened %d asks, want none", n)
	}
	l.mail(t, owedMail{from: address, to: l.seat, subject: "Re: Our offer", inReplyTo: offer, at: l.at.Add(4 * time.Minute)}, true)
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
		t.Errorf("their next message after the ask was worked opened %d asks, want one", n)
	}
}

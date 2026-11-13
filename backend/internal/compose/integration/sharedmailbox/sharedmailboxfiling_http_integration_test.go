// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func strangerMail(msgID string) []byte {
	return []byte(strings.Join([]string{
		"From: Sam Stranger <sam@stranger.example>", "To: " + salesMailbox, "Subject: a question",
		"Date: Wed, 04 Jun 2026 09:00:00 +0000", "Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", "Do you ship abroad?", "",
	}, "\r\n"))
}

func TestSharedMailThatArrivesFirstFilesUnderTheExistingContactAndMintsNoneForAStranger(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := ids.UUID(s.connect(t, e.Team1).Id)
	untouched := map[string]string{
		"contact":          `SELECT count(*) FROM contact`,
		"company":          `SELECT count(*) FROM company`,
		"lead":             `SELECT count(*) FROM lead`,
		"task":             `SELECT count(*) FROM activity WHERE kind = 'task'`,
		"company question": `SELECT count(*) FROM company_domain_disposition`,
		"pending verdict":  `SELECT count(*) FROM capture_pending_counterparty`,
	}
	before := map[string]int{}
	for what, query := range untouched {
		before[what] = e.WsCount(t, query)
	}
	s.shared.raws = [][]byte{orderMail("shared-first@acme.example", salesMailbox), strangerMail("stranger@stranger.example")}
	s.sync(t, box)

	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'email'`); got != 2 {
		t.Fatalf("the shared mailbox landed %d emails, want 2", got)
	}
	var mail ids.UUID
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE l.contact_id = $1 AND a.subject = 'order 42 for the warehouse'`, s.anna); got != 1 {
		t.Fatalf("shared-first mail is filed under Anna %d times, want once", got)
	}
	mail = ids.MustParse(e.WsScalar(t, `SELECT id::text FROM activity WHERE subject = 'order 42 for the warehouse'`))
	if timeline, _ := s.sees(t, e.Rep2, []ids.UUID{e.Team1}, mail); !timeline {
		t.Fatal("a reader-team member must find shared-first mail on the existing contact's timeline")
	}
	for what, query := range untouched {
		if got := e.WsCount(t, query); got != before[what] {
			t.Errorf("shared-only mail changed the %s rows from %d to %d, want none created", what, before[what], got)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE email = 'sam@stranger.example'`); got != 0 {
		t.Fatal("a stranger writing to a shared mailbox must not become a contact")
	}
}

// privateContact is a contact only its owner reads, with address.
func (s *sharedMailboxSuite) privateContact(t *testing.T, name, address string, owner ids.UUID) ids.UUID {
	t.Helper()
	created, err := s.e.Contacts.CreateContact(s.e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(created.Id)
	s.e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, id, owner)
	return id
}

func mailFrom(msgID, from string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + salesMailbox, "Subject: hello from " + from,
		"Date: Wed, 04 Jun 2026 10:00:00 +0000", "Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", "A note.", "",
	}, "\r\n"))
}

func TestSharedMailFilesOnlyUnderAContactSomeReaderMayRead(t *testing.T) {
	s := newSharedMailboxSuite(t)
	e := s.e
	box := ids.UUID(s.connect(t, e.Team1).Id)
	outsidersOwn := s.privateContact(t, "Pat Private", "pat@private.example", e.Rep3)
	membersOwn := s.privateContact(t, "Rhea Reader", "rhea@private.example", e.Rep2)
	s.shared.raws = [][]byte{mailFrom("pat-1@private.example", "pat@private.example"), mailFrom("rhea-1@private.example", "rhea@private.example")}
	s.sync(t, box)

	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE contact_id = $1`, outsidersOwn); got != 0 {
		t.Fatal("shared mail was filed under a contact private to a seat outside the reader team")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE contact_id = $1`, membersOwn); got != 1 {
		t.Fatalf("shared mail is filed %d times under a contact a reader-team member reads, want once", got)
	}
}

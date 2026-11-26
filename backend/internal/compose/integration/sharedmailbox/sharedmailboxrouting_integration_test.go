// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// How shared-mailbox work follows a change of owner, refuses to guess between
// two owners, and stays away from the rules that ask about personal mail.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const supportMailbox = "support@authz.test"

func (s *sharedMailboxSuite) liveReminders(t *testing.T, mail ids.UUID) int {
	t.Helper()
	return s.e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND source_system = 'email_request'
	  AND source_activity_id = $1 AND archived_at IS NULL`, mail)
}

func TestAReplyTaskMovesWhenTheContactChangesOwner(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.connectOpen(t)
	s.ownAnna(t, &s.e.Rep1)
	mail := s.syncOne(t, box, orderMail("moves@acme.example", salesMailbox), "order 42 for the warehouse")
	if got := s.replyTaskHolder(t, mail); got != s.e.Rep1 {
		t.Fatalf("the reply task went to %s, want %s", got, s.e.Rep1)
	}
	s.ownAnna(t, &s.e.Rep2)

	if got := s.passAgain(t, mail); got != s.e.Rep2 {
		t.Fatalf("after the owner changed the reply task is held by %s, want the new owner %s", got, s.e.Rep2)
	}
	if got := s.liveReminders(t, mail); got != 1 {
		t.Fatalf("%d live reminders for one request, want the new owner's alone", got)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
		JOIN activity task ON task.id = a.entity_id WHERE task.source_activity_id = $1 AND a.action = 'update'
		AND o.envelope->'payload'->'changed_fields'->>'assignee_id' = $2::text`, mail, s.e.Rep2); got != 1 {
		t.Fatalf("the move wrote %d audited assignee changes with their event, want 1", got)
	}
}

func TestAReplyTaskIsRetiredWhenASeatsOwnMailboxLaterCapturesTheMail(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.connectOpen(t)
	s.ownAnna(t, &s.e.Rep1)
	// Rep3 is copied. Once Rep3's own mailbox holds the mail, the personal
	// rule decides it, and a copied seat owes no answer.
	raw := []byte(strings.Replace(string(orderMail("later@acme.example", salesMailbox)),
		"To: "+salesMailbox, "To: "+salesMailbox+"\r\nCc: "+repMailbox, 1))
	mail := s.syncOne(t, box, raw, "order 42 for the warehouse")
	if got := s.replyTaskHolder(t, mail); got != s.e.Rep1 {
		t.Fatalf("the reply task went to %s, want %s", got, s.e.Rep1)
	}
	personal := s.connectRepMailbox(t)
	s.personal.raws = [][]byte{raw}
	s.sync(t, personal)

	if got := s.passAgain(t, mail); !got.IsZero() {
		t.Fatalf("once a seat's own mailbox captured the mail, %s still holds a shared-mailbox reminder", got)
	}
}

func outboundMail(msgID string, to ...string) []byte {
	return []byte(strings.Join([]string{
		"From: Sales <" + salesMailbox + ">", "To: " + strings.Join(to, ", "), "Subject: our offer",
		"Date: Wed, 04 Jun 2026 11:00:00 +0000", "Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", "We will send the offer by Friday.", "",
	}, "\r\n"))
}

func (s *sharedMailboxSuite) contactOwnedBy(t *testing.T, name, address string, owner ids.UUID) {
	t.Helper()
	created, err := s.e.Contacts.CreateContact(s.e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.e.WsExec(t, `UPDATE contact SET owner_id = $2 WHERE id = $1`, created.Id, owner)
}

func TestOutboundMailToTwoOwnersFallsToTheBackupOwner(t *testing.T) {
	for name, tc := range map[string]struct {
		bobOwner func(e *integration.Env) ids.UUID
		want     func(e *integration.Env) ids.UUID
	}{
		"two owners":     {func(e *integration.Env) ids.UUID { return e.Rep2 }, func(e *integration.Env) ids.UUID { return e.AdminUser }},
		"the same owner": {func(e *integration.Env) ids.UUID { return e.Rep1 }, func(e *integration.Env) ids.UUID { return e.Rep1 }},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSharedMailboxSuite(t)
			if _, err := integration.OwnerConn(t).Exec(context.Background(),
				`INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, s.e.Team1, s.e.AdminUser); err != nil {
				t.Fatal(err)
			}
			box := ids.UUID(s.connect(t, s.e.Team1).Id)
			if code := s.setBackup(t, box, &s.e.AdminUser); code != http.StatusOK {
				t.Fatalf("naming the backup owner answered %d", code)
			}
			s.ownAnna(t, &s.e.Rep1)
			s.contactOwnedBy(t, "Bob Buyer", "bob@acme.example", tc.bobOwner(s.e))
			mail := s.syncOne(t, box, outboundMail("two@acme.example", "anna@acme.example", "bob@acme.example"), "our offer")
			if got := s.workOwner(t, mail); got != tc.want(s.e) {
				t.Fatalf("outbound mail went to %s, want %s", got, tc.want(s.e))
			}
		})
	}
}

func TestTheMailboxsOwnAddressIsNeverTheOtherParty(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := ids.UUID(s.connect(t, s.e.Team1).Id)
	s.ownAnna(t, &s.e.Rep1)
	s.contactOwnedBy(t, "Sales Desk", salesMailbox, s.e.Rep2)
	mail := s.syncOne(t, box, outboundMail("self@acme.example", "anna@acme.example", salesMailbox), "our offer")
	if got := s.workOwner(t, mail); got != s.e.Rep1 {
		t.Fatalf("mail the mailbox copied to itself went to %s, want Anna's owner %s", got, s.e.Rep1)
	}
}

func TestTwoSharedMailboxesWithDifferentBackupOwnersNameNobody(t *testing.T) {
	for name, tc := range map[string]struct {
		second func(e *integration.Env) ids.UUID
		want   func(e *integration.Env) ids.UUID
	}{
		"different backups": {func(e *integration.Env) ids.UUID { return e.Rep2 }, func(*integration.Env) ids.UUID { return ids.Nil }},
		"the same backup":   {func(e *integration.Env) ids.UUID { return e.Rep1 }, func(e *integration.Env) ids.UUID { return e.Rep1 }},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSharedMailboxSuite(t)
			sales := ids.UUID(s.connect(t, s.e.Team1).Id)
			body := connectBody(s.e.Team1, "app-secret")
			body["address"], body["imap"].(map[string]any)["username"] = supportMailbox, supportMailbox
			rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", body)
			var support crmcontracts.SharedMailbox
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &support) != nil {
				t.Fatalf("connecting support@ answered %d: %s", rec.Code, rec.Body)
			}
			second := tc.second(s.e)
			if s.setBackup(t, sales, &s.e.Rep1) != http.StatusOK || s.setBackup(t, ids.UUID(support.Id), &second) != http.StatusOK {
				t.Fatal("naming the backup owners was refused")
			}
			s.ownAnna(t, nil)
			raw := orderMail("both@acme.example", salesMailbox, supportMailbox)
			mail := s.syncOne(t, sales, raw, "order 42 for the warehouse")
			s.shared.address = supportMailbox
			s.syncOne(t, ids.UUID(support.Id), raw, "order 42 for the warehouse")
			if got := s.e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id IS NULL`, mail); got != 2 {
				t.Fatalf("the mail sits in %d shared mailboxes, want 2", got)
			}
			if got := s.workOwner(t, mail); got != tc.want(s.e) {
				t.Fatalf("mail in two shared mailboxes went to %s, want %s", got, tc.want(s.e))
			}
		})
	}
}

func TestAVendorQuestionIsNeverAskedOfASharedMailboxsWorkOwner(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := ids.UUID(s.connect(t, s.e.Team1).Id)
	s.ownAnna(t, &s.e.Rep1)
	mail := s.syncOne(t, box, orderMail("vendor@acme.example", salesMailbox), "order 42 for the warehouse")
	if got := s.workOwner(t, mail); got != s.e.Rep1 {
		t.Fatalf("the shared-mail rule names %s, want %s, so the check below would prove nothing", got, s.e.Rep1)
	}
	s.e.Contacts.SetClock(func() time.Time { return time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC) })
	s.e.WsExec(t, `INSERT INTO company_domain_disposition (domain, status, pending_reason, evidence)
	  VALUES ('acme.example', 'pending', $1, 'the page sells managed hosting')
	  ON CONFLICT (domain) DO UPDATE SET status = 'pending', pending_reason = $1, company_id = NULL`, contacts.PendingVendorVerdict)

	questions, err := s.e.Contacts.OpenDomainQuestionsForOwner(s.e.As(s.e.Rep1, []ids.UUID{s.e.Team1}, integration.AdminPerms))
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 0 {
		t.Fatalf("a vendor question went to the shared mailbox's work owner: %+v", questions)
	}
}

func TestATeamChangeClearsABackupOwnerTheNewTeamDoesNotHold(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := ids.UUID(s.connect(t, s.e.Team1).Id)
	if code := s.setBackup(t, box, &s.e.Rep1); code != http.StatusOK {
		t.Fatalf("naming the backup owner answered %d", code)
	}
	rec := s.call(s.admin(), t, http.MethodPatch, "/shared-mailboxes/"+box.String(), map[string]any{"reader_team_id": s.e.Team2, "confirm": true})
	var moved crmcontracts.SharedMailbox
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &moved) != nil {
		t.Fatalf("moving the mailbox to Team2 answered %d: %s", rec.Code, rec.Body)
	}
	if moved.BackupOwnerId != nil {
		t.Fatalf("the move answered backup owner %s, who is not in the new team", *moved.BackupOwnerId)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND backup_owner_id IS NULL`, box); got != 1 {
		t.Fatal("the backup owner outside the new team must be cleared")
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
		WHERE a.entity_id = $1 AND o.envelope->'payload'->>'change' = 'backup_owner_changed'`, box); got != 2 {
		t.Fatalf("%d backup owner changes on the trail, want the naming and the clearing", got)
	}
}

func TestResendingABackupOwnerWhoLeftTheTeamIsRefused(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := ids.UUID(s.connect(t, s.e.Team1).Id)
	if code := s.setBackup(t, box, &s.e.Rep1); code != http.StatusOK {
		t.Fatalf("naming the backup owner answered %d", code)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`, s.e.Team1, s.e.Rep1); err != nil {
		t.Fatal(err)
	}
	if code := s.setBackup(t, box, &s.e.Rep1); code != http.StatusUnprocessableEntity {
		t.Fatalf("resending a backup owner who left the team answered %d, want 422", code)
	}
}

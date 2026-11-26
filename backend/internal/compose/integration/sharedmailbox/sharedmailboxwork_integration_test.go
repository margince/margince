// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// Who gets the work from mail only a shared mailbox captured. The sender's
// contact owner comes first, then its company's owner, then the backup owner.
// Each counts only while they may read the mail and hold work.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// workAt is a day after the suite's mail, inside every waiting horizon.
var workAt = time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)

// replyTaskHolder marks mail as a request, runs the hourly reply-task pass
// and answers who holds its live reminder, or zero when none is live.
func (s *sharedMailboxSuite) replyTaskHolder(t *testing.T, mail ids.UUID) ids.UUID {
	t.Helper()
	// Only mail open to the workspace is judged, so held mail never gets here.
	if applied, err := s.e.Activities.SetOwedVerdict(s.requestPass(), mail, activities.OwedVerdictAsksUs, "fixture", workAt); err != nil || !applied {
		t.Fatalf("judging the mail: applied=%v err=%v", applied, err)
	}
	return s.passAgain(t, mail)
}

// passAgain runs the reply-task pass once more and answers who holds the
// live reminder for mail.
func (s *sharedMailboxSuite) passAgain(t *testing.T, mail ids.UUID) ids.UUID {
	t.Helper()
	if err := s.e.Activities.CaptureEmailRequests(s.requestPass(), workAt); err != nil {
		t.Fatal(err)
	}
	holder := s.e.WsScalar(t, `SELECT coalesce(max(assignee_id::text), '') FROM activity
	  WHERE kind = 'task' AND source_system = 'email_request' AND source_activity_id = $1 AND archived_at IS NULL`, mail)
	if holder == "" {
		return ids.Nil
	}
	return ids.MustParse(holder)
}

func (s *sharedMailboxSuite) requestPass() context.Context {
	return principal.WithActor(s.e.Admin(), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:owed_verdict",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// workOwner is the seat the shared-mail rule names for mail, the one every
// producer reads, or zero.
func (s *sharedMailboxSuite) workOwner(t *testing.T, mail ids.UUID) ids.UUID {
	t.Helper()
	owner := s.e.WsScalar(t, `SELECT coalesce((`+storekit.SharedMailWorkOwnerSQL("a")+`)::text, '') FROM activity a WHERE a.id = $1`, mail)
	if owner == "" {
		return ids.Nil
	}
	return ids.MustParse(owner)
}

// syncOne delivers raw to the shared mailbox and answers the newest mail
// with subject.
func (s *sharedMailboxSuite) syncOne(t *testing.T, box ids.UUID, raw []byte, subject string) ids.UUID {
	t.Helper()
	s.shared.raws = [][]byte{raw}
	s.sync(t, box)
	return ids.MustParse(s.e.WsScalar(t, `SELECT max(id::text) FROM activity WHERE kind = 'email' AND subject = $1`, subject))
}

// setBackup names seat as box's backup owner, or clears it for nil.
func (s *sharedMailboxSuite) setBackup(t *testing.T, box ids.UUID, seat *ids.UUID) int {
	t.Helper()
	rec := s.call(s.admin(), t, http.MethodPatch, "/shared-mailboxes/"+box.String(), map[string]any{"backup_owner_id": seat, "confirm": false})
	return rec.Code
}

// connectOpen connects sales@ read by Team1 with its mail open to the
// workspace, the only mail the reply-task pass judges.
func (s *sharedMailboxSuite) connectOpen(t *testing.T) ids.UUID {
	t.Helper()
	on := true
	settings := capture.NewSettings(compose.NewSettingsStore(s.e.Pool), compose.StoredConfig(s.e.Pool))
	if _, err := settings.Update(s.admin(), capture.SettingsPatch{SharedPostureAllowed: &on}); err != nil {
		t.Fatal(err)
	}
	body := connectBody(s.e.Team1, "app-secret")
	body["mail_visibility"] = "shared"
	if rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", body); rec.Code != http.StatusOK {
		t.Fatalf("connecting an open shared mailbox answered %d: %s", rec.Code, rec.Body)
	}
	return ids.MustParse(s.e.WsScalar(t, `SELECT id::text FROM capture_connection WHERE user_id IS NULL`))
}

func (s *sharedMailboxSuite) ownAnna(t *testing.T, owner *ids.UUID) {
	t.Helper()
	s.e.WsExec(t, `UPDATE contact SET owner_id = $2 WHERE id = $1`, s.anna, owner)
}

func TestAKnownSendersOwnerGetsTheReplyTask(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.connectOpen(t)
	s.ownAnna(t, &s.e.Rep1)
	mail := s.syncOne(t, box, orderMail("owned@acme.example", salesMailbox), "order 42 for the warehouse")

	if got := s.replyTaskHolder(t, mail); got != s.e.Rep1 {
		t.Fatalf("the reply task went to %s, want the contact's owner %s", got, s.e.Rep1)
	}
}

func TestAContactWithNoOwnerSendsTheWorkToItsCompanysOwner(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.connectOpen(t)
	s.ownAnna(t, nil)
	company := ids.From[ids.CompanyKind](s.e.SeedCompany(t, "Acme", &s.e.Rep2))
	if _, err := s.e.Contacts.CreateRelationship(s.e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: new(ids.From[ids.ContactKind](s.anna)), CompanyID: &company, Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	mail := s.syncOne(t, box, orderMail("company@acme.example", salesMailbox), "order 42 for the warehouse")

	if got := s.replyTaskHolder(t, mail); got != s.e.Rep2 {
		t.Fatalf("the reply task went to %s, want the company's owner %s", got, s.e.Rep2)
	}
}

func TestAnOwnerWhoCannotReadHeldMailGetsNoWorkAndTheBackupOwnerDoes(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := ids.UUID(s.connect(t, s.e.Team1).Id)
	// Rep3 sits outside the reader team and is not on the mail.
	s.ownAnna(t, &s.e.Rep3)
	mail := s.syncOne(t, box, orderMail("outsider@acme.example", salesMailbox), "order 42 for the warehouse")

	if got := s.workOwner(t, mail); !got.IsZero() {
		t.Fatalf("with no backup owner, held mail went to %s, who cannot read it or is not its owner", got)
	}
	if code := s.setBackup(t, box, &s.e.Rep2); code != http.StatusOK {
		t.Fatalf("naming a reader-team member as backup owner answered %d", code)
	}
	if got := s.workOwner(t, mail); got != s.e.Rep2 {
		t.Fatalf("held mail went to %s, want the backup owner %s", got, s.e.Rep2)
	}
}

func TestAStrangerWithNoBackupOwnerMakesNoContactAndNoTask(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.connectOpen(t)
	mail := s.syncOne(t, box, strangerMail("nobody@stranger.example"), "a question")

	if got := s.replyTaskHolder(t, mail); !got.IsZero() {
		t.Fatalf("mail nobody owns filed a reply task for %s, want none", got)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM contact_email WHERE email = 'sam@stranger.example'`); got != 0 {
		t.Fatal("a stranger writing to a shared mailbox must not become a contact")
	}
}

func TestABackupOwnerWhoLeftTheReaderTeamGetsNoMoreWork(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.connectOpen(t)
	if code := s.setBackup(t, box, &s.e.Rep2); code != http.StatusOK {
		t.Fatalf("naming a reader-team member as backup owner answered %d", code)
	}
	// Anna's contact has no owner and no company, so only the backup is left.
	s.ownAnna(t, nil)
	before := s.syncOne(t, box, mailFrom("before@acme.example", "anna@acme.example"), "hello from anna@acme.example")
	if got := s.replyTaskHolder(t, before); got != s.e.Rep2 {
		t.Fatalf("an unowned sender's request went to %s, want the backup owner %s", got, s.e.Rep2)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`, s.e.Team1, s.e.Rep2); err != nil {
		t.Fatal(err)
	}
	after := s.syncOne(t, box, mailFrom("after@acme.example", "anna@acme.example"), "hello from anna@acme.example")
	if got := s.replyTaskHolder(t, after); !got.IsZero() {
		t.Fatalf("a backup owner who left the reader team got the reply task (%s)", got)
	}
}

func TestOnlyAReaderTeamMemberMayBeTheBackupOwner(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := ids.UUID(s.connect(t, s.e.Team1).Id)
	rec := s.call(s.admin(), t, http.MethodPatch, "/shared-mailboxes/"+box.String(), map[string]any{"backup_owner_id": s.e.Rep3, "confirm": false})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "backup_owner_not_in_reader_team") {
		t.Fatalf("a backup owner outside the reader team answered %d %s, want 422", rec.Code, rec.Body)
	}
	if code := s.setBackup(t, box, &s.e.Rep1); code != http.StatusOK {
		t.Fatalf("a reader-team member as backup owner answered %d", code)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
		WHERE a.entity_id = $1 AND o.envelope->>'type' = 'shared_mailbox.changed' AND o.envelope->'payload'->>'change' = 'backup_owner_changed'`, box); got != 1 {
		t.Fatalf("naming the backup owner wrote %d audit rows with their event, want 1", got)
	}
	if code := s.setBackup(t, box, nil); code != http.StatusOK {
		t.Fatalf("clearing the backup owner answered %d", code)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND backup_owner_id IS NULL`, box); got != 1 {
		t.Fatal("an explicit null must clear the backup owner")
	}
	body := connectBody(s.e.Team1, "app-secret")
	body["backup_owner_id"] = s.e.Rep3
	if rec := s.call(s.admin(), t, http.MethodPost, "/shared-mailboxes", body); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a connect naming a backup owner outside the reader team answered %d, want 422", rec.Code)
	}
}

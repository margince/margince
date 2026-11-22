// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// A shared mailbox with a backup owner judges an unknown sender by the same
// ladder a seat's own mailbox does. What it mints is the backup owner's.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const strangerAddress = "sam@stranger.example"

// newsletterMail is bulk mail from a sending prefix with a List-Unsubscribe
// header, the shape the transactional rule suppresses.
func newsletterMail(msgID string) []byte {
	return []byte(strings.Join([]string{
		"From: Letters <hello@news.letters.example>", "To: " + salesMailbox, "Subject: this week's offers",
		"List-Unsubscribe: <mailto:unsubscribe@news.letters.example>",
		"Date: Wed, 04 Jun 2026 09:30:00 +0000", "Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", "Our offers this week.", "",
	}, "\r\n"))
}

// everyoneIsAContact answers every sender a verdict pass asks about as a
// contact, so only the ladder and the ensure decide what is minted.
type everyoneIsAContact struct{}

func (everyoneIsAContact) Complete(_ context.Context, req model.Request) (model.Response, error) {
	marker, _ := promptfence.MarkerIn(req.System)
	opening := "<" + marker + ` id="`
	var results []map[string]any
	for _, part := range strings.Split(req.Messages[0].Content, opening)[1:] {
		ref, _, _ := strings.Cut(part, `"`)
		results = append(results, map[string]any{"id": ref, "verdict": "contact", "confidence": 0.95})
	}
	payload, err := json.Marshal(map[string]any{"results": results})
	return model.Response{Text: string(payload), ServedModel: "test-local-small"}, err
}

func (s *sharedMailboxSuite) judgeEveryone(t *testing.T) {
	t.Helper()
	engine := compose.NewCounterpartyVerdictEngine(s.e.Pool, everyoneIsAContact{}, compose.CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), s.e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
}

// backedMailbox connects sales@ for Team1 with Rep2 as its backup owner.
func (s *sharedMailboxSuite) backedMailbox(t *testing.T) ids.UUID {
	t.Helper()
	box := ids.UUID(s.connect(t, s.e.Team1).Id)
	if code := s.setBackup(t, box, &s.e.Rep2); code != http.StatusOK {
		t.Fatalf("naming the backup owner answered %d", code)
	}
	return box
}

func (s *sharedMailboxSuite) contactsOf(t *testing.T, address, visibility string) int {
	t.Helper()
	return s.e.WsCount(t, `SELECT count(*) FROM contact c JOIN contact_email ce ON ce.contact_id = c.id
	  WHERE ce.email = $1 AND ce.archived_at IS NULL AND c.archived_at IS NULL AND c.owner_id = $2 AND c.visibility = $3`,
		address, s.e.Rep2, visibility)
}

func (s *sharedMailboxSuite) anyContactFor(t *testing.T, address string) int {
	t.Helper()
	return s.e.WsCount(t, `SELECT count(*) FROM contact_email WHERE email = $1 AND archived_at IS NULL`, address)
}

func (s *sharedMailboxSuite) questionsFor(t *testing.T, address string) (backups, all int) {
	t.Helper()
	backups = s.e.WsCount(t, `SELECT count(*) FROM capture_pending_counterparty WHERE email = $1 AND owner_id = $2`,
		address, s.e.Rep2)
	all = s.e.WsCount(t, `SELECT count(*) FROM capture_pending_counterparty WHERE email = $1`, address)
	return backups, all
}

func TestAStrangerToASharedMailboxBecomesTheBackupOwnersContact(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.backedMailbox(t)
	s.syncOne(t, box, strangerMail("first@stranger.example"), "a question")
	if backups, all := s.questionsFor(t, strangerAddress); backups != 1 || all != 1 {
		t.Fatalf("a first mail opened %d questions for the backup owner of %d, want the one", backups, all)
	}
	if got := s.anyContactFor(t, strangerAddress); got != 0 {
		t.Fatal("an unjudged first mail must wait for its verdict, as on a seat's own mailbox")
	}
	s.judgeEveryone(t)

	if got := s.contactsOf(t, strangerAddress, "owner"); got != 1 {
		t.Fatalf("the judged sender has %d contacts owned by and private to the backup owner, want 1", got)
	}
}

func TestAVerdictMintsNothingForASeatNoLongerTheBackupOwner(t *testing.T) {
	for name, lose := range map[string]func(s *sharedMailboxSuite, t *testing.T, box ids.UUID){
		"another seat became the backup owner": func(s *sharedMailboxSuite, t *testing.T, box ids.UUID) {
			s.ownerExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, s.e.Team1, s.e.AdminUser)
			if code := s.setBackup(t, box, &s.e.AdminUser); code != http.StatusOK {
				t.Fatalf("naming another backup owner answered %d", code)
			}
		},
		"the backup owner left the reader team": func(s *sharedMailboxSuite, t *testing.T, _ ids.UUID) {
			s.ownerExec(t, `DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`, s.e.Team1, s.e.Rep2)
		},
		"the backup owner excluded the sender": func(s *sharedMailboxSuite, t *testing.T, _ ids.UUID) {
			s.excludeForBackup(t, strangerAddress)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSharedMailboxSuite(t)
			box := s.backedMailbox(t)
			s.syncOne(t, box, strangerMail("first@stranger.example"), "a question")
			lose(s, t, box)
			s.judgeEveryone(t)

			if got := s.anyContactFor(t, strangerAddress); got != 0 {
				t.Fatalf("a verdict minted %d contacts for a seat no longer the backup owner who may hold them", got)
			}
		})
	}
}

func TestASenderTheBackupOwnerExcludedNeverBecomesTheirContact(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.backedMailbox(t)
	s.excludeForBackup(t, strangerAddress)
	s.syncOne(t, box, strangerMail("first@stranger.example"), "a question")
	s.judgeEveryone(t)

	if _, all := s.questionsFor(t, strangerAddress); all != 0 {
		t.Fatalf("an excluded sender opened %d questions for the backup owner, want none", all)
	}
	if got := s.anyContactFor(t, strangerAddress); got != 0 {
		t.Fatal("a sender the backup owner excluded became their contact")
	}
}

// excludeForBackup is the backup owner keeping an address out of their capture.
func (s *sharedMailboxSuite) excludeForBackup(t *testing.T, address string) {
	t.Helper()
	asBackup := s.e.As(s.e.Rep2, []ids.UUID{s.e.Team1}, integration.RepPerms)
	if _, err := capture.NewExclusionStore(compose.InstallationDB(s.e.Pool)).Add(asBackup,
		capture.ExclusionScopeUser, capture.ExclusionKindAddress, address, ids.Nil); err != nil {
		t.Fatalf("excluding %s for the backup owner: %v", address, err)
	}
}

func (s *sharedMailboxSuite) ownerExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestASenderTheLadderKnowsIsMintedPrivateToTheBackupOwner(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.backedMailbox(t)
	s.syncOne(t, box, strangerMail("first@stranger.example"), "a question")
	s.judgeEveryone(t)
	// The judged contact is gone, so the next mail finds a sender the ladder
	// already knows and no record that holds the address.
	s.e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id IN (SELECT contact_id FROM contact_email WHERE email = $1)`, strangerAddress)
	s.e.WsExec(t, `UPDATE contact_email SET archived_at = now() WHERE email = $1`, strangerAddress)
	mail := s.syncOne(t, box, mailFrom("again@stranger.example", "Sam Stranger <"+strangerAddress+">"),
		"hello from Sam Stranger <"+strangerAddress+">")

	if got := s.contactsOf(t, strangerAddress, "owner"); got != 1 {
		t.Fatalf("the known sender has %d live contacts private to the backup owner, want 1", got)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM activity_link l JOIN contact_email ce ON ce.contact_id = l.contact_id
	  WHERE l.activity_id = $1 AND ce.email = $2 AND ce.archived_at IS NULL`, mail, strangerAddress); got != 1 {
		t.Fatalf("the mail is filed %d times under the new contact, want once", got)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN contact_email ce ON ce.contact_id = a.entity_id
	  WHERE a.entity_type = 'contact' AND a.action = 'create' AND ce.email = $1 AND ce.archived_at IS NULL
	    AND a.on_behalf_of = $2`, strangerAddress, s.e.Rep2); got != 1 {
		t.Fatalf("%d creations of the new contact act for the backup owner, want 1", got)
	}
}

func TestANewsletterOrAMachineSenderMintsNothingForTheBackupOwner(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.backedMailbox(t)
	machine := "noreply@acme-orders.example"
	s.shared.raws = [][]byte{newsletterMail("letter@news.letters.example"), mailFrom("robot@acme-orders.example", machine)}
	s.sync(t, box)
	if got := s.e.WsCount(t, `SELECT count(*) FROM capture_pending_counterparty
	  WHERE email = 'hello@news.letters.example' AND status = 'suppressed' AND owner_id = $1`, s.e.Rep2); got != 1 {
		t.Fatalf("the newsletter has %d suppressions for the backup owner, want the transactional rule's one", got)
	}
	// Judged mail keeps the audience its reader team reads it under.
	if got := s.e.WsCount(t, `SELECT count(*) FROM activity WHERE subject = 'this week''s offers'
	  AND audience_reason IS NOT DISTINCT FROM 'no_record'`); got != 0 {
		t.Fatal("the suppressed newsletter was held to its participants, away from the reader team")
	}
	s.judgeEveryone(t)

	for _, address := range []string{"hello@news.letters.example", machine} {
		if got := s.anyContactFor(t, address); got != 0 {
			t.Errorf("%s became a contact", address)
		}
	}
}

func TestASharedMailboxMintsNothingWithoutABackupOwnerWhoMayHoldIt(t *testing.T) {
	for name, lose := range map[string]func(s *sharedMailboxSuite, t *testing.T, box ids.UUID){
		"no backup owner": func(s *sharedMailboxSuite, t *testing.T, box ids.UUID) {
			if code := s.setBackup(t, box, nil); code != http.StatusOK {
				t.Fatalf("clearing the backup owner answered %d", code)
			}
		},
		"the backup owner left the reader team": func(s *sharedMailboxSuite, t *testing.T, _ ids.UUID) {
			if _, err := integration.OwnerConn(t).Exec(context.Background(),
				`DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`, s.e.Team1, s.e.Rep2); err != nil {
				t.Fatal(err)
			}
		},
		"the backup owner holds a read seat": func(s *sharedMailboxSuite, t *testing.T, _ ids.UUID) {
			s.e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, s.e.Rep2)
		},
		"the backup owner's role cannot capture": func(s *sharedMailboxSuite, t *testing.T, _ ids.UUID) {
			s.e.MoveToRole(t, s.e.Rep2, "read_only")
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSharedMailboxSuite(t)
			box := s.backedMailbox(t)
			lose(s, t, box)
			s.syncOne(t, box, strangerMail("first@stranger.example"), "a question")
			s.judgeEveryone(t)

			if got := s.anyContactFor(t, strangerAddress); got != 0 {
				t.Fatalf("%d contacts minted with no backup owner who may hold them", got)
			}
			if _, all := s.questionsFor(t, strangerAddress); all != 0 {
				t.Fatalf("%d questions opened with no backup owner who may hold them", all)
			}
		})
	}
}

func TestASenderWhoseContactNoReaderMayReadGetsNoSecondContact(t *testing.T) {
	s := newSharedMailboxSuite(t)
	box := s.backedMailbox(t)
	pat := s.privateContact(t, "Pat Private", "pat@private.example", s.e.Rep3)
	s.syncOne(t, box, mailFrom("pat-1@private.example", "pat@private.example"), "hello from pat@private.example")

	if got := s.anyContactFor(t, "pat@private.example"); got != 1 {
		t.Fatalf("the address is held by %d contacts, want only the one a seat outside the readers owns", got)
	}
	if got := s.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE contact_id = $1`, pat); got != 0 {
		t.Fatal("shared mail was filed under a contact private to a seat outside the reader team")
	}
	s.judgeEveryone(t)
	if got := s.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE contact_id = $1`, pat); got != 0 {
		t.Fatal("a verdict filed shared mail under a contact private to a seat outside the reader team")
	}
}

func TestSharedMailFilesOnALeadOnlyWhenAReaderMayReadLeads(t *testing.T) {
	for name, tc := range map[string]struct {
		withoutLeadRead bool
		want            int
	}{
		"readers who may read leads":     {want: 1},
		"readers who may not read leads": {withoutLeadRead: true, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSharedMailboxSuite(t)
			box := ids.UUID(s.connect(t, s.e.Team1).Id)
			full, address := "Lena Lead", "lena@leads.example"
			lead, _, err := s.e.Contacts.CreateLead(s.e.Admin(), contacts.CreateLeadInput{FullName: &full, Email: &address, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.withoutLeadRead {
				s.e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = ANY($1)`, []ids.UUID{s.e.Rep1, s.e.Rep2})
				s.e.WsExec(t, `WITH r AS (INSERT INTO role (key, name, permissions)
				  VALUES ('no_leads', 'No leads', '{"contact": {"read": true}, "activity": {"read": true}}') RETURNING id)
				  INSERT INTO role_assignment (role_id, user_id) SELECT r.id, u FROM r, unnest($1::uuid[]) u`,
					[]ids.UUID{s.e.Rep1, s.e.Rep2})
			}
			s.syncOne(t, box, mailFrom("lena-1@leads.example", address), "hello from "+address)

			if got := s.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE lead_id = $1`, lead.Id); got != tc.want {
				t.Fatalf("shared mail is filed %d times on the lead, want %d", got, tc.want)
			}
		})
	}
}

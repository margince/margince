// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// One mail two colleagues both received is one timeline entry, through the
// production sink and two connected mailboxes. The second copy differs from the
// first. So the sink cannot prove one message and files it under the second
// seat's own key.

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/project360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	seatCopyKey    = "renewal-copies@acme.example"
	seatCopySender = "jonas@acme.example"
	seatCopyBody   = "please send the signed contract"
)

// seatCopyMail is one mailbox's copy of the message. The second mailbox got it
// in Bcc, so no visible line names it, and its file differs. The sink cannot
// prove one message, although the text matches.
func seatCopyMail(from, to, subject, body, file string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + seatCopyKey + ">",
		`Content-Type: multipart/mixed; boundary="part"`,
		"",
		"--part",
		"Content-Type: text/plain",
		"",
		body,
		"--part",
		`Content-Type: application/octet-stream; name="terms.pdf"`,
		`Content-Disposition: attachment; filename="terms.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte(file)),
		"--part--", "",
	}, "\r\n"))
}

// seatCopyEnv is two connected mailboxes and the sender as a known contact.
type seatCopyEnv struct {
	e             *integration.SearchEnv
	contact       ids.ContactID
	first, second func(t *testing.T)
	// secondFrom files the second mailbox's copy with this sender, subject and body.
	secondFrom func(t *testing.T, from, subject, body string)
	firstRow   func(t *testing.T) mailRow
}

// seatCopyPostures says which of the two mailboxes is shared. A shared one
// files mail every member may read; a classified one only for those on it.
type seatCopyPostures struct{ first, second bool }

// newSeatCopyEnv connects both mailboxes with the given postures.
func newSeatCopyEnv(t *testing.T, shared seatCopyPostures) seatCopyEnv {
	t.Helper()
	// A file store, so the first copy's file is kept and the proof compares it.
	env := newCaptureEnvWithBlob(t, captureNow, blobstore.NewMemory())
	e := env.e
	syncSecond := secondMailbox(t, e, e.Rep3)
	allowSharedPosture(t, e)
	for seat, isShared := range map[ids.UUID]bool{e.Rep1: shared.first, e.Rep3: shared.second} {
		if isShared {
			setPosture(t, env, seat, capturemod.PostureShared)
		}
	}
	created, err := contacts.NewStore(e.DB()).CreateContact(e.AsFullUser(), contacts.CreateContactInput{
		FullName: "Jonas", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: seatCopySender, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the sender: %v", err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(created.Id))
	s := seatCopyEnv{
		e: e, contact: contact,
		first: func(t *testing.T) {
			env.sync(t, seatCopyMail(seatCopySender, captureOwner,
				"Telematics renewal", seatCopyBody, "first mailbox's file"))
		},
		secondFrom: func(t *testing.T, from, subject, body string) {
			syncSecond(t, seatCopyMail(from, captureOwner, subject, body, "second mailbox's file"))
			rows := capturedMailRows(t, e)
			if len(rows) != 2 || rows[1].SourceID == seatCopyKey || connector.SharedMailKey(rows[1].SourceID) != seatCopyKey {
				t.Fatalf("want the second mailbox's copy under its own key, got %+v", rows)
			}
		},
		firstRow: func(t *testing.T) mailRow { return capturedMailRows(t, e)[0] },
	}
	s.second = func(t *testing.T) { s.secondFrom(t, seatCopySender, "Telematics renewal", seatCopyBody) }
	return s
}

func TestAReaderOfBothMailboxCopiesSeesOneEntry(t *testing.T) {
	s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: true})
	s.first(t)
	s.second(t)
	for _, reader := range []ids.UUID{s.e.Rep1, s.e.Rep3} {
		shown := emailTimeline(t, s.e, reader, true)
		if len(shown) != 1 {
			t.Fatalf("reader %s sees %d entries for one mail, want 1", reader, len(shown))
		}
		if shown[0].Copies == nil || *shown[0].Copies != 2 {
			t.Errorf("reader %s: copies = %v, want 2", reader, shown[0].Copies)
		}
		if shown[0].CopyIds == nil || len(*shown[0].CopyIds) != 1 || ids.UUID((*shown[0].CopyIds)[0]) == ids.UUID(shown[0].Id) {
			t.Errorf("reader %s: the entry must name the other copy, got %v", reader, shown[0].CopyIds)
		}
		if got := linksTo(shown[0], s.contact.UUID); got != 1 {
			t.Errorf("reader %s: the entry names the sender %d times, want once over both copies", reader, got)
		}
	}
	if got := len(emailTimeline(t, s.e, s.e.Rep1, false)); got != 2 {
		t.Errorf("a list that does not collapse returns %d rows, want both copies", got)
	}
	project := fileUnderProject(t, s.e)
	work, err := projectPage(s.e).Assemble(projectReader(s.e, s.e.Rep3), project)
	if err != nil {
		t.Fatalf("assembling the project page: %v", err)
	}
	if work.Activities == nil || len(work.Activities.Data) != 1 || work.Activities.Data[0].Copies == nil {
		t.Errorf("the project page's timeline should hold one entry with copies=2, got %d entries", entries(work.Activities))
	}
	fileCopiesUnder(t, s.e, "contact", s.contact.UUID)
	page, err := contactPage(s.e).Assemble(readerCtx(s.e, s.e.Rep3), s.contact)
	if err != nil {
		t.Fatalf("assembling the sender's page: %v", err)
	}
	if page.Activities == nil || len(page.Activities.Data) != 1 || page.Activities.Data[0].Copies == nil ||
		*page.Activities.Data[0].Copies != 2 {
		t.Error("the contact page's recent timeline should hold one entry with copies=2")
	}
}

// fileCopiesUnder files every captured copy under one record, as a colleague does by
// hand: the Bcc copy names no party the sink could link.
func fileCopiesUnder(t *testing.T, e *integration.SearchEnv, entityType string, entity ids.UUID) {
	t.Helper()
	for _, row := range capturedMailRows(t, e) {
		if _, err := activities.NewStore(e.DB()).RelinkActivity(seatWriter(e), ids.From[ids.ActivityKind](row.ID),
			activities.RelinkActivityInput{EntityType: entityType, EntityID: entity}); err != nil {
			t.Fatalf("filing %s under the %s: %v", row.ID, entityType, err)
		}
	}
}

// fileUnderProject files every captured copy under one new project.
func fileUnderProject(t *testing.T, e *integration.SearchEnv) ids.ProjectID {
	t.Helper()
	db, writer := e.DB(), seatWriter(e)
	company, err := contacts.NewStore(db).CreateCompany(writer, contacts.CreateCompanyInput{DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	project, err := integration.ProjectsStore(db).CreateProject(writer, projects.CreateProjectInput{
		Name: "Telematics", CompanyID: ids.From[ids.CompanyKind](ids.UUID(company.Id)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	fileCopiesUnder(t, e, "project", ids.UUID(project.Id))
	return ids.From[ids.ProjectKind](ids.UUID(project.Id))
}

// seatWriter is Rep1 allowed to create the records these tests file mail under.
func seatWriter(e *integration.SearchEnv) context.Context {
	return principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull,
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
				"company":  {Create: true, Read: true, Update: true},
				"project":  {Create: true, Read: true, Update: true},
				"contact":  {Read: true, Update: true},
				"activity": {Read: true, Update: true},
			}, RowScope: principal.RowScopeAll},
		})
}

// projectReader is readerCtx with the records a project page reads.
func projectReader(e *integration.SearchEnv, user ids.UUID) context.Context {
	read := principal.ObjectGrant{Read: true}
	return principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
			Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
				"activity": read, "contact": read, "company": read, "project": read, "deal": read,
			}, RowScope: principal.RowScopeAll},
		})
}

// projectPage is the project 360 assembled on this env's stores.
func projectPage(e *integration.SearchEnv) *project360.Service {
	db := e.DB()
	dealStore := deals.NewStore(db, installseam.Deals())
	return project360.NewService(e.Pool, dealStore, integration.ProjectsStore(db), contacts.NewStore(db),
		integration.ContractsStore(db, dealStore), activities.NewStore(db),
		func() time.Time { return captureNow.Add(24 * time.Hour) })
}

// contactPage is the contact 360 assembled on this env's stores.
func contactPage(e *integration.SearchEnv) *contact360.Service {
	db := e.DB()
	now := func() time.Time { return captureNow.Add(24 * time.Hour) }
	return contact360.NewService(e.Pool, contacts.NewStore(db), deals.NewStore(db, installseam.Deals()),
		projects.NewStore(db), consent.NewStore(db), comms.NewStore(db, now, activities.NewStore(db)),
		ai.NewFeedbackStore(db), now)
}

// The sender types the Message-ID. A mail reusing one that differs in subject,
// sender or body is a different message and keeps its own row.
func TestACopyThatDiffersInSubjectSenderOrBodyIsNotCollapsed(t *testing.T) {
	for name, mail := range map[string]struct{ from, subject, body string }{
		"subject": {seatCopySender, "Updated bank details", seatCopyBody},
		"sender":  {"billing@acme-payments.example", "Telematics renewal", seatCopyBody},
		"body":    {seatCopySender, "Telematics renewal", "our bank details changed, pay to the new account"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: true})
			s.first(t)
			s.secondFrom(t, mail.from, mail.subject, mail.body)
			shown := emailTimeline(t, s.e, s.e.Rep1, true)
			if len(shown) != 2 {
				t.Fatalf("a mail that reuses a Message-ID with another %s was hidden behind a count: %d entries", name, len(shown))
			}
			for _, row := range shown {
				if row.Copies != nil {
					t.Errorf("row %s carries copies=%d", row.Id, *row.Copies)
				}
			}
		})
	}
}

func TestAReaderOfOneCopySeesItWithoutACount(t *testing.T) {
	s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: false})
	s.first(t)
	s.second(t)
	first := s.firstRow(t)
	var own *crmcontracts.Activity
	for _, row := range emailTimeline(t, s.e, s.e.Rep1, true) {
		if row.Copies != nil {
			t.Errorf("row %s carries copies=%d for a reader who can open one copy", row.Id, *row.Copies)
		}
		if ids.UUID(row.Id) == first.ID {
			own = &row
		}
	}
	if own == nil || *own.ContentState != crmcontracts.ActivityContentStateAvailable {
		t.Fatalf("the reader's own copy is missing or withheld: %+v", own)
	}
	shown := emailTimeline(t, s.e, s.e.Rep3, true)
	if len(shown) != 1 || shown[0].Copies == nil || *shown[0].Copies != 2 {
		t.Errorf("the second seat reads both copies and should see one entry with copies=2, got %d entries", len(shown))
	}
}

func TestASecondMailboxCopyAddsNoExchange(t *testing.T) {
	s := newSeatCopyEnv(t, seatCopyPostures{first: true, second: true})
	s.first(t)
	before := exchanges(t, s.e, s.contact)
	s.second(t)
	fileCopiesUnder(t, s.e, "contact", s.contact.UUID)
	if after := exchanges(t, s.e, s.contact); before != 1 || after != before {
		t.Errorf("exchanges with the sender: %d before the second copy, %d after; want 1 both times", before, after)
	}
}

func TestSharedMailKeySQLAgreesWithSharedMailKey(t *testing.T) {
	e := integration.SetupSearch(t)
	for _, key := range []string{seatCopyKey, connector.SeatScopedMailKey(seatCopyKey, ids.NewV7().String())} {
		var got string
		err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT `+connector.SharedMailKeySQL("a")+
				` FROM (SELECT 'email'::text AS source_system, $1::text AS source_id) a`, key).Scan(&got)
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := connector.SharedMailKey(key); got != want {
			t.Errorf("the SQL shared key of %q is %q, SharedMailKey says %q", key, got, want)
		}
	}
}

// exchanges is the contact's interaction count over the last 90 days, as the
// relationship strength reads it.
func exchanges(t *testing.T, e *integration.SearchEnv, contact ids.ContactID) int {
	t.Helper()
	strength, err := contacts.NewStore(e.DB()).ContactStrength(e.Admin(), contact, captureNow.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("reading the contact's strength: %v", err)
	}
	return strength.InteractionCount90d
}

// entries counts a page's rows, zero for an absent section.
func entries(page *crmcontracts.ActivityListResponse) int {
	if page == nil {
		return 0
	}
	return len(page.Data)
}

func linksTo(a crmcontracts.Activity, entity ids.UUID) int {
	n := 0
	if a.Links != nil {
		for _, l := range *a.Links {
			if ids.UUID(l.EntityId) == entity {
				n++
			}
		}
	}
	return n
}

// emailTimeline is one reader's captured mail, read through the timeline list.
func emailTimeline(t *testing.T, e *integration.SearchEnv, user ids.UUID, collapse bool) []crmcontracts.Activity {
	t.Helper()
	kind := "email"
	rows, _, err := activities.NewStore(e.DB()).ListActivities(readerCtx(e, user), activities.ListActivitiesInput{
		Kind: &kind, CollapseSeatCopies: collapse,
	})
	if err != nil {
		t.Fatalf("reading the timeline: %v", err)
	}
	return rows
}

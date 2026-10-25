// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A document link sent by mail is an attachment row of the category `link`.
// The real capture writer writes every row here. The contact, company and deal
// Documents stores serve the lists.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const slidesLink = "https://docs.google.com/presentation/d/abc123/edit?usp=sharing"

// captureMailLink records one document link on a mail through the real writer,
// twice, as a replayed pull would.
func captureMailLink(t *testing.T, e *Env, mail ids.UUID, system, address, text string) ids.UUID {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:imap",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	store := activities.NewStore(e.DB())
	for range 2 {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return store.RecordCapturedLinks(ctx, tx, ids.From[ids.ActivityKind](mail),
				activities.CapturedFileSource{System: system, MessageID: "m-" + mail.String(), CapturedBy: "connector:imap", Category: "email_attachment"},
				[]activities.CapturedLink{{URL: address, Text: text}})
		}); err != nil {
			t.Fatalf("record link: %v", err)
		}
	}
	var id ids.UUID
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT id FROM attachment WHERE entity_id = $1 AND url = $2`, mail, address).Scan(&id); err != nil {
		t.Fatalf("read back the link: %v", err)
	}
	return id
}

func TestADocumentLinkIsListedWhereTheMailsFilesAre(t *testing.T) {
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	deal := e.SeedDeal(t, "Acme renewal", pipeline, stage, &e.Rep1)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:test')`, contact, company)
	mail := seedActivityWithContact(t, e, contact)
	linkMail(t, e, mail, "deal_id", deal)
	link := captureMailLink(t, e, mail, "imap", slidesLink, "Q4 plan")

	d, c, co := documentIDs(e.Admin(), t, e, deal, contact, company)
	if !d[link] || !c[link] || !co[link] {
		t.Errorf("link listed by deal=%v contact=%v company=%v, want all", d[link], c[link], co[link])
	}

	if n := e.WsCount(t, `SELECT count(*) FROM attachment WHERE entity_id = $1`, mail); n != 1 {
		t.Errorf("recording the same link twice left %d rows, want 1", n)
	}
	captureMailLink(t, e, mail, "gmail", slidesLink, "Q4 plan")
	if n := e.WsCount(t, `SELECT count(*) FROM attachment WHERE entity_id = $1`, mail); n != 2 {
		t.Errorf("a different adapter's copy is its own source key; got %d rows, want 2", n)
	}

	if v := e.WsScalar(t, `SELECT filename FROM attachment WHERE id = $1`, link); v != "Q4 plan" {
		t.Errorf("link row is named %q, want the anchor text", v)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1 || '%'`, link.String()); n != 0 {
		t.Errorf("a link row emitted %d events of its own, want 0", n)
	}
}

func TestADocumentLinkIsNotAFileOfItsMail(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	mail := seedActivityWithContact(t, e, contact)
	link := captureMailLink(t, e, mail, "imap", slidesLink, "")
	captureMailFile(t, e, mail, "contract.pdf", "application/pdf", []byte("%PDF-1.4"))

	if v := e.WsScalar(t, `SELECT filename FROM attachment WHERE id = $1`, link); v != "docs.google.com" {
		t.Errorf("link with no anchor text is named %q, want its host", v)
	}
	var counts map[ids.UUID]int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) (err error) {
		counts, err = activities.AttachmentCountsFor(e.Admin(), tx, []ids.UUID{mail})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if counts[mail] != 1 {
		t.Errorf("paperclip counts %d for a mail with one file and one link, want 1", counts[mail])
	}

	store := activities.NewStore(e.DB())
	if _, _, err := store.OpenAttachment(e.Admin(), link); !errors.Is(err, activities.ErrBytesWithheld) {
		t.Errorf("opening a link row answered %v, want ErrBytesWithheld", err)
	}
}

func TestADocumentLinkAuditHoldsTheHostNotTheAddress(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	mail := seedActivityWithContact(t, e, contact)
	secret := "https://drive.google.com/file/d/xyz/view?resourcekey=SECRET"
	link := captureMailLink(t, e, mail, "imap", secret, "Contract")

	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'attachment' AND entity_id = $1`, link); n != 1 {
		t.Fatalf("a link wrote %d audit rows, want 1", n)
	}
	image := e.WsScalar(t, `SELECT after::text FROM audit_log WHERE entity_type = 'attachment' AND entity_id = $1`, link)
	if strings.Contains(image, "SECRET") || !strings.Contains(image, "drive.google.com") {
		t.Errorf("audit image %s must hold the host and never the address", image)
	}
}

func TestADocumentLinkFollowsItsMailsAudience(t *testing.T) {
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	deal := e.SeedDeal(t, "D", pipeline, stage, &e.Rep1)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:test')`, contact, company)
	mail := seedActivityWithContact(t, e, contact)
	e.WsExec(t, `UPDATE activity SET audience = 'selected', captured_by = $2 WHERE id = $1`,
		mail, "connector:imap:"+e.Rep1.String())
	link := captureMailLink(t, e, mail, "imap", slidesLink, "Q4 plan")

	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	if _, c, co := documentIDs(owner, t, e, deal, contact, company); !c[link] || !co[link] {
		t.Errorf("the mailbox owner lost the link: contact=%v company=%v", c[link], co[link])
	}
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, AccountRepPerms)
	if _, c, co := documentIDs(colleague, t, e, deal, contact, company); c[link] || co[link] {
		t.Errorf("a colleague outside the audience sees the link: contact=%v company=%v", c[link], co[link])
	}
}

func TestAnArchivedDocumentLinkStaysArchivedWhenTheMailIsPulledAgain(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	pipeline, stage, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "D", pipeline, stage, &e.Rep1)
	mail := seedActivityWithContact(t, e, contact)
	link := captureMailLink(t, e, mail, "imap", slidesLink, "Q4 plan")

	if err := activities.NewStore(e.DB()).ArchiveAttachment(e.Admin(), link); err != nil {
		t.Fatalf("archive: %v", err)
	}
	captureMailLink(t, e, mail, "imap", slidesLink, "Q4 plan")
	if _, c, _ := documentIDs(e.Admin(), t, e, deal, contact, company); c[link] {
		t.Error("a replayed mail brought an archived link back")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM attachment WHERE entity_id = $1`, mail); n != 1 {
		t.Errorf("replaying after an archive left %d rows, want 1", n)
	}
}

func TestADocumentLinkRowCannotHoldBytesOrChangeCategory(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	mail := seedActivityWithContact(t, e, contact)
	link := captureMailLink(t, e, mail, "imap", slidesLink, "Q4 plan")

	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE attachment SET bytes_withheld = false WHERE id = $1`, link); err == nil {
		t.Error("the database accepted a link row that claims to hold bytes")
	}
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE attachment SET url = 'javascript:alert(1)' WHERE id = $1`, link); err == nil {
		t.Error("the database accepted a link row whose address is not http(s)")
	}
	other := "other"
	_, err := activities.NewStore(e.DB()).UpdateAttachmentMetadata(e.Admin(), link, activities.DocumentMetadata{Category: &other})
	if err == nil {
		t.Error("a metadata patch moved a link row out of its category")
	}
}

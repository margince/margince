// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Documents lists of a deal, a contact and a company all hold the files of
// the mails linked to them, read through the links as they are now. Every file
// here is written by the real capture writer: a hand-inserted row would carry
// whatever the test author expected, not what capture stamps.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureMailFile records one file on a mail through RecordCapturedFiles.
func captureMailFile(t *testing.T, e *Env, mail ids.UUID, name, contentType string, body []byte) ids.UUID {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:imap",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	staged, err := store.StageCapturedFiles(ctx, []activities.CapturedFile{{
		PartID: name, Filename: name, ContentType: contentType, Body: body,
	}})
	if err != nil {
		t.Fatalf("stage %s: %v", name, err)
	}
	record := func() {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return store.RecordCapturedFiles(ctx, tx, ids.From[ids.ActivityKind](mail),
				activities.CapturedFileSource{System: "imap", MessageID: "m-" + mail.String(), CapturedBy: "connector:imap", Category: "email_attachment"}, staged)
		}); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}
	record()
	record()
	var id ids.UUID
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT id FROM attachment WHERE entity_id = $1 AND filename = $2`, mail, name).Scan(&id); err != nil {
		t.Fatalf("read back %s: %v", name, err)
	}
	return id
}

// withholdMailFile records a file whose bytes capture did not keep, through the
// real withheld-file writer.
func withholdMailFile(t *testing.T, e *Env, mail ids.UUID, name string) ids.UUID {
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
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return store.RecordWithheldFiles(ctx, tx, ids.From[ids.ActivityKind](mail),
			activities.CapturedFileSource{System: "imap", MessageID: "w-" + mail.String(), CapturedBy: "connector:imap", Category: "email_attachment"},
			[]activities.WithheldFile{{PartID: name, Filename: name, ContentType: "application/pdf", ByteSize: 500000}})
	}); err != nil {
		t.Fatalf("withhold %s: %v", name, err)
	}
	var id ids.UUID
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT id FROM attachment WHERE entity_id = $1 AND filename = $2`, mail, name).Scan(&id); err != nil {
		t.Fatalf("read back %s: %v", name, err)
	}
	return id
}

func linkMail(t *testing.T, e *Env, mail ids.UUID, column string, record ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, `+column+`) VALUES ($1, $2, $3)`,
		mail, column[:len(column)-3], record)
}

// documentIDs reads the three lists as the caller and returns the attachment
// ids each holds.
func documentIDs(t *testing.T, e *Env, ctx context.Context, deal, contact, company ids.UUID) (dealDocs, contactDocs, companyDocs map[ids.UUID]bool) {
	t.Helper()
	store := activities.NewStore(e.DB())
	dealDocs, contactDocs, companyDocs = map[ids.UUID]bool{}, map[ids.UUID]bool{}, map[ids.UUID]bool{}
	dd, _, err := store.ListDealDocuments(ctx, deal, activities.DealDocumentFilters{})
	if err != nil {
		t.Fatalf("deal documents: %v", err)
	}
	for _, d := range dd {
		dealDocs[ids.UUID(d.Attachment.Id)] = true
	}
	cd, _, err := store.ListAttachments(ctx, "contact", contact, nil, nil)
	if err != nil {
		t.Fatalf("contact documents: %v", err)
	}
	for _, d := range cd {
		contactDocs[ids.UUID(d.Id)] = true
	}
	co, _, err := store.ListCompanyDocuments(ctx, company, activities.DocumentFilters{})
	if err != nil {
		t.Fatalf("company documents: %v", err)
	}
	for _, d := range co {
		companyDocs[ids.UUID(d.Id)] = true
	}
	return dealDocs, contactDocs, companyDocs
}

func TestAMailFileReachesEveryRecordTheMailIsLinkedTo(t *testing.T) {
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	deal := e.SeedDeal(t, "Acme renewal", pipeline, stage, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET company_id = $2 WHERE id = $1`, deal, company)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	other := e.SeedContact(t, "Bob", &e.Rep1)
	mail := seedActivityWithContact(t, e, contact)
	file := captureMailFile(t, e, mail, "contract.pdf", "application/pdf", []byte("%PDF-1.4"))
	ctx := e.Admin()

	_, byContact, byCompany := documentIDs(t, e, ctx, deal, contact, company)
	if !byContact[file] {
		t.Error("the contact the mail is linked to does not list its file")
	}
	if byCompany[file] {
		t.Error("the company lists a mail file before any contact works there")
	}
	if _, otherDocs, _ := documentIDs(t, e, ctx, deal, other, company); otherDocs[file] {
		t.Error("a contact the mail is not linked to lists its file")
	}

	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:test')`, contact, company)
	if _, _, byCompany = documentIDs(t, e, ctx, deal, contact, company); !byCompany[file] {
		t.Error("the company does not list the file after the contact joined it")
	}

	linkMail(t, e, mail, "deal_id", deal)
	byDeal, _, _ := documentIDs(t, e, ctx, deal, contact, company)
	if !byDeal[file] {
		t.Error("the deal the mail is linked to does not list its file")
	}

	if n := e.WsCount(t, `SELECT count(*) FROM attachment WHERE entity_id = $1`, mail); n != 1 {
		t.Errorf("recording the same part twice left %d rows, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'attachment' AND entity_id = $1`, file); n != 1 {
		t.Errorf("recording the same part twice wrote %d audit rows, want 1", n)
	}
	if v := e.WsScalar(t, `SELECT coalesce(company_id::text, '') FROM attachment WHERE id = $1`, file); v != "" {
		t.Errorf("a captured mail file was stamped with company %s", v)
	}
}

func TestAStaleCompanyStampDoesNotPlaceAMailFile(t *testing.T) {
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	reached := e.SeedCompany(t, "Reached", &e.Rep1)
	stamped := e.SeedCompany(t, "Stamped", &e.Rep1)
	deal := e.SeedDeal(t, "D", pipeline, stage, &e.Rep1)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:test')`, contact, reached)
	mail := seedActivityWithContact(t, e, contact)
	file := captureMailFile(t, e, mail, "old.pdf", "application/pdf", []byte("%PDF-1.4"))
	e.WsExec(t, `UPDATE attachment SET company_id = $2 WHERE id = $1`, file, stamped)

	_, _, byReached := documentIDs(t, e, e.Admin(), deal, contact, reached)
	_, _, byStamped := documentIDs(t, e, e.Admin(), deal, contact, stamped)
	if !byReached[file] || byStamped[file] {
		t.Errorf("reached lists it = %v, stamped lists it = %v; want true, false", byReached[file], byStamped[file])
	}
}

func TestAMailFileLeavesEveryListWhenItIsNotLive(t *testing.T) {
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	deal := e.SeedDeal(t, "D", pipeline, stage, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET company_id = $2 WHERE id = $1`, deal, company)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	mail := seedActivityWithContact(t, e, contact)
	linkMail(t, e, mail, "deal_id", deal)
	file := captureMailFile(t, e, mail, "big.pdf", "application/pdf", []byte("%PDF-1.4"))
	logo := captureMailFile(t, e, mail, "logo.png", "image/png", []byte("png"))
	held := withholdMailFile(t, e, mail, "held.pdf")

	d, c, co := documentIDs(t, e, e.Admin(), deal, contact, company)
	for name, got := range map[string]map[ids.UUID]bool{"deal": d, "contact": c} {
		if !got[file] {
			t.Errorf("%s list lost the live file", name)
		}
		if got[logo] {
			t.Errorf("%s list holds an inline-sized image", name)
		}
		if got[held] {
			t.Errorf("%s list holds a file whose bytes were withheld", name)
		}
	}
	if co[logo] || co[held] {
		t.Error("company list holds an inline image or a withheld file")
	}

	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, mail)
	d, c, co = documentIDs(t, e, e.Admin(), deal, contact, company)
	if d[file] || c[file] || co[file] {
		t.Errorf("an archived mail's file is listed: deal=%v contact=%v company=%v", d[file], c[file], co[file])
	}
}

func TestALimitedAudienceMailFileStaysWithItsAudience(t *testing.T) {
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
	file := captureMailFile(t, e, mail, "private.pdf", "application/pdf", []byte("%PDF-1.4"))

	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	if _, c, co := documentIDs(t, e, owner, deal, contact, company); !c[file] || !co[file] {
		t.Errorf("the mailbox owner lost the file: contact=%v company=%v", c[file], co[file])
	}
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, AccountRepPerms)
	if _, c, co := documentIDs(t, e, colleague, deal, contact, company); c[file] || co[file] {
		t.Errorf("a colleague outside the audience sees the file: contact=%v company=%v", c[file], co[file])
	}
	if _, c, co := documentIDs(t, e, e.Admin(), deal, contact, company); c[file] || co[file] {
		t.Errorf("an admin outside the audience sees the file: contact=%v company=%v", c[file], co[file])
	}
}

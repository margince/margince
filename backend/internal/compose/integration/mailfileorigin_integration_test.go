// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A mail's file on a Documents list names the mail it came with, to a reader
// who may read that mail and to nobody else.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mailFromBuyer seeds a mail from a named sender to a contact employed at the
// company, and captures one file on it.
func mailFromBuyer(t *testing.T, e *Env, contact ids.UUID, subject string) (mail, file ids.UUID) {
	t.Helper()
	mail = seedActivityWithContact(t, e, contact)
	e.WsExec(t, `UPDATE activity SET subject = $2 WHERE id = $1`, mail, subject)
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, role, address, display_name)
		VALUES ($1, $2, 'from', 'buyer@origin.test', 'Bea Buyer')`, ids.NewV7(), mail)
	return mail, captureMailFile(t, e, mail, "attachment.pdf", "application/pdf", []byte("%PDF-1.4"))
}

func TestAMailFileNamesTheMailItCameWith(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:test')`, contact, company)
	mail, file := mailFromBuyer(t, e, contact, "Signed order")
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	upload, err := store.UploadAttachment(e.As(e.Rep1, []ids.UUID{e.Team1}, docUploadPerms), activities.AttachmentInput{
		EntityType: "company", EntityID: company, Filename: "nda.pdf", Content: bytes.NewReader([]byte("bytes")),
	})
	if err != nil {
		t.Fatalf("uploading on the company: %v", err)
	}

	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	contactRows, _, err := store.ListAttachments(ctx, "contact", contact, nil, nil)
	if err != nil {
		t.Fatalf("contact documents: %v", err)
	}
	companyRows, _, err := store.ListCompanyDocuments(ctx, company, activities.DocumentFilters{})
	if err != nil {
		t.Fatalf("company documents: %v", err)
	}
	lists := map[string]map[ids.UUID]crmcontracts.Attachment{"contact": {}, "company": {}}
	for _, row := range contactRows {
		lists["contact"][ids.UUID(row.Id)] = row
	}
	for _, row := range companyRows {
		lists["company"][ids.UUID(row.Id)] = row
	}
	for list, rows := range lists {
		origin := rows[file].Origin
		if origin == nil {
			t.Fatalf("the %s list names no origin for the mail's file", list)
		}
		if ids.UUID(origin.ActivityId) != mail || origin.Kind != "email" || origin.Subject == nil || *origin.Subject != "Signed order" {
			t.Errorf("the %s list names origin %+v, want mail %s, kind email, subject %q", list, origin, mail, "Signed order")
		}
		if origin.Sender == nil || origin.Sender.Address != "buyer@origin.test" ||
			origin.Sender.DisplayName == nil || *origin.Sender.DisplayName != "Bea Buyer" {
			t.Errorf("the %s list names sender %+v, want Bea Buyer <buyer@origin.test>", list, origin.Sender)
		}
		if origin.OccurredAt.IsZero() {
			t.Errorf("the %s list's origin carries no sent date", list)
		}
	}
	if row, ok := lists["company"][ids.UUID(upload.Id)]; !ok || row.Origin != nil {
		t.Errorf("the upload is listed = %v with origin %+v, want listed with none", ok, row.Origin)
	}
}

func TestAMailFileTheReaderMayNotReadNamesNoMail(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	contact := e.SeedContact(t, "Ada", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:test')`, contact, company)
	mail, file := mailFromBuyer(t, e, contact, "Salary talk")
	e.WsExec(t, `UPDATE activity SET audience = 'selected', captured_by = $2 WHERE id = $1`,
		mail, "connector:imap:"+e.Rep1.String())
	store := activities.NewStore(e.DB())

	for name, reader := range map[string]ids.UUID{"the mailbox owner": e.Rep1, "a colleague outside the audience": e.Rep2} {
		ctx := e.As(reader, []ids.UUID{e.Team1}, AccountRepPerms)
		contactRows, _, err := store.ListAttachments(ctx, "contact", contact, nil, nil)
		if err != nil {
			t.Fatalf("contact documents: %v", err)
		}
		companyRows, _, err := store.ListCompanyDocuments(ctx, company, activities.DocumentFilters{})
		if err != nil {
			t.Fatalf("company documents: %v", err)
		}
		body, err := json.Marshal([][]crmcontracts.Attachment{contactRows, companyRows})
		if err != nil {
			t.Fatal(err)
		}
		mayRead := reader == e.Rep1
		if got := strings.Contains(string(body), file.String()); got != mayRead {
			t.Errorf("%s: the file is listed = %v, want %v", name, got, mayRead)
		}
		for _, secret := range []string{"Salary talk", "buyer@origin.test", "Bea Buyer"} {
			if got := strings.Contains(string(body), secret); got != mayRead {
				t.Errorf("%s: the lists name %q = %v, want %v", name, secret, got, mayRead)
			}
		}
	}
}

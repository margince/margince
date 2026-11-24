// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package systemmail

// The attachment reads over real Postgres, as a reader with its own grants. The
// files are written by capture's own writer.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

type filesEnv struct {
	*integration.Env
	blob blobstore.Store
}

// mail adds a mail with the audience given, and runs extra as its UPDATE.
func (e filesEnv) mail(t *testing.T, audience, extra string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by, audience, audience_reason)
		VALUES ($1, 'email', 'Quote', 'Hello', 'system', 'system', $2, CASE WHEN $2 = 'workspace' THEN NULL ELSE 'manual' END)`, id, audience)
	if extra != "" {
		e.WsExec(t, `UPDATE activity SET `+extra+` WHERE id = $1`, id)
	}
	return id
}

func (e filesEnv) file(t *testing.T, mail ids.UUID, name string) ids.UUID {
	t.Helper()
	return e.CaptureMailFile(t, e.blob, mail, name, "application/octet-stream", []byte("%PDF-1.4 "+name))
}

// reader is a seat with its own role, as the unit's service member is.
func (e filesEnv) reader(t *testing.T, scope principal.RowScope) context.Context {
	t.Helper()
	seat := ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name, status, is_agent) VALUES ($1, $2, 'Feed', 'active', true)`,
		seat, "svc-"+seat.String()+"@members.invalid")
	return e.As(seat, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{"activity": {Read: true}, "company": {Read: true}}, RowScope: scope,
	})
}

// in runs fn in a transaction of the reader's context, and answers its error as
// a unit sees it, or the core's own fault.
func (e filesEnv) in(t *testing.T, ctx context.Context, fn func(pgx.Tx) error) error {
	t.Helper()
	err := database.WithWorkspaceTx(ctx, e.Pool, fn)
	if published := Published(err); published != nil || err == nil {
		return published
	}
	t.Fatalf("a core fault: %v", err)
	return nil
}

func setupFiles(t *testing.T) filesEnv {
	t.Helper()
	return filesEnv{Env: integration.Setup(t), blob: blobstore.NewMemory()}
}

func (e filesEnv) open(t *testing.T, ctx context.Context, files Files, id ids.UUID) (extension.MailAttachment, error) {
	t.Helper()
	var got extension.MailAttachment
	err := e.in(t, ctx, func(tx pgx.Tx) (err error) {
		got, _, err = files.Open(ctx, tx, id)
		return err
	})
	return got, err
}

func TestAWorkspaceMailsFileListsOpensAndReadsItsBytes(t *testing.T) {
	e := setupFiles(t)
	mail := e.mail(t, "workspace", "")
	quote := e.file(t, mail, "quote.bin")
	ctx, files := e.reader(t, principal.RowScopeAll), Files{Unit: "mirror", ActAs: ids.NewV7(), Blob: e.blob}
	var (
		listed []extension.MailAttachment
		key    string
		opened extension.MailAttachment
	)
	err := e.in(t, ctx, func(tx pgx.Tx) (err error) {
		if listed, err = files.List(ctx, tx, mail); err != nil {
			return err
		}
		opened, key, err = files.Open(ctx, tx, quote)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != quote.String() || listed[0].MailID != mail.String() || listed[0].Filename != "quote.bin" {
		t.Fatalf("List = %+v, want the one file of the mail", listed)
	}
	read, err := files.Read(ctx, opened, key)
	if err != nil || string(read.Content) != "%PDF-1.4 quote.bin" || read.ContentType != "application/pdf" {
		t.Errorf("Read = %q as %q, %v; want the stored bytes typed by what they hold", read.Content, read.ContentType, err)
	}
	for kind, want := range map[string]int{"extension_read": 1, Opened: 1} {
		rows := e.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'export' AND detail->>'kind' = $1
			AND detail->>'extension' = 'mirror' AND detail->>'act_as' = $2`, kind, files.ActAs.String())
		if rows != want {
			t.Errorf("%d export rows of kind %s, want %d", rows, kind, want)
		}
	}
}

// heldMail is a mail under a statutory hold, with the evidence the table asks for.
func (e filesEnv) heldMail(t *testing.T) ids.UUID {
	t.Helper()
	id := e.mail(t, "workspace", "")
	e.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, decided_by_name, reason, qualified_at)
		VALUES ($1, 'controller_pin', 'Test Controller', 'statutory retention under test', now())`, id)
	return id
}

// restrictedMail has a contact under a restriction of processing among its
// participants.
func (e filesEnv) restrictedMail(t *testing.T) ids.UUID {
	t.Helper()
	mail, contact := e.mail(t, "workspace", ""), ids.NewV7()
	e.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by, visibility) VALUES ($1, 'Restricted', 'manual', 'system', 'workspace')`, contact)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'attendee')`, mail, contact)
	e.WsExec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:test', 'subject')`, contact)
	return mail
}

func TestEveryHiddenFileAnswersAsOneThatDoesNotExist(t *testing.T) {
	e := setupFiles(t)
	hidden := map[string]ids.UUID{
		"participants-only":  e.mail(t, "participants", ""),
		"archived mail":      e.mail(t, "workspace", ""),
		"held":               e.heldMail(t),
		"Art. 18 restricted": e.restrictedMail(t),
		"not an email":       e.mail(t, "workspace", ""),
	}
	files := map[string]ids.UUID{}
	for name, mail := range hidden {
		files[name] = e.file(t, mail, name)
	}
	// Each mail turns hidden after its file arrived, as in the product.
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, hidden["archived mail"])
	e.WsExec(t, `UPDATE activity SET kind = 'note' WHERE id = $1`, hidden["not an email"])
	e.WsExec(t, `UPDATE activity SET retention_class = 'commercial_correspondence', retention_class_at = now(), restricted_at = now(),
		restricted_reason = 'statutory retention', restricted_until = now() + interval '6 years', archived_at = now() WHERE id = $1`, hidden["held"])
	open := e.mail(t, "workspace", "")
	files["withheld file"], files["archived file"] = e.file(t, open, "withheld"), e.file(t, open, "archived")
	e.WsExec(t, `UPDATE attachment SET bytes_withheld = true WHERE id = $1`, files["withheld file"])
	e.WsExec(t, `UPDATE attachment SET archived_at = now() WHERE id = $1`, files["archived file"])
	files["no such file"] = ids.NewV7()

	ctx, reader := e.reader(t, principal.RowScopeAll), Files{Unit: "mirror", ActAs: ids.NewV7(), Blob: e.blob}
	for name, id := range files {
		if _, err := e.open(t, ctx, reader, id); !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("opening the %s: err = %v, want ErrNotFound", name, err)
		}
	}
	for name, mail := range hidden {
		err := e.in(t, ctx, func(tx pgx.Tx) error { _, err := reader.List(ctx, tx, mail); return err })
		if !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("listing the %s mail: err = %v, want ErrNotFound", name, err)
		}
	}
	var left []extension.MailAttachment
	if err := e.in(t, ctx, func(tx pgx.Tx) (err error) { left, err = reader.List(ctx, tx, open); return err }); err != nil || len(left) != 0 {
		t.Errorf("a mail whose files are withheld or archived listed %+v, %v; want none", left, err)
	}
	if rows := e.WsCount(t, `SELECT count(*) FROM system_log WHERE detail->>'kind' = $1`, Opened); rows != 0 {
		t.Errorf("refused opens left %d export rows, want 0", rows)
	}
}

func TestAFileOutsideTheReadersScopeAnswersAsOneThatDoesNotExist(t *testing.T) {
	e := setupFiles(t)
	ctx := e.reader(t, principal.RowScopeOwn)
	seat, _ := principal.Actor(ctx)
	onCompanyOf := func(owner ids.UUID, name string) ids.UUID {
		company, mail := ids.NewV7(), e.mail(t, "workspace", "")
		e.WsExec(t, `INSERT INTO company (id, display_name, source, captured_by, owner_id, visibility)
			VALUES ($1, $2, 'system', 'system', $3, 'owner')`, company, name, owner)
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, mail, company)
		return e.file(t, mail, name)
	}
	mine, theirs := onCompanyOf(seat.UserID, "Mine GmbH"), onCompanyOf(e.Rep1, "Theirs GmbH")
	reader := Files{Unit: "mirror", ActAs: seat.UserID, Blob: e.blob}
	if _, err := e.open(t, ctx, reader, theirs); !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("a colleague's file: err = %v, want ErrNotFound", err)
	}
	if _, err := e.open(t, ctx, reader, mine); err != nil {
		t.Errorf("a file on the reader's own company: err = %v, want it opened", err)
	}
}

func TestAUnitPastItsDailyOpensIsRefusedAndAnotherUnitIsNot(t *testing.T) {
	e := setupFiles(t)
	file := e.file(t, e.mail(t, "workspace", ""), "quote")
	ctx := e.reader(t, principal.RowScopeAll)
	capped, other := Files{Unit: "mirror", ActAs: ids.NewV7(), Blob: e.blob}, Files{Unit: "other", ActAs: ids.NewV7(), Blob: e.blob}
	for i := range extension.MaxAttachmentOpensPerDay {
		if _, err := e.open(t, ctx, capped, file); err != nil {
			t.Fatalf("open %d of the day: %v", i+1, err)
		}
	}
	if _, err := e.open(t, ctx, capped, file); !errors.Is(err, extension.ErrAttachmentCapReached) {
		t.Errorf("one open past the cap: err = %v, want ErrAttachmentCapReached", err)
	}
	if _, err := e.open(t, ctx, capped, ids.NewV7()); !errors.Is(err, extension.ErrAttachmentCapReached) {
		t.Errorf("an unknown file past the cap: err = %v, want the cap, which tells nothing about the file", err)
	}
	if _, err := e.open(t, ctx, other, file); err != nil {
		t.Errorf("another unit's first open: err = %v, want it opened", err)
	}
}

func TestAFileOverTheBoundIsRefused(t *testing.T) {
	e := setupFiles(t)
	file := e.file(t, e.mail(t, "workspace", ""), "huge")
	e.WsExec(t, `UPDATE attachment SET byte_size = $2 WHERE id = $1`, file, extension.MaxInboundFileBytes+1)
	reader := Files{Unit: "mirror", ActAs: ids.NewV7(), Blob: e.blob}
	if _, err := e.open(t, e.reader(t, principal.RowScopeAll), reader, file); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a file over the bound: err = %v, want ErrInvalid", err)
	}
}

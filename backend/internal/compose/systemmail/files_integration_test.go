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

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

type filesEnv struct {
	*integration.Env
	blob blobstore.Store
}

// mail adds a mail with the audience given.
func (e filesEnv) mail(t *testing.T, audience string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by, audience, audience_reason)
		VALUES ($1, 'email', 'Quote', 'Hello', 'system', 'system', $2, CASE WHEN $2 = 'workspace' THEN NULL ELSE 'manual' END)`, id, audience)
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

func setupFiles(t *testing.T) filesEnv {
	t.Helper()
	return filesEnv{Env: integration.Setup(t), blob: blobstore.NewMemory()}
}

// files is the reader of one unit, with a service member of its own.
func (e filesEnv) files(unit string) Files {
	return Files{Unit: unit, ActAs: ids.NewV7(), Blob: e.blob, Pool: e.Pool}
}

// publishedOrFail answers err as a unit sees it, and fails the test on a core fault.
func publishedOrFail(t *testing.T, err error) error {
	t.Helper()
	if seen := Published(err); seen != nil || err == nil {
		return seen
	}
	t.Fatalf("a core fault: %v", err)
	return nil
}

func (e filesEnv) open(ctx context.Context, t *testing.T, files Files, id ids.UUID) (extension.MailAttachmentFile, error) {
	t.Helper()
	got, err := files.Open(ctx, id)
	return got, publishedOrFail(t, err)
}

func (e filesEnv) list(ctx context.Context, t *testing.T, files Files, mail ids.UUID) ([]extension.MailAttachment, error) {
	t.Helper()
	got, err := files.List(ctx, mail)
	return got, publishedOrFail(t, err)
}

// exports counts the export rows of one kind that the mirror unit left.
func (e filesEnv) exports(t *testing.T, kind string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'export' AND detail->>'kind' = $1 AND detail->>'extension' = 'mirror'`, kind)
}

func TestAWorkspaceMailsFileListsOpensAndReadsItsBytes(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	mail := e.mail(t, "workspace")
	quote := e.file(t, mail, "quote.bin")
	ctx, files := e.reader(t, principal.RowScopeAll), e.files("mirror")
	listed, err := e.list(ctx, t, files, mail)
	if err != nil || len(listed) != 1 || listed[0].ID != quote.String() || listed[0].MailID != mail.String() || listed[0].Filename != "quote.bin" {
		t.Fatalf("List = %+v, %v; want the one file of the mail", listed, err)
	}
	opened, err := e.open(ctx, t, files, quote)
	if err != nil || string(opened.Content) != "%PDF-1.4 quote.bin" || opened.ContentType != "application/pdf" {
		t.Errorf("Open = %q as %q, %v; want the stored bytes typed by what they hold", opened.Content, opened.ContentType, err)
	}
	for kind, want := range map[string]int{"extension_read": 1, Opened: 1} {
		if rows := e.exports(t, kind); rows != want {
			t.Errorf("%d export rows of kind %s, want %d", rows, kind, want)
		}
	}
}

// heldMail is a mail under a statutory hold, with the evidence the table asks for.
func (e filesEnv) heldMail(t *testing.T) ids.UUID {
	t.Helper()
	id := e.mail(t, "workspace")
	e.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, decided_by_name, reason, qualified_at)
		VALUES ($1, 'controller_pin', 'Test Controller', 'statutory retention under test', now())`, id)
	return id
}

// restrictedMail has a contact under a restriction of processing among its
// participants.
func (e filesEnv) restrictedMail(t *testing.T) ids.UUID {
	t.Helper()
	mail, contact := e.mail(t, "workspace"), ids.NewV7()
	e.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by, visibility) VALUES ($1, 'Restricted', 'manual', 'system', 'workspace')`, contact)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'attendee')`, mail, contact)
	e.WsExec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:test', 'subject')`, contact)
	return mail
}

func TestEveryHiddenFileAnswersAsOneThatDoesNotExist(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	hidden := map[string]ids.UUID{
		"participants-only":  e.mail(t, "participants"),
		"archived mail":      e.mail(t, "workspace"),
		"held":               e.heldMail(t),
		"Art. 18 restricted": e.restrictedMail(t),
		"not an email":       e.mail(t, "workspace"),
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
	open := e.mail(t, "workspace")
	files["withheld file"], files["archived file"] = e.file(t, open, "withheld"), e.file(t, open, "archived")
	e.WsExec(t, `UPDATE attachment SET bytes_withheld = true WHERE id = $1`, files["withheld file"])
	e.WsExec(t, `UPDATE attachment SET archived_at = now() WHERE id = $1`, files["archived file"])
	files["file a user uploaded"] = e.file(t, open, "uploaded")
	e.WsExec(t, `UPDATE attachment SET category = 'other', source = 'upload' WHERE id = $1`, files["file a user uploaded"])
	files["no such file"] = ids.NewV7()

	ctx, reader := e.reader(t, principal.RowScopeAll), e.files("mirror")
	for name, id := range files {
		if _, err := e.open(ctx, t, reader, id); !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("opening the %s: err = %v, want ErrNotFound", name, err)
		}
	}
	for name, mail := range hidden {
		if _, err := e.list(ctx, t, reader, mail); !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("listing the %s mail: err = %v, want ErrNotFound", name, err)
		}
	}
	if left, err := e.list(ctx, t, reader, open); err != nil || len(left) != 0 {
		t.Errorf("a mail whose files are withheld, archived or uploaded listed %+v, %v; want none", left, err)
	}
	if rows := e.WsCount(t, `SELECT count(*) FROM system_log WHERE detail->>'kind' = $1`, Opened); rows != 0 {
		t.Errorf("refused opens left %d export rows, want 0", rows)
	}
}

func TestAFileOutsideTheReadersScopeAnswersAsOneThatDoesNotExist(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	ctx := e.reader(t, principal.RowScopeOwn)
	seat, _ := principal.Actor(ctx)
	type mailFile struct{ mail, file ids.UUID }
	onCompanyOf := func(owner ids.UUID, name string) mailFile {
		company, mail := ids.NewV7(), e.mail(t, "workspace")
		e.WsExec(t, `INSERT INTO company (id, display_name, source, captured_by, owner_id, visibility)
			VALUES ($1, $2, 'system', 'system', $3, 'owner')`, company, name, owner)
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, mail, company)
		return mailFile{mail: mail, file: e.file(t, mail, name)}
	}
	mine, theirs := onCompanyOf(seat.UserID, "Mine GmbH"), onCompanyOf(e.Rep1, "Theirs GmbH")
	reader := e.files("mirror")
	if _, err := e.open(ctx, t, reader, theirs.file); !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("opening a colleague's file: err = %v, want ErrNotFound", err)
	}
	if _, err := e.list(ctx, t, reader, theirs.mail); !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("listing a colleague's mail: err = %v, want ErrNotFound", err)
	}
	if _, err := e.open(ctx, t, reader, mine.file); err != nil {
		t.Errorf("opening a file on the reader's own company: err = %v, want it opened", err)
	}
	if listed, err := e.list(ctx, t, reader, mine.mail); err != nil || len(listed) != 1 {
		t.Errorf("listing the reader's own mail: %+v, %v; want its file", listed, err)
	}
}

func TestAUnitPastItsDailyOpensIsRefusedAndAnotherUnitIsNot(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	file := e.file(t, e.mail(t, "workspace"), "quote")
	ctx := e.reader(t, principal.RowScopeAll)
	capped, other := e.files("mirror"), e.files("other")
	for i := range extension.MaxAttachmentOpensPerDay {
		if _, err := e.open(ctx, t, capped, file); err != nil {
			t.Fatalf("open %d of the day: %v", i+1, err)
		}
	}
	if _, err := e.open(ctx, t, capped, file); !errors.Is(err, extension.ErrAttachmentCapReached) {
		t.Errorf("one open past the cap: err = %v, want ErrAttachmentCapReached", err)
	}
	if _, err := e.open(ctx, t, capped, ids.NewV7()); !errors.Is(err, extension.ErrAttachmentCapReached) {
		t.Errorf("an unknown file past the cap: err = %v, want the cap, which tells nothing about the file", err)
	}
	if _, err := e.open(ctx, t, other, file); err != nil {
		t.Errorf("another unit's first open: err = %v, want it opened", err)
	}
}

func TestAFileOverTheBoundIsRefused(t *testing.T) {
	t.Parallel()
	e := setupFiles(t)
	file := e.file(t, e.mail(t, "workspace"), "huge")
	e.WsExec(t, `UPDATE attachment SET byte_size = $2 WHERE id = $1`, file, extension.MaxInboundFileBytes+1)
	if _, err := e.open(e.reader(t, principal.RowScopeAll), t, e.files("mirror"), file); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a file over the bound: err = %v, want ErrInvalid", err)
	}
}

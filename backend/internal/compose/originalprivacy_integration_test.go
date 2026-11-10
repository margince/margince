// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type failingOriginalBlob struct {
	blobstore.Store
	failPut, failDelete bool
	gets                atomic.Int64
}

func (b *failingOriginalBlob) Get(ctx context.Context, key string) (io.ReadCloser, blobstore.Object, error) {
	b.gets.Add(1)
	return b.Store.Get(ctx, key)
}

func (b *failingOriginalBlob) Put(ctx context.Context, key string, body io.Reader, size int64, kind string) error {
	if b.failPut {
		return errors.New("object write failed")
	}
	return b.Store.Put(ctx, key, body, size, kind)
}

func (b *failingOriginalBlob) Delete(ctx context.Context, key string) error {
	if b.failDelete {
		return errors.New("object deletion failed")
	}
	return b.Store.Delete(ctx, key)
}

type failingOriginalCertificate struct {
	capturedTextPurger
	fail bool
}

func (p *failingOriginalCertificate) ScrubDeliveryCopiesTx(ctx context.Context, tx pgx.Tx, kind string, ids []ids.UUID) error {
	if p.fail {
		return errors.New("certificate transaction failed after original destruction")
	}
	return p.capturedTextPurger.ScrubDeliveryCopiesTx(ctx, tx, kind, ids)
}

func storeSubjectOriginal(t *testing.T, e *integration.Env, blob blobstore.Store, source, email string) originalStorageRow {
	t.Helper()
	ctx := originalstore.Bind(e.Admin(), e.DB(), blob)
	payload, err := json.Marshal(map[string]string{"email": email, "text": strings.Repeat("x", originalstore.InlineLimit)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		_, err := capture.InsertRawCaptureTx(ctx, tx, capture.RawRecord{SourceSystem: "fixture", SourceID: source, Payload: payload})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	row := originalRow(ctx, t, e.DB(), "fixture", source)
	if row.object == nil {
		t.Fatal("fixture did not exercise external storage")
	}
	return row
}

func TestOriginalErasureRetriesAfterDeletionAndTransactionRollback(t *testing.T) {
	e := integration.Setup(t)
	email := "original-subject@example.test"
	contact := seedContactWithAddress(t, e, "Original subject", email)
	blob := &failingOriginalBlob{Store: blobstore.NewMemory()}
	row := storeSubjectOriginal(t, e, blob, "subject", email)
	other := storeSubjectOriginal(t, e, blob, "unrelated", "somebody-else@example.test")
	purger := &failingOriginalCertificate{fail: true}
	eraser := privacy.NewEraser(e.DB()).WithBlobstore(blob).WithRawCapturePurger(purger)
	if err := eraser.EraseContact(e.Admin(), contact, "subject request"); err == nil {
		t.Fatal("the final transaction failure was hidden")
	}
	if _, _, err := blob.Get(e.Admin(), row.object.Key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("object was not destroyed before rollback: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM raw_capture WHERE id = $1`, row.id); got != 1 {
		t.Fatal("rollback did not preserve the reference")
	}
	cached := originalRow(e.Admin(), t, e.DB(), "fixture", "subject")
	if cached.object == nil || len(cached.object.MatchedSubjects) != 1 || cached.object.MatchedSubjects[0] != contact.String() {
		t.Fatal("rollback lost subject ownership needed for retry")
	}
	otherContact := seedContactWithAddress(t, e, "Another subject", "different-original-subject@example.test")
	if _, err := privacy.AssembleSAR(originalstore.Bind(e.Admin(), e.DB(), blob), e.DB(), ids.From[ids.ContactKind](otherContact)); err != nil {
		t.Fatal("a different subject was blocked by the pending erasure retry", err)
	}
	purger.fail = false
	if err := eraser.EraseContact(e.Admin(), contact, "subject request"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM raw_capture WHERE id = $1`, row.id); got != 0 {
		t.Fatal("retry retained the original")
	}
	body, _, err := blob.Get(e.Admin(), other.object.Key)
	if err != nil {
		t.Fatal("erasure destroyed another subject's object", err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedOriginalDeletionKeepsErasureRetryable(t *testing.T) {
	e := integration.Setup(t)
	email := "delete-subject@example.test"
	contact := seedContactWithAddress(t, e, "Delete subject", email)
	blob := &failingOriginalBlob{Store: blobstore.NewMemory()}
	row := storeSubjectOriginal(t, e, blob, "delete", email)
	blob.failDelete = true
	eraser := privacy.NewEraser(e.DB()).WithBlobstore(blob)
	if err := eraser.EraseContact(e.Admin(), contact, "subject request"); err == nil {
		t.Fatal("failed object deletion certified erasure")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM raw_capture WHERE id = $1`, row.id); got != 1 {
		t.Fatal("failed deletion lost its reference")
	}
	blob.failDelete = false
	if err := eraser.EraseContact(e.Admin(), contact, "subject request"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := blob.Get(e.Admin(), row.object.Key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatal("retry did not destroy object", err)
	}
}

func TestSubjectExportsHydrateOnlyReleasedOriginals(t *testing.T) {
	e := integration.Setup(t)
	email := "export-subject@example.test"
	contact := seedContactWithAddress(t, e, "Export subject", email)
	blob := blobstore.NewMemory()
	row := storeSubjectOriginal(t, e, blob, "export", email)
	ctx := originalstore.Bind(e.Admin(), e.DB(), blob)
	pkg, err := privacy.AssembleSAR(ctx, e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.RawCapture) != 1 || pkg.RawCapture[0]["payload"] != nil || pkg.RawCapture[0]["content_disclosed"] != false {
		t.Fatal("unlinked original was disclosed")
	}
	e.WsExec(t, `INSERT INTO activity (kind, subject, body, occurred_at, source, source_system, source_id, raw_capture_id, captured_by)
 VALUES ('email','Subject original','content',now(),'capture_email','fixture','export', $1,'connector:test')`, row.id)
	pkg, err = privacy.AssembleSAR(ctx, e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(pkg.RawCapture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(email)) || bytes.Contains(encoded, []byte(row.object.Key)) || bytes.Contains(encoded, []byte("sha256")) {
		t.Fatal("export did not restore provider content or leaked storage metadata")
	}
	e.WsExec(t, `UPDATE activity SET audience='participants' WHERE raw_capture_id=$1`, row.id)
	pkg, err = privacy.AssembleSAR(ctx, e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.RawCapture) != 1 || pkg.RawCapture[0]["payload"] != nil {
		t.Fatal("held original escaped disclosure gate")
	}
}

func TestOriginalSubjectHintsDoNotDownloadUnrelatedEvidence(t *testing.T) {
	e := integration.Setup(t)
	blob := &failingOriginalBlob{Store: blobstore.NewMemory()}
	subject := seedContactWithAddress(t, e, "Indexed subject", "alice@first.example.test")
	other := seedContactWithAddress(t, e, "Unrelated subject", "bob@second.example.test")
	storeSubjectOriginal(t, e, blob, "indexed", "alice@first.example.test")
	ctx := originalstore.Bind(e.Admin(), e.DB(), blob)
	unrelated, err := privacy.AssembleSAR(ctx, e.DB(), ids.From[ids.ContactKind](other))
	if err != nil {
		t.Fatal(err)
	}
	if len(unrelated.RawCapture) != 0 || blob.gets.Load() != 0 {
		t.Fatal("unrelated export read the object's bytes")
	}
	if _, err := privacy.AssembleNoticeDisclosure(principal.SystemActing(ctx, "notice_fixture"), e.DB(), nil, privacy.NoticeSubject{ContactID: ids.From[ids.ContactKind](other)}); err != nil {
		t.Fatal(err)
	}
	if blob.gets.Load() != 0 {
		t.Fatal("unrelated notice read the object's bytes")
	}
	related, err := privacy.AssembleSAR(ctx, e.DB(), ids.From[ids.ContactKind](subject))
	if err != nil {
		t.Fatal(err)
	}
	if len(related.RawCapture) != 1 || blob.gets.Load() != 1 {
		t.Fatal("search hints skipped an actual subject match")
	}
}

func TestMissingOriginalDoesNotBlockTheDocumentLinkRepairCursor(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	ctx := originalstore.Bind(asConnectorFor(e.Admin(), "connector:imap", e.AdminUser), e.DB(), blob)
	record := mailRecord("missing-repair-original")
	record.Raw = []byte("From: her@example.com\r\n\r\n" + strings.Repeat("original ", originalstore.InlineLimit/5))
	if _, err := capture.NewSink(e.DB()).Upsert(ctx, record); err != nil {
		t.Fatal(err)
	}
	row := originalRow(ctx, t, e.DB(), "email", record.NaturalKey.SourceID)
	if row.object == nil {
		t.Fatal("fixture stayed inline")
	}
	if err := blob.Delete(ctx, row.object.Key); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(ctx, "document_link_repair")
	if err := e.DB().Tx(system, func(tx pgx.Tx) error {
		result, err := recordDocumentLinksBatch(system, tx, datatask.BatchIn{Limit: 10})
		if err != nil {
			return err
		}
		if result.Scanned != 1 || result.Changed != 0 {
			t.Errorf("repair did not advance past missing evidence: %+v", result)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

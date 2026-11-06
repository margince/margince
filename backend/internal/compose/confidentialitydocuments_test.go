// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedFile is what the fake object store holds for one attachment id.
type storedFile struct {
	contentType string
	raw         []byte
	err         error
}

// fakeAttachments is the object-store boundary: it serves bytes by id and
// counts what was fetched.
type fakeAttachments struct {
	files   map[ids.UUID]storedFile
	fetched []ids.UUID
}

func (f *fakeAttachments) OpenAttachment(_ context.Context, id ids.UUID) (crmcontracts.Attachment, io.ReadCloser, error) {
	f.fetched = append(f.fetched, id)
	file, ok := f.files[id]
	if !ok {
		return crmcontracts.Attachment{}, nil, apperrors.ErrNotFound
	}
	if file.err != nil {
		return crmcontracts.Attachment{}, nil, file.err
	}
	return crmcontracts.Attachment{ContentType: &file.contentType}, io.NopCloser(bytes.NewReader(file.raw)), nil
}

func salarySheet(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../platform/doctext/testdata/salaries.xlsx")
	if err != nil {
		t.Fatalf("reading the salary workbook fixture: %v", err)
	}
	return raw
}

func readerOver(files map[ids.UUID]storedFile) (*threadDocuments, *fakeAttachments) {
	fake := &fakeAttachments{files: files}
	return &threadDocuments{files: fake, pdf: processPDFReader, slots: make(chan struct{}, 1)}, fake
}

func TestAThreadsSalarySheetReachesThePromptInsideTheFence(t *testing.T) {
	sheet := ids.NewV7()
	reader, _ := readerOver(map[ids.UUID]storedFile{
		// Stored as the sniffer stores every xlsx.
		sheet: {contentType: "application/zip", raw: salarySheet(t)},
	})
	row := capture.PendingThread{
		ID: ids.NewV7(), Subject: "Invoice", Body: "please find attached the invoice",
		Documents: []capture.ThreadDocument{{AttachmentID: sheet, Filename: "Cost+.xlsx", ByteSize: int64(len(salarySheet(t)))}},
	}

	read, err := reader.read(context.Background(), row, newDocumentPass())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := read.Documents[0]; got.Unreadable || !strings.Contains(got.Excerpt, "Minh Tran | Senior developer | 2450") {
		t.Fatalf("document = %+v, want the salary rows", got)
	}
	prompt := confidentialityRequest(read).Messages[0].Content
	excerptAt := strings.Index(prompt, "Minh Tran")
	if excerptAt < 0 || excerptAt > strings.Index(prompt, "Return JSON:") {
		t.Errorf("the excerpt is not inside the fenced thread:\n%s", prompt)
	}
}

func TestADocumentTheVerdictCannotReadIsMarkedNotDropped(t *testing.T) {
	legacy, oversized, grown := ids.NewV7(), ids.NewV7(), ids.NewV7()
	reader, fake := readerOver(map[ids.UUID]storedFile{
		legacy: {contentType: "application/octet-stream", raw: []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}},
		grown:  {contentType: "text/plain", raw: bytes.Repeat([]byte("a"), confidentialityDocumentBytes+1)},
	})
	row := capture.PendingThread{Documents: []capture.ThreadDocument{
		{AttachmentID: legacy, Filename: "old.xls", ByteSize: 8},
		{AttachmentID: oversized, Filename: "huge.pdf", ByteSize: confidentialityDocumentBytes + 1},
		{AttachmentID: grown, Filename: "grew.csv", ByteSize: 10},
	}}

	read, err := reader.read(context.Background(), row, newDocumentPass())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Documents) != 3 {
		t.Fatalf("documents = %+v, want all three kept with a mark", read.Documents)
	}
	for _, doc := range read.Documents {
		if !doc.Unreadable || doc.Excerpt != "" {
			t.Errorf("%s = %+v, want marked unreadable with no excerpt", doc.Filename, doc)
		}
	}
	for _, id := range fake.fetched {
		if id == oversized {
			t.Error("a file over the size cap was fetched; its stated size alone marks it")
		}
	}
}

func TestAFileWithheldSinceTheClaimIsMarkedNotDropped(t *testing.T) {
	file := ids.NewV7()
	reader, _ := readerOver(map[ids.UUID]storedFile{file: {err: activities.ErrBytesWithheld}})
	read, err := reader.read(context.Background(),
		capture.PendingThread{Documents: []capture.ThreadDocument{{AttachmentID: file, Filename: "Cost+.xlsx"}}},
		newDocumentPass())
	if err != nil || len(read.Documents) != 1 || !read.Documents[0].Unreadable {
		t.Errorf("read = %+v, %v; want the withheld file kept and marked unreadable", read.Documents, err)
	}
}

func TestAFileTheStoreNoLongerServesIsMarkedNotDropped(t *testing.T) {
	reader, _ := readerOver(map[ids.UUID]storedFile{})
	read, err := reader.read(context.Background(),
		capture.PendingThread{Documents: []capture.ThreadDocument{{AttachmentID: ids.NewV7(), Filename: "gone.pdf"}}},
		newDocumentPass())
	if err != nil || len(read.Documents) != 1 || !read.Documents[0].Unreadable {
		t.Errorf("read = %+v, %v; want the missing file kept and marked unreadable", read.Documents, err)
	}
}

func TestAnObjectStoreOutageDefersTheThreadRatherThanJudgingWithoutTheFile(t *testing.T) {
	file := ids.NewV7()
	reader, _ := readerOver(map[ids.UUID]storedFile{file: {err: errors.New("dial tcp: connection refused")}})
	_, err := reader.read(context.Background(),
		capture.PendingThread{Documents: []capture.ThreadDocument{{AttachmentID: file, Filename: "Cost+.xlsx"}}},
		newDocumentPass())
	if !confidentialityDeferred(err) {
		t.Errorf("read = %v, want a deferral: judged without the file, the thread could open", err)
	}
}

func TestAPassThatSpentItsReadBudgetDefersTheRest(t *testing.T) {
	file := ids.NewV7()
	reader, fake := readerOver(map[ids.UUID]storedFile{file: {contentType: "text/csv", raw: []byte("a,b")}})
	_, err := reader.read(context.Background(),
		capture.PendingThread{Documents: []capture.ThreadDocument{{AttachmentID: file, ByteSize: 3}}},
		&documentPass{bytesLeft: 2})
	if !confidentialityDeferred(err) || len(fake.fetched) != 0 {
		t.Errorf("read = %v after %d fetches, want a deferral before fetching", err, len(fake.fetched))
	}
}

func TestACancelledPassDefersTheThread(t *testing.T) {
	file := ids.NewV7()
	reader, _ := readerOver(map[ids.UUID]storedFile{file: {contentType: "text/csv", raw: []byte("a,b")}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := reader.read(ctx, capture.PendingThread{Documents: []capture.ThreadDocument{{AttachmentID: file}}}, newDocumentPass())
	if !confidentialityDeferred(err) {
		t.Errorf("read = %v, want a deferral for a cancelled pass", err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// salaryReadingBrain answers as a model that read the thread would: personnel
// when the prompt states the salary rows, ordinary otherwise. So the verdict
// can only hold the thread if the file's text reached the call.
type salaryReadingBrain struct{ prompts []string }

func (b *salaryReadingBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	prompt := req.Messages[0].Content
	b.prompts = append(b.prompts, prompt)
	asked := fencedIDs(req.System, prompt, "id")
	if len(asked) != 1 {
		return model.Response{}, errors.New("the prompt fenced no thread, or more than one")
	}
	kind := confidentialityOrdinary
	if strings.Contains(prompt, "Minh Tran | Senior developer | 2450") {
		kind = confidentialityPersonnel
	}
	payload, err := json.Marshal(map[string]any{"results": []map[string]any{
		{"id": asked[0], "verdict": kind, "confidence": 0.95},
	}})
	return model.Response{Text: string(payload)}, err
}

func TestAnInvoiceMailCarryingASalarySheetStaysWithItsParticipants(t *testing.T) {
	e := integration.Setup(t)
	sheet, err := os.ReadFile("../platform/doctext/testdata/salaries.xlsx")
	if err != nil {
		t.Fatalf("reading the salary workbook fixture: %v", err)
	}
	blob := blobstore.NewMemory()
	mail := seedHeldThreadMail(t, e, "thread-cost-plus", "billing@vendor.example", "Development Service - March")
	// Stored as the sniffer stores every xlsx: application/zip.
	file := captureMailDocument(t, e, blob, mail, "2026 03 Cost+ VER 2.xlsx", "application/zip", sheet)
	thread := seedThreadQuestion(t, e, "thread-cost-plus", mail)

	brain := &salaryReadingBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}

	if len(brain.prompts) != 1 {
		t.Fatalf("the model was asked %d times, want once", len(brain.prompts))
	}
	if got := threadStatus(t, e, thread); got != capture.VerdictHeld {
		t.Fatalf("thread status = %q, want held: the salary sheet never reached the verdict", got)
	}
	if got := activityAudience(t, e, mail); got != "participants" {
		t.Fatalf("the mail is %q, want participants", got)
	}

	files := activities.NewStore(InstallationDB(e.Pool)).WithBlobstore(blob)
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	switch _, body, err := files.OpenAttachment(colleague, file); {
	case err == nil:
		t.Errorf("a colleague downloaded the salary sheet (close: %v), want not found", body.Close())
	case !errors.Is(err, apperrors.ErrNotFound):
		t.Errorf("a colleague downloading the salary sheet got %v, want not found", err)
	}
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	_, body, err := files.OpenAttachment(owner, file)
	if err != nil {
		t.Fatalf("the mailbox owner lost their own file: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("closing the owner's download: %v", err)
	}
}

func TestAnInvoiceMailWithoutTheSheetIsStillOpened(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	mail := seedHeldThreadMail(t, e, "thread-invoice", "billing@vendor.example", "Invoice 1042")
	captureMailDocument(t, e, blob, mail, "INV 1042.csv", "text/plain", []byte("Invoice,1042\nTotal,24800 EUR\n"))
	thread := seedThreadQuestion(t, e, "thread-invoice", mail)

	brain := &salaryReadingBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}
	if got := threadStatus(t, e, thread); got != capture.VerdictCleared {
		t.Fatalf("thread status = %q, want cleared: an ordinary invoice must still open", got)
	}
	if len(brain.prompts) != 1 || !strings.Contains(brain.prompts[0], "Total,24800 EUR") {
		t.Errorf("the invoice's own text did not reach the call: %q", brain.prompts)
	}
}

func TestASalarySheetUnderANeutralNameIsStillRead(t *testing.T) {
	e := integration.Setup(t)
	sheet, err := os.ReadFile("../platform/doctext/testdata/salaries.xlsx")
	if err != nil {
		t.Fatalf("reading the salary workbook fixture: %v", err)
	}
	blob := blobstore.NewMemory()
	mail := seedHeldThreadMail(t, e, "thread-neutral-name", "billing@vendor.example", "Report")
	captureMailDocument(t, e, blob, mail, "data.bin", "application/zip", sheet)
	thread := seedThreadQuestion(t, e, "thread-neutral-name", mail)

	engine := NewConfidentialityVerdictEngine(e.Pool, &salaryReadingBrain{}, slog.Default()).WithObjectStore(blob)
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}
	if got := threadStatus(t, e, thread); got != capture.VerdictHeld {
		t.Fatalf("thread status = %q, want held: the sender's choice of name hid the sheet", got)
	}
}

func TestAThreadWithDocumentsPastTheCapDoesNotOpen(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	mail := seedHeldThreadMail(t, e, "thread-many-files", "billing@vendor.example", "Invoices")
	for i := range capture.MaxThreadDocuments + 1 {
		name := "INV " + string(rune('A'+i)) + ".csv"
		captureMailDocument(t, e, blob, mail, name, "text/plain", []byte("Invoice,"+name+"\nTotal,100 EUR\n"))
	}
	thread := seedThreadQuestion(t, e, "thread-many-files", mail)

	brain := &salaryReadingBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}
	if len(brain.prompts) != 1 || !strings.Contains(brain.prompts[0], "1 more attached documents were not read") {
		t.Fatalf("the call was not told a document went unread: %q", brain.prompts)
	}
	if got := threadStatus(t, e, thread); got != capture.VerdictUnsure {
		t.Fatalf("thread status = %q, want unsure: an ordinary answer about files it never read must not open", got)
	}
	if got := activityAudience(t, e, mail); got != "participants" {
		t.Fatalf("the mail is %q, want participants", got)
	}
}

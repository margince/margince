// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"

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
	mail := seedHeldThreadMail(t, e, "thread-many-files", "billing@vendor.example", "Reports")
	for i := range capture.MaxThreadDocuments + 1 {
		name := "Report " + string(rune('A'+i)) + ".csv"
		captureMailDocument(t, e, blob, mail, name, "text/plain", []byte("Report,"+name+"\nTotal,100 EUR\n"))
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

// alwaysOrdinaryBrain is the worst model there is for this check: it calls
// every thread routine company business, with confidence.
type alwaysOrdinaryBrain struct{ asked int }

func (b *alwaysOrdinaryBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	b.asked++
	asked := fencedIDs(req.System, req.Messages[0].Content, "id")
	if len(asked) != 1 {
		return model.Response{}, errors.New("the prompt fenced no thread, or more than one")
	}
	payload, err := json.Marshal(map[string]any{"results": []map[string]any{
		{"id": asked[0], "verdict": confidentialityOrdinary, "confidence": 0.99},
	}})
	return model.Response{Text: string(payload)}, err
}

// staffInvoicePDF is an invoice shaped like one that bills a customer for one
// named engineer at a monthly rate, which is what that engineer costs.
func staffInvoicePDF(t *testing.T) []byte {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	pdf.MultiCell(0, 5, `Invoice
BILL TO
Northwind GmbH
INVOICE 1043 SAP
SERVICE PERIOD March 2026
ACTIVITY QTY RATE AMOUNT
Development Services Senior PHP Quang Le
Van
Northwind SAP ERP Support
1 6,212.00 6,212.00
BALANCE DUE USD 6,212.00`, "", "L", false)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("rendering the invoice fixture: %v", err)
	}
	return buf.Bytes()
}

// scannedPDF is a page with no text layer, as a scan or a print to an image is.
func scannedPDF(t *testing.T) []byte {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.Rect(20, 20, 120, 60, "F")
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("rendering the scan fixture: %v", err)
	}
	return buf.Bytes()
}

// curriculumVitaePDF is one candidate's CV under a file name that says nothing.
// The prose before its headings fills more than the prompt's excerpt.
func curriculumVitaePDF(t *testing.T) []byte {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	pdf.MultiCell(0, 5, `Anna Berger
Senior Project Manager
`+strings.Repeat("I lead delivery teams through rollouts and keep customers informed. ", 30)+`
Work Experience
2019 - today  Delivery lead, Northwind GmbH
Education
2012 - 2016  M.Sc. Computer Science`, "", "L", false)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("rendering the CV fixture: %v", err)
	}
	return buf.Bytes()
}

func TestAStaffInvoiceAndCostPlusSheetStayPrivateWhateverTheModelSays(t *testing.T) {
	costPlus, err := os.ReadFile("../platform/doctext/testdata/costplus.xlsx")
	if err != nil {
		t.Fatalf("reading the cost-plus fixture: %v", err)
	}
	for name, files := range map[string]map[string][2]string{
		"the invoice alone":         {"2026.03 Northwind INV 1043 SAP.pdf": {"application/pdf", string(staffInvoicePDF(t))}},
		"the cost-plus sheet alone": {"2026 03 Northwind Cost+ VER 2.xlsx": {"application/zip", string(costPlus)}},
		"a scanned invoice":         {"2023.09 NORTHWIND INV 3556.pdf": {"application/pdf", string(scannedPDF(t))}},
		"a CV under a plain name":   {"anna-berger.pdf": {"application/pdf", string(curriculumVitaePDF(t))}},
		"a photographed CV":         {"CV_Anna_Berger.jpg": {"image/jpeg", "\xff\xd8\xff\xe0 not a real photo"}},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			blob := blobstore.NewMemory()
			mail := seedHeldThreadMail(t, e, "thread-staff-invoice", "finance@gradion.example", "[Northwind] Development Service - March 2026")
			for filename, file := range files {
				captureMailDocument(t, e, blob, mail, filename, file[0], []byte(file[1]))
			}
			thread := seedThreadQuestion(t, e, "thread-staff-invoice", mail)

			brain := &alwaysOrdinaryBrain{}
			engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
			if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
				t.Fatalf("confidentiality pass: %v", err)
			}
			if got := threadStatus(t, e, thread); got != capture.VerdictHeld {
				t.Fatalf("thread status = %q, want held: a named engineer's rate opened to the workspace", got)
			}
			if got := activityAudienceReason(t, e, mail); got != confidentialityPersonnel {
				t.Errorf("audience reason = %q, want personnel", got)
			}
			if brain.asked != 0 {
				t.Errorf("the model was asked %d times; the rule decides this thread without it", brain.asked)
			}
		})
	}
}

// A scan under a plain name is still asked of the model, which keeps its kind
// on the thread. A confident "ordinary" does not open it: nobody read the file.
func TestAScanUnderAPlainNameIsAskedButNeverOpened(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	mail := seedHeldThreadMail(t, e, "thread-scanned-map", "office@gradion.example", "Directions to the venue")
	captureMailDocument(t, e, blob, mail, "venue map.pdf", "application/pdf", scannedPDF(t))
	thread := seedThreadQuestion(t, e, "thread-scanned-map", mail)

	brain := &alwaysOrdinaryBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}
	if brain.asked != 1 {
		t.Fatalf("the model was asked %d times, want once: only a file named as private skips it", brain.asked)
	}
	if got := threadStatus(t, e, thread); got != capture.VerdictUnsure {
		t.Errorf("thread status = %q, want unsure: a file nobody read keeps the thread held", got)
	}
}

func TestAnEarlierReplyCarryingASalarySheetIsNotOpenedWithItsThread(t *testing.T) {
	e := integration.Setup(t)
	sheet, err := os.ReadFile("../platform/doctext/testdata/salaries.xlsx")
	if err != nil {
		t.Fatalf("reading the salary workbook fixture: %v", err)
	}
	blob := blobstore.NewMemory()
	first := seedHeldThreadMail(t, e, "thread-reply-sheet", "billing@vendor.example", "Invoice 1042")
	thread := seedThreadQuestion(t, e, "thread-reply-sheet", first)
	reply := seedHeldThreadMail(t, e, "thread-reply-sheet", "billing@vendor.example", "Re: Invoice 1042")
	captureMailDocument(t, e, blob, reply, "Cost+.xlsx", "application/zip", sheet)

	brain := &salaryReadingBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
	ws := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := engine.RunWorkspace(ws, 0); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if got := activityAudience(t, e, first); got != "workspace" {
		t.Fatalf("the plain first message is %q, want workspace", got)
	}
	if got := activityAudience(t, e, reply); got != "participants" {
		t.Fatalf("the reply with the sheet is %q, want participants: the verdict never read its file", got)
	}
	if got := threadStatus(t, e, thread); got != capture.VerdictPending {
		t.Fatalf("thread status = %q, want pending at the reply", got)
	}

	e.WsExec(t, `UPDATE capture_thread_verdict SET next_attempt_at = now() WHERE id = $1`, thread)
	if err := engine.RunWorkspace(ws, 0); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if got := activityAudience(t, e, reply); got != "participants" {
		t.Fatalf("after its own verdict the reply is %q, want participants", got)
	}
	if got := threadStatus(t, e, thread); got != capture.VerdictHeld {
		t.Fatalf("thread status = %q, want held once the sheet is read", got)
	}
}

func TestAnUnreadableImageCannotBeOpenedByAConfidentAnswer(t *testing.T) {
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, file := range []struct {
		name, media string
		raw         []byte
	}{
		{"image001.png", "image/png", picture.Bytes()},
		{"diagram.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>Demo picture</text></svg>`)},
	} {
		t.Run(file.name, func(t *testing.T) {
			e := integration.Setup(t)
			blob := blobstore.NewMemory()
			mail := seedHeldThreadMail(t, e, "thread-unreadable-image", "buyer@example.test", "Documents")
			captureMailDocument(t, e, blob, mail, file.name, file.media, file.raw)
			thread := seedThreadQuestion(t, e, "thread-unreadable-image", mail)
			brain := &alwaysOrdinaryBrain{}
			engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()).WithObjectStore(blob)
			if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
				t.Fatal(err)
			}
			if brain.asked != 1 || threadStatus(t, e, thread) != capture.VerdictUnsure || activityAudience(t, e, mail) != "participants" {
				t.Fatalf("unreadable image: calls=%d status=%s audience=%s", brain.asked, threadStatus(t, e, thread), activityAudience(t, e, mail))
			}
		})
	}
}

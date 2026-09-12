// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One reading, from claiming the run record to closing it — the file two
// reviews found three defects in, none of which any test could see because the
// orchestration had none of its own.
//
// The store is a double rather than the real one on purpose: what is under test
// is which outcome a reading reaches and which claim it writes under, and a real
// Postgres would only add a way for those assertions to fail for another reason.
// The store's OWN behaviour (the CAS, the lease, the in-flight index) is proven
// against a real database in the integration lane.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// fakeReadStore records what a reading did to the run record.
type fakeReadStore struct {
	claimedAt time.Time
	beginErr  error
	openErr   error
	meta      crmcontracts.Attachment
	body      string

	outcome  *activities.ExtractionReadOutcome
	released bool
	finishes int
}

func (f *fakeReadStore) BeginExtractionRead(
	context.Context, ids.UUID, time.Duration,
) (activities.ExtractionRead, error) {
	if f.beginErr != nil {
		return activities.ExtractionRead{}, f.beginErr
	}
	at := f.claimedAt
	return activities.ExtractionRead{Status: activities.ExtractionReadRunning, StartedAt: &at}, nil
}

func (f *fakeReadStore) FinishExtractionRead(
	_ context.Context, _ ids.UUID, outcome activities.ExtractionReadOutcome,
) error {
	f.finishes++
	f.outcome = &outcome
	return nil
}

func (f *fakeReadStore) ReleaseExtractionRead(context.Context, ids.UUID, time.Time) error {
	f.released = true
	return nil
}

func (f *fakeReadStore) OpenAttachment(
	context.Context, ids.UUID,
) (crmcontracts.Attachment, io.ReadCloser, error) {
	if f.openErr != nil {
		return crmcontracts.Attachment{}, nil, f.openErr
	}
	return f.meta, io.NopCloser(strings.NewReader(f.body)), nil
}

func textAttachment(mime string) crmcontracts.Attachment {
	return crmcontracts.Attachment{Filename: "order-form.txt", ContentType: &mime}
}

// runReading drives one reading and answers what the store recorded.
func runReading(t *testing.T, store *fakeReadStore, brain documentCompleter) error {
	t.Helper()
	if store.claimedAt.IsZero() {
		store.claimedAt = time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	}
	d := NewDocumentExtractor(nil, brain, discardLogger())
	return d.Read(t.Context(), store, ids.NewV7(), ids.NewV7())
}

// scriptedBrain answers with one reply and declares the carriage it is given.
type scriptedBrain struct {
	*ai.FakeClient
	carriage []string
}

func (s scriptedBrain) AttachmentMIMEs() []string { return s.carriage }

func groundedDocumentReply() string {
	return allFour(
		statedField("name", "Order form", "ORDER FORM"),
		statedField(modelFieldAmount, "148500.00", "Contract value: EUR 148,500.00"),
		statedField("currency", "EUR", "Contract value: EUR 148,500.00"),
		"")
}

// The ordinary case: a text document, four fields asked for, three grounded.
func TestAReadingOfATextDocumentStoresWhatItGrounded(t *testing.T) {
	store := &fakeReadStore{meta: textAttachment("text/plain"), body: uatDocument}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(groundedDocumentReply()), carriage: ai.DocumentMIMEs()}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome == nil || store.outcome.Status != activities.ExtractionReadDone {
		t.Fatalf("outcome = %+v, want done", store.outcome)
	}
	if store.outcome.Detail != "" {
		t.Errorf("a reading that grounded fields explained itself anyway: %q", store.outcome.Detail)
	}
	grounded := 0
	for _, f := range store.outcome.Fields {
		if !f.Omitted {
			grounded++
		}
	}
	if grounded != 3 {
		t.Errorf("grounded %d fields, want 3", grounded)
	}
	// Every write is scoped to the claim this attempt holds.
	if !store.outcome.ClaimedAt.Equal(store.claimedAt) {
		t.Errorf("finished under claim %v, want %v", store.outcome.ClaimedAt, store.claimedAt)
	}
}

// Read it, and it states none of them. A CORRECT answer that must explain
// itself, or it cannot be told from a broken one.
func TestADocumentStatingNoneOfThemIsDoneWithAReason(t *testing.T) {
	store := &fakeReadStore{meta: textAttachment("text/plain"), body: uatDocument}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(allFour()), carriage: ai.DocumentMIMEs()}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadDone {
		t.Fatalf("status = %q, want done — a document that states none of them is not a failure", store.outcome.Status)
	}
	if store.outcome.Detail == "" {
		t.Error("an empty reading did not say why, and reads as a broken feature")
	}
}

// COULD NOT read it is a different answer, and `done` is what the panel renders
// as "it states none of them" — so an unreadable file must not reach it.
func TestADocumentThisBindingCannotCarryFailsRatherThanReadingEmpty(t *testing.T) {
	// A zip is neither text this build can read nor a type any adapter carries.
	// image/tiff would NOT do here: it matches the `image/*` declaration, so it
	// takes the bytes lane and the vendor is the one that refuses it.
	store := &fakeReadStore{meta: textAttachment("application/zip"), body: "PK\x03\x04"}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient(), carriage: ai.DocumentMIMEs()}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadFailed {
		t.Fatalf("status = %q, want failed — it was never read", store.outcome.Status)
	}
	if !strings.Contains(store.outcome.Detail, "application/zip") {
		t.Errorf("detail = %q, want it to name the type nothing can read", store.outcome.Detail)
	}
	// And no model call was made: the lane is chosen from the declaration.
	if calls := brain.Calls(); len(calls) != 0 {
		t.Errorf("made %d model call(s) for a type the binding cannot carry, want 0", len(calls))
	}
}

// A binding that carries the type gets the BYTES, not the text.
func TestACarriedDocumentGoesAsAnInputPart(t *testing.T) {
	store := &fakeReadStore{meta: textAttachment("application/pdf"), body: "%PDF-1.4 body"}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(allFour()), carriage: ai.DocumentMIMEs()}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	calls := brain.Calls()
	if len(calls) == 0 {
		t.Fatal("no model call was made for a carried document")
	}
	if !strings.Contains(string(calls[0].Payload), "application/pdf") {
		t.Errorf("the document did not ride as an input part:\n%s", calls[0].Payload)
	}
}

// A reply this site may not act on fails the READING, not the job: retrying
// would ask the same question of the same document and get the same answer.
func TestAnUnusableReplyFailsTheReadingWithoutRetrying(t *testing.T) {
	store := &fakeReadStore{meta: textAttachment("text/plain"), body: uatDocument}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().Script(`{"fields":[]}`), carriage: ai.DocumentMIMEs()}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read returned an error, so the job will retry a reading that cannot change: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadFailed {
		t.Fatalf("status = %q, want failed", store.outcome.Status)
	}
	if store.released {
		t.Error("an unusable reply released the reading, which invites an identical retry")
	}
}

// A transient fault is the JOB's to retry — and the reading goes back with it.
// Without the release the row stays running, the retry declines its own claim,
// and the reading is stranded live with nothing coming for it.
func TestARetryableFaultReleasesTheReadingBeforeHandingBackTheJob(t *testing.T) {
	boom := errors.New("object store unreachable")
	store := &fakeReadStore{meta: textAttachment("text/plain"), openErr: boom}

	err := runReading(t, store, scriptedBrain{FakeClient: ai.NewFakeClient(), carriage: ai.DocumentMIMEs()})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the transient fault so the job retries", err)
	}
	if !store.released {
		t.Error("the reading was not released, so the retry will find its own claim held")
	}
	if store.finishes != 0 {
		t.Error("a transient fault closed the reading, turning a blip into a permanent verdict")
	}
}

// A document that is gone is terminal, and says so in words a rep can act on
// rather than a driver string.
func TestAVanishedDocumentIsTerminalAndReadable(t *testing.T) {
	store := &fakeReadStore{meta: textAttachment("text/plain"), openErr: apperrors.ErrNotFound}

	if err := runReading(t, store, scriptedBrain{FakeClient: ai.NewFakeClient(), carriage: ai.DocumentMIMEs()}); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadFailed {
		t.Fatalf("status = %q, want failed", store.outcome.Status)
	}
	if strings.Contains(store.outcome.Detail, "not found") {
		t.Errorf("detail = %q, want a rep-readable reason rather than the sentinel", store.outcome.Detail)
	}
}

// A claim this worker no longer holds is not its reading to work.
func TestAReadingItCouldNotClaimIsNotWorked(t *testing.T) {
	store := &fakeReadStore{beginErr: apperrors.ErrConflict}

	err := runReading(t, store, scriptedBrain{FakeClient: ai.NewFakeClient(), carriage: ai.DocumentMIMEs()})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("err = %v, want the claim conflict", err)
	}
	if store.finishes != 0 || store.released {
		t.Error("a reading nobody claimed was written to anyway")
	}
}

// The binding refusing a type it declared is a CONFIGURATION fault, not this
// document's — so it is refused rather than retried forever.
func TestABindingRefusingWhatItDeclaredFailsTheReading(t *testing.T) {
	store := &fakeReadStore{meta: textAttachment("application/pdf"), body: "%PDF"}
	// Declares PDF; the fake carries nothing, so the wire refuses it.
	brain := scriptedBrain{FakeClient: ai.NewFakeClient().CarryingNothing(), carriage: ai.DocumentMIMEs()}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadFailed {
		t.Fatalf("status = %q, want failed", store.outcome.Status)
	}
}

var _ = model.ErrAttachmentUnsupported

// carriesImagesOnly is a wire with an image part and no document part — the
// shape the extraction lane exists for. Named rather than spelled at each case.
//
// WHICH providers declare this is the ai module's question and is pinned there
// (carriage_test.go); restating the list here would be a copy free to go stale.
var carriesImagesOnly = []string{"image/*"}

// invoicePDFBytes builds the document this feature reads: an order confirmation
// whose text the model is expected to quote back.
func invoicePDFBytes(t *testing.T, lines ...string) string {
	t.Helper()
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	for _, line := range lines {
		doc.Cell(0, 8, line)
		doc.Ln(8)
	}
	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		t.Fatalf("building the fixture PDF: %v", err)
	}
	return out.String()
}

func pdfAttachment() crmcontracts.Attachment {
	mime := documentPDFMIME
	return crmcontracts.Attachment{Filename: "order-confirmation.pdf", ContentType: &mime}
}

// The case this lane exists for, at the boundary an operator meets it: a binding on a
// wire with no document part still reads the PDF, because the PDF stopped being
// a PDF before the router saw it.
func TestAPDFOnAWireWithNoDocumentLaneIsReadAsItsOwnText(t *testing.T) {
	store := &fakeReadStore{
		meta: pdfAttachment(),
		body: invoicePDFBytes(t, "ORDER FORM", "Contract value: EUR 148,500.00"),
	}
	brain := scriptedBrain{
		FakeClient: ai.NewFakeClient().Script(groundedDocumentReply()),
		carriage:   carriesImagesOnly,
	}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadDone {
		t.Fatalf("outcome = %+v, want done — this is the case the whole lane exists for", store.outcome)
	}
	calls := brain.Calls()
	if len(calls) == 0 {
		t.Fatal("no model call was made, so the document was never read")
	}
	payload := string(calls[0].Payload)
	// The document's own words reached the model...
	if !strings.Contains(payload, "148,500.00") {
		t.Errorf("the extracted text did not reach the model:\n%s", payload)
	}
	// ...and its BYTES did not. A wire with no document part must not be handed
	// one, which is the failure this lane replaces rather than hides.
	//
	// Asserted on the bytes lane's own sentence rather than on the media type:
	// the media type appears legitimately in the extracted reading's provenance
	// line, so searching for it would fail on a correct payload.
	if strings.Contains(payload, attachedSentence) {
		t.Errorf("the PDF rode as an input part on a wire that has none:\n%s", payload)
	}
}

// attachedSentence is the line documentExtractRequest writes ONLY on the bytes
// lane, so its presence is what separates the two lanes in a recorded call.
const attachedSentence = "The document itself is attached to this message."

// The seam the production lane is actually wired through, against a REAL router.
//
// Every case above drives a double, which proves what the lane does with an
// answer but nothing about where the answer comes from — and the wiring is the
// half that decides whether an operator's `input:` reaches this decision at all.
// A routerBrain that returned a constant would pass every test above.
func TestTheDocumentLaneAsksTheRouterItIsBoundTo(t *testing.T) {
	const pdf = documentPDFMIME
	brainFor := func(t *testing.T, routing string) routerBrain {
		t.Helper()
		cfg, err := ai.ParseRouting([]byte(routing))
		if err != nil {
			t.Fatalf("ParseRouting: %v", err)
		}
		router, err := ai.NewRouter(cfg, nil, ai.DefaultMonthlyTokens, nil, false, nil)
		if err != nil {
			t.Fatalf("NewRouter: %v", err)
		}
		return routerBrain{router: router, task: ai.TaskDocumentExtract}
	}

	t.Run("an undeclared binding carries the document natively", func(t *testing.T) {
		brain := brainFor(t, `profile: eu_hosted
tiers:
  local_small: {provider: fake, model: m}
  cheap_cloud: {provider: fake, model: m}
  premium: {provider: fake, model: m}
  frontier: {provider: fake, model: m}
embeddings: {provider: fake, model: m-embed, dimensions: 8}
`)
		if !model.CarriesMIME(brain.AttachmentMIMEs(), pdf) {
			t.Errorf("an undeclared binding must carry a PDF, got %v", brain.AttachmentMIMEs())
		}
	})

	t.Run("`input:` on the bound tiers is what the lane sees", func(t *testing.T) {
		brain := brainFor(t, `profile: eu_hosted
tiers:
  local_small: {provider: fake, model: m, input: [text, image]}
  cheap_cloud: {provider: fake, model: m, input: [text, image]}
  premium: {provider: fake, model: m, input: [text, image]}
  frontier: {provider: fake, model: m, input: [text, image]}
embeddings: {provider: fake, model: m-embed, dimensions: 8}
`)
		if model.CarriesMIME(brain.AttachmentMIMEs(), pdf) {
			t.Errorf("a narrowed binding must not carry a PDF, got %v", brain.AttachmentMIMEs())
		}
	})
}

// A model told it is reading extracted text is told so IN the prompt, because
// the alternative is a reading that reports a layout it was never shown.
func TestAnExtractedReadingTellsTheModelWhatItIsLookingAt(t *testing.T) {
	store := &fakeReadStore{
		meta: pdfAttachment(),
		body: invoicePDFBytes(t, "Contract value: EUR 148,500.00"),
	}
	brain := scriptedBrain{
		FakeClient: ai.NewFakeClient().Script(groundedDocumentReply()),
		carriage:   carriesImagesOnly,
	}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	payload := string(brain.Calls()[0].Payload)
	if !strings.Contains(payload, "extracted by this system") {
		t.Errorf("the prompt does not say the text is a derivation, so a quote checked "+
			"against it reads as a quote checked against the document:\n%s", payload)
	}
}

// The ORDER of the lanes. A vendor that opens the document itself sees its
// layout where extraction sees only characters, so native carriage wins and
// extraction is what a wire without the lane falls back to — never an
// improvement applied to one that has it.
func TestNativeCarriageIsPreferredToExtraction(t *testing.T) {
	store := &fakeReadStore{
		meta: pdfAttachment(),
		body: invoicePDFBytes(t, "Contract value: EUR 148,500.00"),
	}
	brain := scriptedBrain{
		FakeClient: ai.NewFakeClient().Script(groundedDocumentReply()),
		carriage:   ai.DocumentMIMEs(), // carries application/pdf
	}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	payload := string(brain.Calls()[0].Payload)
	if !strings.Contains(payload, attachedSentence) {
		t.Errorf("a binding that carries a PDF was handed extracted text instead:\n%s", payload)
	}
	if strings.Contains(payload, "extracted by this system") {
		t.Error("the extraction lane ran for a binding that can open the document itself")
	}
}

// A scan is a real document somebody can re-send in another form, and saying so
// is a different answer from "this file is broken". The refusal has to be the
// one a rep can act on.
func TestAScannedPDFSaysItHoldsNoTextRatherThanFailingVaguely(t *testing.T) {
	blank := fpdf.New("P", "mm", "A4", "")
	blank.AddPage()
	var out bytes.Buffer
	if err := blank.Output(&out); err != nil {
		t.Fatalf("building the fixture PDF: %v", err)
	}
	store := &fakeReadStore{meta: pdfAttachment(), body: out.String()}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient(), carriage: carriesImagesOnly}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadFailed {
		t.Fatalf("status = %q, want failed", store.outcome.Status)
	}
	if !strings.Contains(store.outcome.Detail, "holds no text") {
		t.Errorf("detail = %q, want it to name the missing text layer — "+
			"an operator told only that a PDF could not be read goes to their model config",
			store.outcome.Detail)
	}
	if calls := brain.Calls(); len(calls) != 0 {
		t.Errorf("made %d model call(s) for a document with nothing to read, want 0", len(calls))
	}
}

// The bound maxDocumentBytes' own comment states — "refused as a reading rather
// than truncated, because nothing on the panel would say which half it saw" —
// applies to the extracted lane too. The text lane already honours it; reading
// the first 60,000 characters of a contract and reporting `done` is the outcome
// it exists to refuse.
func TestAPDFLongerThanOneReadingIsRefusedRatherThanTruncated(t *testing.T) {
	lines := make([]string, 0, 4000)
	for range 4000 {
		lines = append(lines,
			"Contract value: EUR 148,500.00 for the packaging line retrofit and its commissioning")
	}
	store := &fakeReadStore{meta: pdfAttachment(), body: invoicePDFBytes(t, lines...)}
	brain := scriptedBrain{
		FakeClient: ai.NewFakeClient().Script(groundedDocumentReply()),
		carriage:   carriesImagesOnly,
	}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadFailed {
		t.Fatalf("status = %q, want failed — a reading of part of a contract cannot say which part",
			store.outcome.Status)
	}
	if calls := brain.Calls(); len(calls) != 0 {
		t.Errorf("made %d model call(s) with a truncated document, want 0", len(calls))
	}
}

// What ingress stored is a HEADER, and a header carries parameters. The same
// document must not be read or refused depending on which client uploaded it.
func TestAContentTypeCarryingParametersStillNamesItsLane(t *testing.T) {
	withParams := "application/pdf; charset=binary"
	store := &fakeReadStore{
		meta: crmcontracts.Attachment{Filename: "order.pdf", ContentType: &withParams},
		body: invoicePDFBytes(t, "ORDER FORM", "Contract value: EUR 148,500.00"),
	}
	brain := scriptedBrain{
		FakeClient: ai.NewFakeClient().Script(groundedDocumentReply()),
		carriage:   carriesImagesOnly,
	}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadDone {
		t.Fatalf("outcome = %+v, want done — the parameter decided the lane", store.outcome)
	}
}

// A file that claims to be a PDF and is not closes the reading rather than
// taking the worker down with it.
func TestAPDFThatCannotBeOpenedClosesTheReading(t *testing.T) {
	store := &fakeReadStore{meta: pdfAttachment(), body: "%PDF-1.4\nthis is not one"}
	brain := scriptedBrain{FakeClient: ai.NewFakeClient(), carriage: carriesImagesOnly}

	if err := runReading(t, store, brain); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if store.outcome.Status != activities.ExtractionReadFailed {
		t.Fatalf("status = %q, want failed", store.outcome.Status)
	}
	if calls := brain.Calls(); len(calls) != 0 {
		t.Errorf("made %d model call(s) for a document that could not be opened, want 0", len(calls))
	}
}

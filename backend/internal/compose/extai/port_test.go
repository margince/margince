// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extai

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/pdftext"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

type extAICalls struct{ rows []ai.Call }

func (s *extAICalls) Record(_ context.Context, attempts []ai.Call) error {
	s.rows = append(s.rows, attempts...)
	return nil
}

func (s *extAICalls) EnsureConfig(context.Context, ai.ConfigSnapshot) error { return nil }

const awsKey = "AKIAIOSFODNN7EXAMPLE"

var aiUnitTasks = []extension.AITask{
	{Name: "summarise", Prompt: "Summarise the text."},
	{Name: "invoice_total", Prompt: "Return the invoice total.", Documents: true, Schema: `{"type":"object","required":["total"],"properties":{"total":{"type":"number"}}}`},
}

// routerLane is the extension_task lane over a real router, as the model path
// builds it.
type routerLane struct{ router *ai.Router }

func (l routerLane) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	resp, _, err := l.router.Complete(ctx, ai.TaskExtensionTask, req)
	return resp, err
}

func (l routerLane) CompleteValidated(ctx context.Context, req model.Request, validate ai.Validator) (model.Response, error) {
	resp, _, err := l.router.CompleteStructured(ctx, ai.TaskExtensionTask, req, validate)
	return resp, err
}

// harness is the books unit's port over a real router and the offline model,
// so the call takes the path a core task takes.
type harness struct {
	lane       Lane
	restricted func(context.Context) (bool, error)
	live       bool
	workspace  ids.UUID
}

func (h *harness) release() { h.live = false }

func (h *harness) AI() Port {
	return Port{
		Lane: h.lane, Restricted: h.restricted, PDF: testPDF, MaxDocumentBytes: 8 << 20, MaxInputChars: 60000,
		Scope: func(ctx context.Context) (context.Context, error) {
			if !h.live {
				return nil, extension.ErrRuntimeExpired
			}
			ctx = principal.WithWorkspaceID(ctx, h.workspace)
			return provenance.WithExtension(ctx, provenance.Extension{Unit: "books", Version: "1.0.0"}), nil
		},
		Unit: "books",
		Declared: func() []extension.Extension {
			return []extension.Extension{
				{Name: "books", AITasks: aiUnitTasks},
				{Name: "other", AITasks: []extension.AITask{{Name: "private_task", Prompt: "Do it."}}},
			}
		},
	}
}

var testPDF = pdftext.New()

func aiRuntime(t *testing.T, cfg ai.RoutingConfig, fake *ai.FakeClient) (*harness, *extAICalls) {
	t.Helper()
	calls := &extAICalls{}
	router, err := ai.NewLocalRouter(cfg, ai.WithFakeClient(fake), ai.WithCallStore(calls), ai.WithoutResultCache(),
		ai.WithRestrictionCheck(ai.NoRestrictionData{}))
	if err != nil {
		t.Fatal(err)
	}
	return &harness{
		lane: routerLane{router}, live: true, workspace: ids.NewV7(),
		restricted: func(context.Context) (bool, error) { return false, nil },
	}, calls
}

func offline() ai.RoutingConfig { return ai.FakeRoutingConfig() }

func noContact() extension.AIInput {
	return extension.AIInput{Text: "hello", NoContact: "a public price list"}
}

func TestAnUndeclaredAITaskIsRefusedBeforeAnyModelCall(t *testing.T) {
	fake := ai.NewFakeClient().Script("x")
	rt, _ := aiRuntime(t, offline(), fake)
	for name, task := range map[string]extension.AITaskName{
		"a name nobody declared":  "nope",
		"another unit's task":     "private_task",
		"the empty name":          "",
		"the shared task's label": "extension_task",
	} {
		if _, err := rt.AI().Run(t.Context(), task, noContact()); !errors.Is(err, extension.ErrAITaskUndeclared) {
			t.Errorf("%s: err = %v, want ErrAITaskUndeclared", name, err)
		}
	}
	if n := len(fake.Calls()); n != 0 {
		t.Fatalf("the model was called %d times for refused tasks", n)
	}
}

func TestTheAICallRowNamesTheExtensionAndTheTask(t *testing.T) {
	fake := ai.NewFakeClient().Script("a summary")
	rt, calls := aiRuntime(t, offline(), fake)

	got, err := rt.AI().Run(t.Context(), "summarise", noContact())
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "a summary" || !got.MachineMade {
		t.Fatalf("result = %+v, want the model's text marked machine-made", got)
	}
	if len(calls.rows) != 1 {
		t.Fatalf("ai_call rows = %d, want 1", len(calls.rows))
	}
	row := calls.rows[0]
	if row.Task != ai.TaskExtensionTask || row.Extension != "books/summarise" {
		t.Fatalf("row task=%q extension=%q, want extension_task and books/summarise", row.Task, row.Extension)
	}
}

func TestTheUnitNameOnTheRowComesFromTheInvocationNotTheHandler(t *testing.T) {
	fake := ai.NewFakeClient().Script("ok")
	rt, calls := aiRuntime(t, offline(), fake)
	ctx := ai.WithExtensionTask(t.Context(), "someone_elses")
	if _, err := rt.AI().Run(ctx, "summarise", noContact()); err != nil {
		t.Fatal(err)
	}
	if got := calls.rows[0].Extension; got != "books/summarise" {
		t.Fatalf("extension = %q, want the declared task of the invoking unit", got)
	}
}

func TestRedactionRunsOnAnExtensionsInput(t *testing.T) {
	fake := ai.NewFakeClient().Script("ok")
	rt, calls := aiRuntime(t, offline(), fake)
	in := extension.AIInput{Text: "key is " + awsKey, NoContact: "a ledger export"}
	if _, err := rt.AI().Run(t.Context(), "summarise", in); err != nil {
		t.Fatal(err)
	}
	sent := fake.Calls()
	if len(sent) != 1 || bytes.Contains(sent[0].Payload, []byte(awsKey)) {
		t.Fatalf("the credential reached the model: %d calls, payload %q", len(sent), sent[0].Payload)
	}
	if row := calls.rows[0]; row.SecretsRemoved != 1 {
		t.Fatalf("row records %d secrets removed, want 1", row.SecretsRemoved)
	}
}

func TestNothingAnExtensionSendsIsKeptInTheCallPayloadStore(t *testing.T) {
	if !ai.NoPayload(ai.TaskExtensionTask) {
		t.Fatal("extension_task must be declared no_payload, so an extension's input is never kept")
	}
}

func TestACallMustNameItsContactsOrSayItHasNone(t *testing.T) {
	fake := ai.NewFakeClient().Script("x")
	rt, _ := aiRuntime(t, offline(), fake)
	for name, in := range map[string]extension.AIInput{
		"neither":          {Text: "hello"},
		"a blank reason":   {Text: "hello", NoContact: "  "},
		"no valid contact": {Text: "hello", Contacts: []string{"not-an-id"}},
	} {
		if _, err := rt.AI().Run(t.Context(), "summarise", in); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if n := len(fake.Calls()); n != 0 {
		t.Fatalf("the model was called %d times for an undeclared input", n)
	}
}

func TestAScannedPDFIsRefusedAndNothingIsSent(t *testing.T) {
	fake := ai.NewFakeClient().Script(`{"total":1}`)
	rt, _ := aiRuntime(t, offline(), fake)
	blank := fpdf.New("P", "mm", "A4", "")
	blank.AddPage()
	var scan bytes.Buffer
	if err := blank.Output(&scan); err != nil {
		t.Fatal(err)
	}
	in := noContact()
	in.Documents = []extension.AIDocument{{Filename: "scan.pdf", ContentType: "application/pdf", Body: scan.Bytes()}}

	_, err := rt.AI().Run(t.Context(), "invoice_total", in)
	if !errors.Is(err, extension.ErrAIScannedDocument) {
		t.Fatalf("err = %v, want ErrAIScannedDocument", err)
	}
	if !strings.Contains(err.Error(), "text layer") {
		t.Errorf("the refusal does not say what to do: %v", err)
	}
	if n := len(fake.Calls()); n != 0 {
		t.Fatalf("the model was called %d times for a scan", n)
	}
}

func TestAPDFWithTextIsReadAsTextAndTheAnswerIsCheckedAgainstTheSchema(t *testing.T) {
	fake := ai.NewFakeClient().Script(`{"total":148500}`)
	rt, _ := aiRuntime(t, offline(), fake)
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	doc.Cell(0, 8, "Contract value: EUR 148,500.00")
	var pdf bytes.Buffer
	if err := doc.Output(&pdf); err != nil {
		t.Fatal(err)
	}
	in := noContact()
	in.Documents = []extension.AIDocument{{Filename: "inv.pdf", ContentType: "application/pdf; charset=binary", Body: pdf.Bytes()}}

	got, err := rt.AI().Run(t.Context(), "invoice_total", in)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.JSON) != `{"total":148500}` {
		t.Fatalf("JSON = %s", got.JSON)
	}
	payload := string(fake.Calls()[0].Payload)
	if !strings.Contains(payload, "148,500.00") || strings.Contains(payload, "%PDF") {
		t.Fatalf("the model must be handed the PDF's text and not its bytes, got %q", payload)
	}
}

func TestAnAnswerOutsideTheSchemaIsNotHandedToTheUnit(t *testing.T) {
	fake := ai.NewFakeClient().Script(`{"total":"lots"}`, `{"total":"lots"}`, `{"total":"lots"}`, `{"total":"lots"}`)
	rt, _ := aiRuntime(t, offline(), fake)
	if _, err := rt.AI().Run(t.Context(), "invoice_total", noContact()); !errors.Is(err, extension.ErrAIUnavailable) {
		t.Fatalf("err = %v, want ErrAIUnavailable", err)
	}
}

func TestDocumentsAreRefusedForATaskThatDoesNotDeclareThem(t *testing.T) {
	rt, _ := aiRuntime(t, offline(), ai.NewFakeClient().Script("x"))
	in := noContact()
	in.Documents = []extension.AIDocument{{ContentType: "text/plain", Body: []byte("hi")}}
	if _, err := rt.AI().Run(t.Context(), "summarise", in); !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	in.Documents = []extension.AIDocument{{ContentType: "text/plain", Body: png}}
	if _, err := rt.AI().Run(t.Context(), "invoice_total", in); !errors.Is(err, extension.ErrAIDocumentType) {
		t.Fatalf("an image: err = %v, want ErrAIDocumentType", err)
	}
}

func TestARoleWithNoAIPathAnswersUnavailable(t *testing.T) {
	rt, _ := aiRuntime(t, offline(), ai.NewFakeClient())
	rt.lane = nil
	if _, err := rt.AI().Run(t.Context(), "summarise", noContact()); !errors.Is(err, extension.ErrAIUnavailable) {
		t.Fatalf("err = %v, want ErrAIUnavailable", err)
	}
}

func TestAReleasedRuntimeRefusesAI(t *testing.T) {
	rt, _ := aiRuntime(t, offline(), ai.NewFakeClient())
	rt.release()
	if _, err := rt.AI().Run(t.Context(), "summarise", noContact()); !errors.Is(err, extension.ErrRuntimeExpired) {
		t.Fatalf("err = %v, want ErrRuntimeExpired", err)
	}
}

func TestPreflightRefusesABadOrRepeatedAITask(t *testing.T) {
	good := extension.Extension{Name: "books", AITasks: aiUnitTasks}
	if err := Preflight(good); err != nil {
		t.Fatalf("a good declaration was refused: %v", err)
	}
	twice := extension.Extension{Name: "books", AITasks: []extension.AITask{aiUnitTasks[0], aiUnitTasks[0]}}
	bad := extension.Extension{Name: "books", AITasks: []extension.AITask{{Name: "Bad", Prompt: "x"}}}
	badSchema := extension.Extension{Name: "books", AITasks: []extension.AITask{{Name: "s", Prompt: "x", Schema: `{"type":"nope"}`}}}
	for name, e := range map[string]extension.Extension{"twice": twice, "bad name": bad, "bad schema": badSchema} {
		if err := Preflight(e); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
}

func blankPDF(t *testing.T, text string) []byte {
	t.Helper()
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	if text != "" {
		doc.SetFont("Helvetica", "", 12)
		doc.Cell(0, 8, text)
	}
	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestADocumentIsReadByItsBytesAndNotByItsLabel(t *testing.T) {
	fake := ai.NewFakeClient().Script(`{"total":1}`)
	rt, _ := aiRuntime(t, offline(), fake)
	in := noContact()

	in.Documents = []extension.AIDocument{{ContentType: "text/plain", Body: blankPDF(t, "")}}
	if _, err := rt.AI().Run(t.Context(), "invoice_total", in); !errors.Is(err, extension.ErrAIScannedDocument) {
		t.Fatalf("a scan labelled text/plain: err = %v, want ErrAIScannedDocument", err)
	}
	in.Documents = []extension.AIDocument{{ContentType: "text/plain", Body: blankPDF(t, "Total EUR 77.00")}}
	if _, err := rt.AI().Run(t.Context(), "invoice_total", in); err != nil {
		t.Fatal(err)
	}
	payload := string(fake.Calls()[0].Payload)
	if !strings.Contains(payload, "77.00") || strings.Contains(payload, "%PDF") {
		t.Fatalf("a PDF labelled text/plain must be read as a PDF, got %q", payload)
	}
}

// The unit's own statement about its contacts is not what the check rests on:
// a restricted contact anywhere in the workspace refuses the call.
func TestARestrictedContactRefusesTheCallWhateverTheUnitSaysAboutItsInput(t *testing.T) {
	fake := ai.NewFakeClient().Script("x")
	rt, _ := aiRuntime(t, offline(), fake)
	rt.restricted = func(context.Context) (bool, error) { return true, nil }
	for name, in := range map[string]extension.AIInput{
		"a reason that says no contact data": {Text: "hello", NoContact: "not contact data"},
		"an unrelated contact id":            {Text: "hello", Contacts: []string{ids.NewV7().String()}},
	} {
		if _, err := rt.AI().Run(t.Context(), "summarise", in); !errors.Is(err, extension.ErrAIUnavailable) {
			t.Errorf("%s: err = %v, want ErrAIUnavailable", name, err)
		}
	}
	if n := len(fake.Calls()); n != 0 {
		t.Fatalf("the model was called %d times", n)
	}
}

func TestAPortOnAnotherPoolFindsNoLane(t *testing.T) {
	rt, _ := aiRuntime(t, ai.FakeRoutingConfig(), ai.NewFakeClient().Script("x"))
	port := rt.AI()
	pool := &pgxpool.Pool{}
	Bind(pool, rt.lane)
	t.Cleanup(func() { Bind(nil, nil) })
	port.Lane, port.Pool = nil, pool
	if _, err := port.Run(t.Context(), "summarise", noContact()); err != nil {
		t.Fatalf("a port on the bound pool found no lane: %v", err)
	}
	port.Pool = &pgxpool.Pool{}
	if _, err := port.Run(t.Context(), "summarise", noContact()); !errors.Is(err, extension.ErrAIUnavailable) {
		t.Fatalf("a port on another pool: err = %v, want ErrAIUnavailable", err)
	}
}

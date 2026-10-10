// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fakeDocuments stands where the attachment store's seam does.
type fakeDocuments struct {
	maxBytes  int64
	attachErr error
	listErr   error
	page      DocumentPage
	attached  []DocumentUpload
	listed    []string
}

func (f *fakeDocuments) Attach(_ context.Context, upload DocumentUpload) (AttachedDocument, error) {
	f.attached = append(f.attached, upload)
	if f.attachErr != nil {
		return AttachedDocument{}, f.attachErr
	}
	return AttachedDocument{
		RecordLink: upload.RecordLink, AttachmentID: ids.NewV7(),
		Filename: upload.Filename, ContentType: upload.ContentType, ByteSize: int64(len(upload.Content)),
		Checksum: "sha256", CapturedBy: "agent:docs", CreatedAt: time.Unix(1_800_000_000, 0).UTC(),
	}, nil
}

func (f *fakeDocuments) List(_ context.Context, record RecordLink, cursor string, limit int) (DocumentPage, error) {
	f.listed = append(f.listed, fmt.Sprintf("%s/%s cursor=%q limit=%d", record.EntityType, record.EntityID, cursor, limit))
	return f.page, f.listErr
}

func (f *fakeDocuments) MaxBytes(context.Context) int64 { return f.maxBytes }

func documentsRegistry(docs Documents) *Registry {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	RegisterDocumentTools(r, docs)
	return r
}

func attachCall(t *testing.T, overrides map[string]any) json.RawMessage {
	t.Helper()
	args := map[string]any{
		"entity_type": "company", "entity_id": ids.NewV7().String(), "filename": "offer.pdf",
		"content_type": "application/pdf", "content_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.7")),
	}
	maps.Copy(args, overrides)
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encoding the call: %v", err)
	}
	return raw
}

func docsCtx() context.Context { return scopedAgentCtx(principal.ScopeRead, principal.ScopeWrite) }

func TestAttachDocumentHandsTheStoreTheDecodedFileAndItsRecord(t *testing.T) {
	docs := &fakeDocuments{}
	company := ids.NewV7()
	out, err := documentsRegistry(docs).Invoke(docsCtx(), "attach_document", attachCall(t, map[string]any{
		"entity_id": company.String(), "filename": "Q3 offer.pdf", "content_type": "application/pdf; charset=binary",
	}))
	if err != nil {
		t.Fatalf("attach_document: %v", err)
	}
	if len(docs.attached) != 1 {
		t.Fatalf("the store was asked %d times, want once", len(docs.attached))
	}
	got := docs.attached[0]
	want := DocumentUpload{
		EntityType: "company", EntityID: company,
		Filename: "Q3 offer.pdf", ContentType: "application/pdf; charset=binary", Content: []byte("%PDF-1.7"),
	}
	if got.RecordLink != want.RecordLink || got.Filename != want.Filename ||
		got.ContentType != want.ContentType || !bytes.Equal(got.Content, want.Content) {
		t.Errorf("the store was handed %+v, want %+v", got, want)
	}
	for _, field := range []string{`"attachment_id"`, `"checksum"`, `"entity_id":"` + company.String()} {
		if !strings.Contains(string(out), field) {
			t.Errorf("the answer carries no %s: %s", field, out)
		}
	}
	if strings.Contains(string(out), "storage_key") {
		t.Errorf("the answer names where the bytes are stored: %s", out)
	}
}

// Every refusal names the argument to change, and none reaches the store.
func TestAttachDocumentRefusesWhatItCannotStore(t *testing.T) {
	cases := map[string]struct {
		overrides map[string]any
		field     string
	}{
		"not base64":            {map[string]any{"content_base64": "not base64!"}, "content_base64"},
		"URL-safe base64":       {map[string]any{"content_base64": "-_-_"}, "content_base64"},
		"no padding":            {map[string]any{"content_base64": "YWI"}, "content_base64"},
		"an empty file":         {map[string]any{"content_base64": ""}, "content_base64"},
		"a word, not a type":    {map[string]any{"content_type": "pdf"}, "content_type"},
		"an unparsable type":    {map[string]any{"content_type": "application/pdf; ="}, "content_type"},
		"a blank filename":      {map[string]any{"filename": "  "}, "filename"},
		"a record without docs": {map[string]any{"entity_type": "relationship"}, "entity_type"},
		"an unknown argument":   {map[string]any{"contract_id": ids.NewV7().String()}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			docs := &fakeDocuments{}
			_, err := documentsRegistry(docs).Invoke(docsCtx(), "attach_document", attachCall(t, tc.overrides))
			var bad *BadArgsError
			if !errors.As(err, &bad) {
				t.Fatalf("err = %v, want a refusal of the arguments", err)
			}
			if bad.Field != tc.field {
				t.Errorf("the refusal names field %q, want %q: %v", bad.Field, tc.field, bad)
			}
			if len(docs.attached) != 0 {
				t.Errorf("a refused file reached the store")
			}
		})
	}
}

// Measured on the encoded text, so an oversize file is never decoded.
func TestAnOversizeFileIsRefusedByItsEncodedLengthNamingTheLimit(t *testing.T) {
	docs := &fakeDocuments{maxBytes: 3_000_000}
	huge := strings.Repeat("A", base64.StdEncoding.EncodedLen(3_000_000)+4)
	_, err := documentsRegistry(docs).Invoke(docsCtx(), "attach_document",
		attachCall(t, map[string]any{"content_base64": huge}))
	var bad *BadArgsError
	if !errors.As(err, &bad) || bad.Field != "content_base64" {
		t.Fatalf("err = %v, want content_base64 refused", err)
	}
	for _, want := range []string{"3 MB", "Margince app"} {
		if !strings.Contains(bad.Error(), want) {
			t.Errorf("the refusal %q does not say %q", bad.Error(), want)
		}
	}
	if len(docs.attached) != 0 {
		t.Error("an oversize file reached the store")
	}
}

func TestTheCapIsTheSmallerOfTheOperatorLimitAndWhatOneRequestCarries(t *testing.T) {
	for _, tc := range []struct {
		operator, want int64
	}{
		{operator: 0, want: maxInlineFileBytes},
		{operator: 50_000_000, want: maxInlineFileBytes},
		{operator: 2_000_000, want: 2_000_000},
	} {
		if got := (attachDocument{docs: &fakeDocuments{maxBytes: tc.operator}}).maxBytes(context.Background()); got != tc.want {
			t.Errorf("operator limit %d gave a cap of %d, want %d", tc.operator, got, tc.want)
		}
	}
	// The largest file the cap admits must fit the transport once encoded.
	if base64.StdEncoding.EncodedLen(maxInlineFileBytes) >= MaxMCPRequestBytes {
		t.Error("a file at the cap does not fit one MCP request")
	}
}

func TestAWrappedBase64FileIsDecodedWhole(t *testing.T) {
	docs := &fakeDocuments{}
	file := bytes.Repeat([]byte("margince "), 40)
	encoded := base64.StdEncoding.EncodeToString(file)
	wrapped := encoded[:76] + "\r\n" + encoded[76:]
	if _, err := documentsRegistry(docs).Invoke(docsCtx(), "attach_document",
		attachCall(t, map[string]any{"content_base64": wrapped})); err != nil {
		t.Fatalf("a line-wrapped file was refused: %v", err)
	}
	if !bytes.Equal(docs.attached[0].Content, file) {
		t.Error("the store was handed different bytes than were encoded")
	}
}

// The store's own refusals reach the agent as what they are, not as a fault.
func TestTheStoresRefusalsReachTheAgentAsRefusals(t *testing.T) {
	d := NewDispatcher(documentsRegistry(nil), bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
	for name, tc := range map[string]struct {
		err  error
		says string
	}{
		"an unaccepted kind":  {unacceptedKind{}, "unsupported_file_type"},
		"a hidden record":     {fmt.Errorf("company: %w", apperrors.ErrNotFound), "No such record"},
		"no update authority": {fmt.Errorf("company: %w", apperrors.ErrPermissionDenied), "Refused on authority"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := documentsRegistry(&fakeDocuments{attachErr: tc.err}).Invoke(docsCtx(), "attach_document", attachCall(t, nil))
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want the store's %v", err, tc.err)
			}
			said := d.explain("attach_document", err)
			if strings.Contains(said, internalFaultAdvice) || !strings.Contains(said, tc.says) {
				t.Errorf("the agent was told %q, want it to say %q", said, tc.says)
			}
		})
	}
}

type unacceptedKind struct{}

func (unacceptedKind) Error() string { return "text/html is not an accepted kind of file" }
func (unacceptedKind) FieldFault() (field, code, message string) {
	return "file", "unsupported_file_type", "text/html is not an accepted kind of file; send a PDF"
}

func TestListDocumentsPassesItsPageAndAnswersTheNextCursor(t *testing.T) {
	deal := ids.NewV7()
	docs := &fakeDocuments{page: DocumentPage{
		Documents:  []AttachedDocument{{AttachmentID: ids.NewV7(), Filename: "terms.pdf"}},
		NextCursor: "c-2",
	}}
	out, err := documentsRegistry(docs).Invoke(docsCtx(), "list_documents", json.RawMessage(
		`{"entity_type":"deal","entity_id":"`+deal.String()+`","cursor":"c-1","limit":10}`,
	))
	if err != nil {
		t.Fatalf("list_documents: %v", err)
	}
	if want := fmt.Sprintf(`deal/%s cursor="c-1" limit=10`, deal); len(docs.listed) != 1 || docs.listed[0] != want {
		t.Errorf("the seam was asked %v, want [%s]", docs.listed, want)
	}
	if !strings.Contains(string(out), `"next_cursor":"c-2"`) || !strings.Contains(string(out), "terms.pdf") {
		t.Errorf("the answer lost the page or its cursor: %s", out)
	}
}

func TestListDocumentsHoldsItsLimitToFifty(t *testing.T) {
	docs := &fakeDocuments{}
	_, err := documentsRegistry(docs).Invoke(docsCtx(), "list_documents", json.RawMessage(
		`{"entity_type":"deal","entity_id":"`+ids.NewV7().String()+`","limit":51}`,
	))
	var bad *BadArgsError
	if !errors.As(err, &bad) || len(docs.listed) != 0 {
		t.Fatalf("a limit of 51 answered %v and reached the seam %d times, want a refusal", err, len(docs.listed))
	}
}

func TestAnEmptyRecordListsAnEmptyArrayNotNull(t *testing.T) {
	out, err := documentsRegistry(&fakeDocuments{}).Invoke(docsCtx(), "list_documents", json.RawMessage(
		`{"entity_type":"lead","entity_id":"`+ids.NewV7().String()+`"}`,
	))
	if err != nil {
		t.Fatalf("list_documents: %v", err)
	}
	if !strings.Contains(string(out), `"documents":[]`) {
		t.Errorf("an empty record answered %s, want an empty documents array", out)
	}
}

// A file's bytes must never be persisted as a proposal's arguments.
func TestAttachDocumentCanNeverBeStaged(t *testing.T) {
	r := documentsRegistry(&fakeDocuments{})
	if r.Stageable("attach_document") {
		t.Error("attach_document is stageable, so an approval would store the file as its arguments")
	}
	if _, typed := any(attachDocument{}).(recordTypedTool); typed {
		t.Error("attach_document is record-typed, so a contract floor could stage it")
	}
}

// Were attach_document ever made confirm-first, nothing could be staged for it,
// so the refusal must say why and where the file goes instead.
func TestAnAttachThatNeedsApprovalSaysTheFileCannotWait(t *testing.T) {
	approvals := &recordingApprovals{}
	registry := NewRegistry(approvals, nil)
	RegisterDocumentTools(registry, &fakeDocuments{})

	err := registry.stageRefusedCall(context.Background(), registry.tools["attach_document"], "attach_document",
		attachCall(t, nil), "hash", apperrors.ErrRequiresApproval)

	if !errors.Is(err, apperrors.ErrRequiresApproval) || len(approvals.staged) != 0 {
		t.Fatalf("answered %v after staging %d calls, want the approval refusal and nothing staged", err, len(approvals.staged))
	}
	said := NewDispatcher(registry, bindAuthenticated, "margince-crm", "test").WithLogger(discardLog()).
		explain("attach_document", err)
	for _, want := range []string{"cannot wait for an approval", "Margince app"} {
		if !strings.Contains(said, want) {
			t.Errorf("the agent was told %q, want it to say %q", said, want)
		}
	}
	if strings.Contains(said, "list_approvals") {
		t.Errorf("the agent was told %q, which sends the user to an approval that does not exist", said)
	}
}

// A retry after a lost answer gets the first receipt, and the file is stored once.
func TestARetriedAttachReplaysTheFirstReceipt(t *testing.T) {
	docs := &fakeDocuments{}
	claims := &recordingClaims{verdict: Claim{State: ClaimFresh, Attempt: freshAttempt}}
	reader := &answeringReader{}
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}),
		WithIdempotency(claims), WithReplayReader(reader), WithVolumeCharger(newCountingCharger()))
	RegisterDocumentTools(r, docs)
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:docs", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeWrite),
	})
	call := attachCall(t, map[string]any{"idempotency_key": "attach-1"})

	first, err := r.Invoke(ctx, "attach_document", call)
	if err != nil {
		t.Fatalf("first attach: %v", err)
	}
	claims.verdict = Claim{State: ClaimReplay, Result: claims.stored, Records: claims.storedRecords}
	again, err := r.Invoke(ctx, "attach_document", call)
	if err != nil {
		t.Fatalf("the retry was refused: %v — the receipt named no record a replay can re-prove", err)
	}
	if len(docs.attached) != 1 {
		t.Errorf("the file was stored %d times, want once", len(docs.attached))
	}
	if !bytes.Equal(first, again) || reader.reads != 1 {
		t.Errorf("the replay answered %s after %d re-read(s), want the first receipt after one", again, reader.reads)
	}
}

// kindRefusal stands for the store's refusal of a file's kind, as a field fault.
type kindRefusal struct{ code string }

func (e kindRefusal) Error() string {
	return `"logo.svg" is not a kind of file that can be attached; choose one of: PDF, zip archives`
}
func (e kindRefusal) FieldFault() (field, code, message string) { return "file", e.code, e.Error() }

// A refused kind is final: told to correct its call, a model packed an SVG
// into a zip, so the refusal sends it to the user instead.
func TestAttachDocumentReportsARefusedKindAsFinal(t *testing.T) {
	docs := &fakeDocuments{attachErr: kindRefusal{code: unsupportedFileTypeCode}}
	_, err := documentsRegistry(docs).Invoke(docsCtx(), "attach_document", attachCall(t, map[string]any{
		"filename": "logo.svg", "content_type": "image/svg+xml",
	}))
	if !errors.Is(err, errFileKindRefused) {
		t.Fatalf("err = %v, want the refused-kind sentinel", err)
	}
	if _, ok := errors.AsType[apperrors.FieldFault](err); !ok {
		t.Fatalf("err = %v lost the store's field fault, which the REST door classifies", err)
	}

	got := NewDispatcher(nil, nil, "t", "0").
		WithLogger(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))).
		explain("attach_document", err)
	for _, want := range []string{"choose one of: PDF, zip archives", "Do not retry", "zip or other archive", "as a note", "Tell the user"} {
		if !strings.Contains(got, want) {
			t.Errorf("explain = %q, want it to say %q", got, want)
		}
	}
	if strings.Contains(got, "Correct the arguments") {
		t.Errorf("explain = %q, invites a change of arguments that works around the refusal", got)
	}
}

func TestAttachDocumentPassesAnotherFieldFaultThrough(t *testing.T) {
	docs := &fakeDocuments{attachErr: kindRefusal{code: "validation_error"}}
	_, err := documentsRegistry(docs).Invoke(docsCtx(), "attach_document", attachCall(t, nil))
	if err == nil || errors.Is(err, errFileKindRefused) {
		t.Fatalf("err = %v, want the store's own refusal, not the refused-kind sentinel", err)
	}
}

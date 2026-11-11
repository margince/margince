// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gdocs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func run(text string) map[string]any {
	return map[string]any{"textRun": map[string]any{"content": text}}
}

func para(elements ...map[string]any) map[string]any {
	return map[string]any{"paragraph": map[string]any{"elements": elements}}
}

func decodeDoc(t *testing.T, doc map[string]any) rawDocument {
	t.Helper()
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var raw rawDocument
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// A Doc read with its tabs carries its words in the tabs, the transcript tab
// after the notes and each child tab after its parent.
func TestTheTextOfADocIsEveryTabInReadingOrder(t *testing.T) {
	doc := decodeDoc(t, map[string]any{
		"title": "Kick-off - Notes by Gemini",
		"tabs": []any{
			map[string]any{
				"documentTab": map[string]any{"body": map[string]any{"content": []any{
					map[string]any{"sectionBreak": map[string]any{}},
					para(run("Summary\n")),
					para(run("Ada will send "), run("the offer by Friday.\n")),
				}}},
				"childTabs": []any{map[string]any{
					"documentTab": map[string]any{"body": map[string]any{"content": []any{para(run("Child tab\n"))}}},
				}},
			},
			map[string]any{"documentTab": map[string]any{"body": map[string]any{"content": []any{
				para(run("Transcript\n")),
			}}}},
		},
	})
	want := "Summary\nAda will send the offer by Friday.\nChild tab\nTranscript\n"
	if got := doc.text(); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// A Doc read without tabs has its words in the body.
func TestTheTextOfADocWithoutTabsIsItsBody(t *testing.T) {
	doc := decodeDoc(t, map[string]any{"body": map[string]any{"content": []any{para(run("Only the body\n"))}}})
	if got := doc.text(); got != "Only the body\n" {
		t.Errorf("text = %q, want the body", got)
	}
}

// A table row stays one line, and a mention of a colleague reads as their name,
// so an action item names who owns it.
func TestATableRowIsOneLineAndAMentionIsAName(t *testing.T) {
	cell := func(elements ...map[string]any) map[string]any {
		return map[string]any{"content": []any{para(elements...)}}
	}
	doc := decodeDoc(t, map[string]any{"body": map[string]any{"content": []any{
		map[string]any{"table": map[string]any{"tableRows": []any{
			map[string]any{"tableCells": []any{
				cell(map[string]any{"person": map[string]any{"personProperties": map[string]any{"name": "Ada Lovelace", "email": "ada@myco.example"}}}),
				cell(run("Send the\n"), run("offer\n")),
			}},
			map[string]any{"tableCells": []any{
				cell(map[string]any{"person": map[string]any{"personProperties": map[string]any{"email": "eli@myco.example"}}}),
				cell(map[string]any{"richLink": map[string]any{"richLinkProperties": map[string]any{"title": "Pricing sheet"}}}),
			}},
		}}},
	}}})
	want := "Ada Lovelace | Send the offer\neli@myco.example | Pricing sheet\n"
	if got := doc.text(); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// docsServer answers documents.get with one status and body for every Doc.
func docsServer(t *testing.T, status int, body map[string]any) DocsAPI {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeTabsContent") != "true" {
			t.Errorf("documents.get without its tabs: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("writing the document: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return NewDocsAPI(srv.Client(), srv.URL)
}

func googleError(status, reason string) map[string]any {
	inner := map[string]any{"code": 403, "status": status}
	if reason != "" {
		inner["details"] = []any{map[string]any{"reason": reason}}
	}
	return map[string]any{"error": inner}
}

// A refused grant, a Doc this account cannot open and a throttle are three
// different answers, because each asks something different of the sync.
func TestAReadRefusalSaysWhoseFaultItIs(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   map[string]any
		want   error
	}{
		{"scope refused", http.StatusForbidden, googleError("PERMISSION_DENIED", "ACCESS_TOKEN_SCOPE_INSUFFICIENT"), ErrScopeRefused},
		{"no access to the doc", http.StatusForbidden, googleError("PERMISSION_DENIED", ""), ErrDocumentUnreadable},
		{"doc gone", http.StatusNotFound, googleError("NOT_FOUND", ""), ErrDocumentUnreadable},
		{"token revoked", http.StatusUnauthorized, googleError("UNAUTHENTICATED", ""), connector.ErrAuthRejected},
		{"throttled", http.StatusTooManyRequests, googleError("RESOURCE_EXHAUSTED", ""), connector.ErrRateLimited},
		{"api disabled", http.StatusForbidden, googleError("PERMISSION_DENIED", "SERVICE_DISABLED"), connector.ErrAuthRejected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := docsServer(t, tc.status, tc.body).Document(context.Background(), "access-1", "doc-1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want != ErrScopeRefused && errors.Is(err, ErrScopeRefused) {
				t.Errorf("err = %v reads as a refused grant", err)
			}
		})
	}
}

func TestADocReadsItsTitleRevisionAndText(t *testing.T) {
	doc, err := docsServer(t, http.StatusOK, map[string]any{
		"title": " Kick-off - Transcript ", "revisionId": "rev-7",
		"tabs": []any{map[string]any{"documentTab": map[string]any{"body": map[string]any{"content": []any{para(run("Hello\n"))}}}}},
	}).Document(context.Background(), "access-1", "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if doc != (Document{Title: "Kick-off - Transcript", Revision: "rev-7", Text: "Hello\n"}) {
		t.Errorf("document = %+v", doc)
	}
}

// keeperFake is the Sink's meeting-notes side, recording what the sync handed it.
type keeperFake struct {
	connector.Sink
	due     []connector.MeetingNotesDoc
	kept    []string
	refused int
}

func (k *keeperFake) MeetingNotesDue(context.Context) ([]connector.MeetingNotesDoc, error) {
	return k.due, nil
}

func (k *keeperFake) KeepMeetingNotes(_ context.Context, doc connector.MeetingNotesDoc, notes connector.MeetingNotesText) error {
	k.kept = append(k.kept, doc.FileID+"="+notes.Text)
	return nil
}

func (k *keeperFake) MeetingNotesRefused(context.Context) error {
	k.refused++
	return nil
}

// docsByID answers each Doc id with its own document or error.
type docsByID map[string]struct {
	doc Document
	err error
}

func (d docsByID) Document(_ context.Context, _, fileID string) (Document, error) {
	return d[fileID].doc, d[fileID].err
}

func syncWith(t *testing.T, docs DocsAPI, keeper *keeperFake) error {
	t.Helper()
	auth := authenticate(t, ConsentScopes())
	_, err := New(fakeOAuth{}, nil).WithDocs(docs).Sync(context.Background(), auth, nil, keeper)
	return err
}

// A Doc this account cannot open is passed over and the next one is still read.
func TestASyncReadsPastADocItCannotOpen(t *testing.T) {
	keeper := &keeperFake{due: []connector.MeetingNotesDoc{
		{Meeting: ids.NewV7(), FileID: "closed"}, {Meeting: ids.NewV7(), FileID: "open"},
	}}
	docs := docsByID{
		"closed": {err: ErrDocumentUnreadable},
		"open":   {doc: Document{Text: "notes"}},
	}
	if err := syncWith(t, docs, keeper); err != nil {
		t.Fatal(err)
	}
	if len(keeper.kept) != 1 || keeper.kept[0] != "open=notes" || keeper.refused != 0 {
		t.Errorf("kept %v, refused %d; want only the open Doc", keeper.kept, keeper.refused)
	}
}

// A refused grant is recorded on the connection and stops the pass: no later
// Doc can be read with the same token.
func TestARefusedGrantIsRecordedAndStopsThePass(t *testing.T) {
	keeper := &keeperFake{due: []connector.MeetingNotesDoc{
		{Meeting: ids.NewV7(), FileID: "first"}, {Meeting: ids.NewV7(), FileID: "second"},
	}}
	docs := docsByID{
		"first":  {err: ErrScopeRefused},
		"second": {doc: Document{Text: "notes"}},
	}
	if err := syncWith(t, docs, keeper); err != nil {
		t.Fatal(err)
	}
	if keeper.refused != 1 || len(keeper.kept) != 0 {
		t.Errorf("refused %d, kept %v; want the refusal recorded and nothing read", keeper.refused, keeper.kept)
	}
}

// A failure that is not about one Doc fails the sync. The registry then backs
// off or parks the connection, as for any other Google sync.
func TestAThrottledReadFailsTheSync(t *testing.T) {
	keeper := &keeperFake{due: []connector.MeetingNotesDoc{{Meeting: ids.NewV7(), FileID: "doc"}}}
	throttled := &connector.RateLimitedError{Status: http.StatusTooManyRequests}
	if err := syncWith(t, docsByID{"doc": {err: throttled}}, keeper); !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("err = %v, want the throttle", err)
	}
	if err := syncWith(t, docsByID{"doc": {err: googleconn.ErrUnreachable}}, keeper); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("err = %v, want unreachable", err)
	}
}

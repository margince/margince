// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMailDraftHTTPKeepsEditorState(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	body := fmt.Sprintf(`{"anchor_type":"activity","anchor_id":%q,"body":"Draft words","editor_state":{"intent":"Ask for scope","ai_generated":true}}`, anchor.ID.String())
	request := httptest.NewRequest(http.MethodPut, "/v1/mail-drafts", strings.NewReader(body)).WithContext(ctx)
	response := httptest.NewRecorder()
	h := Handlers{store: draftStore(e)}
	h.SaveMailDraft(response, request, crmcontracts.SaveMailDraftParams{})
	if response.Code != http.StatusOK {
		t.Fatalf("save status %d: %s", response.Code, response.Body)
	}
	var saved crmcontracts.MailDraft
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.EditorState == nil || saved.EditorState.Intent == nil || *saved.EditorState.Intent != "Ask for scope" || saved.EditorState.AiGenerated == nil || !*saved.EditorState.AiGenerated {
		t.Fatalf("HTTP response lost editor state: %+v", saved.EditorState)
	}
	got, err := draftStore(e).GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content.EditorState.ContentFingerprint == nil {
		t.Fatal("saved state is not bound to its words")
	}
}

func TestOversizedEditorMetadataDoesNotLoseTheMessage(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	content := typedDraft()
	content.EditorState = crmcontracts.MailDraftEditorState{Intent: new(strings.Repeat("x", 450)), ServedBody: new(strings.Repeat("z", 21000))}
	saved, err := draftStore(e).SaveMailDraft(ctx, anchor, content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Content.Body != content.Body || saved.Content.EditorState.Intent == nil || len(*saved.Content.EditorState.Intent) != 400 {
		t.Fatalf("saving metadata changed the message: %+v", saved.Content)
	}
}

func TestAnOlderWriterCannotLeaveEditorStateOnDifferentWords(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := replyAnchor(e.seedAnchor(t, "", ""))
	content := typedDraft()
	content.EditorState = crmcontracts.MailDraftEditorState{Intent: new("Original purpose"), AiGenerated: new(true)}
	saved, err := draftStore(e).SaveMailDraft(ctx, anchor, content, nil)
	if err != nil {
		t.Fatal(err)
	}
	// This is the old API's update shape, which predates editor_state.
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`UPDATE mail_draft SET body=$%d WHERE id=$%d`, arg("Different human words"), arg(saved.ID))
	if _, err := e.owner.Exec(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
	got, err := draftStore(e).GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content.Body != "Different human words" || got.Content.EditorState.AiGenerated != nil || got.Content.EditorState.Intent != nil {
		t.Fatalf("an older writer left stale attribution: %+v", got.Content)
	}
}

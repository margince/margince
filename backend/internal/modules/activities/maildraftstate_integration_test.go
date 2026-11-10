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
	deal := e.seedDeal(t, "Draft context")
	body := fmt.Sprintf(`{"anchor_type":"activity","anchor_id":%q,"body":"Draft words","editor_state":{"intent":"Ask for scope","ai_generated":true,"filing":[{"anchor_type":"deal","anchor_id":%q}]}}`, anchor.ID.String(), deal.String())
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
	if saved.EditorState.Filing == nil || len(*saved.EditorState.Filing) != 1 ||
		(*saved.EditorState.Filing)[0].AnchorId != crmcontracts.Id(deal) {
		t.Fatalf("HTTP response lost the selected deal: %+v", saved.EditorState.Filing)
	}
	got, err := draftStore(e).GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content.EditorState.Filing == nil || len(*got.Content.EditorState.Filing) != 1 ||
		(*got.Content.EditorState.Filing)[0].AnchorId != crmcontracts.Id(deal) {
		t.Fatalf("saved draft lost its selected deal: %+v", got.Content.EditorState.Filing)
	}
	if got.Content.EditorState.ContentFingerprint == nil {
		t.Fatal("saved state is not bound to its words")
	}
	clearBody := fmt.Sprintf(`{"anchor_type":"activity","anchor_id":%q,"body":"Draft words","editor_state":{"filing":[]}}`, anchor.ID.String())
	request = httptest.NewRequest(http.MethodPut, "/v1/mail-drafts", strings.NewReader(clearBody)).WithContext(ctx)
	request.Header.Set("If-Match", fmt.Sprint(saved.Version))
	response = httptest.NewRecorder()
	h.SaveMailDraft(response, request, crmcontracts.SaveMailDraftParams{})
	if response.Code != http.StatusOK {
		t.Fatalf("clear status %d: %s", response.Code, response.Body)
	}
	var cleared crmcontracts.MailDraft
	if err := json.Unmarshal(response.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.EditorState == nil || cleared.EditorState.Filing == nil || len(*cleared.EditorState.Filing) != 0 {
		t.Fatalf("HTTP response lost the explicit empty filing: %+v", cleared.EditorState)
	}
	got, err = draftStore(e).GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content.EditorState.Filing == nil || len(*got.Content.EditorState.Filing) != 0 {
		t.Fatalf("saved draft lost the explicit empty filing: %+v", got.Content.EditorState.Filing)
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

// The language a rep chose for the AI draft comes back with the words, so a
// reload drafts in it again. A code the product does not write is dropped, and
// the words still save.
func TestMailDraftHTTPKeepsTheChosenDraftLanguage(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	h := Handlers{store: draftStore(e)}
	save := func(anchor MailDraftAnchor, language string) crmcontracts.MailDraft {
		t.Helper()
		body := fmt.Sprintf(`{"anchor_type":"activity","anchor_id":%q,"body":"Draft words","editor_state":{"language":%q}}`,
			anchor.ID.String(), language)
		response := httptest.NewRecorder()
		h.SaveMailDraft(response, httptest.NewRequest(http.MethodPut, "/v1/mail-drafts", strings.NewReader(body)).WithContext(ctx),
			crmcontracts.SaveMailDraftParams{})
		if response.Code != http.StatusOK {
			t.Fatalf("saving with language %q answered %d: %s", language, response.Code, response.Body)
		}
		var saved crmcontracts.MailDraft
		if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
			t.Fatal(err)
		}
		return saved
	}

	chosen := replyAnchor(e.seedAnchor(t, "", ""))
	save(chosen, "vi")
	got, err := draftStore(e).GetMailDraft(ctx, chosen)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content.EditorState.Language == nil || *got.Content.EditorState.Language != crmcontracts.MailDraftEditorStateLanguageVi {
		t.Fatalf("the saved draft lost the chosen language: %+v", got.Content.EditorState)
	}

	unknown := replyAnchor(e.seedAnchor(t, "", ""))
	answered := save(unknown, "fr")
	if answered.EditorState != nil && answered.EditorState.Language != nil {
		t.Errorf("an unknown language came back as %q, want it dropped", *answered.EditorState.Language)
	}
	got, err = draftStore(e).GetMailDraft(ctx, unknown)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content.Body != "Draft words" || got.Content.EditorState.Language != nil {
		t.Fatalf("an unknown language kept %+v with body %q, want the words saved without it", got.Content.EditorState.Language, got.Content.Body)
	}
}

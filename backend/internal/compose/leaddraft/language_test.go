// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// recordingLane answers one fixed reply and keeps the request it was sent.
type recordingLane struct {
	answer string
	sent   *model.Request
}

func (l recordingLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	*l.sent = req
	return model.Response{Text: l.answer}, nil
}

// The language the rep chose is what the model is told and what the draft
// reports, on the model path and on the floor alike.
func TestALeadDraftIsWrittenInTheChosenLanguage(t *testing.T) {
	t.Parallel()
	vietnamese := `{"subject":"Thư của bạn","body":"Chào Dung,\n\nCảm ơn anh đã viết thư cho chúng tôi.","reasoning":[]}`
	for name, tc := range map[string]struct {
		body string
		want crmcontracts.CompanyEmailDraftLanguage
	}{
		"Vietnamese chosen": {`{"language":"vi"}`, crmcontracts.CompanyEmailDraftLanguageVi},
		"nothing chosen":    {`{}`, crmcontracts.CompanyEmailDraftLanguageEn},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var sent model.Request
			modelDraft := draftLeadAs(t, recordingLane{answer: vietnamese, sent: &sent}, tc.body)
			floorDraft := draftLeadAs(t, nil, tc.body)
			for path, draft := range map[string]crmcontracts.CompanyEmailDraft{"model": modelDraft, "floor": floorDraft} {
				if draft.Language == nil || *draft.Language != tc.want {
					t.Errorf("the %s draft reports %v, want %s", path, draft.Language, tc.want)
				}
			}
			told := `"output_language":"` + string(tc.want) + `"`
			if len(sent.Messages) == 0 || !strings.Contains(sent.Messages[0].Content, told) {
				t.Errorf("the model was not told %s: %+v", told, sent.Messages)
			}
		})
	}
}

func TestALeadDraftInALanguageTheProductDoesNotWriteIsRefused(t *testing.T) {
	t.Parallel()
	rec := serveLeadDraft(nil, `{"language":"fr"}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"language"`) {
		t.Fatalf("an unknown language answered %d: %s, want 422 naming language", rec.Code, rec.Body)
	}
}

func draftLeadAs(t *testing.T, lane contactdraft.Completer, body string) crmcontracts.CompanyEmailDraft {
	t.Helper()
	rec := serveLeadDraft(lane, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("draft answered %d: %s", rec.Code, rec.Body)
	}
	var draft crmcontracts.CompanyEmailDraft
	if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decoding the draft: %v", err)
	}
	return draft
}

func serveLeadDraft(lane contactdraft.Completer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(humanCtx(), http.MethodPost, "/leads/x/draft-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	svc := leaddraft.NewService(&leadReader{lead: lead(nil)}, &correspondence{}, lane)
	leaddraft.NewHandlers(svc).DraftLeadEmail(rec, req, openapi_types.UUID(someLead.UUID))
	return rec
}

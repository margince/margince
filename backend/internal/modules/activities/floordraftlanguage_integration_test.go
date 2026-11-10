// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// With no model bound, the floor writes the reply. It still writes in the
// language the rep chose, and reports the language it wrote in.
func TestTheFloorReplyWritesInTheChosenLanguage(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := e.seedAnchor(t, "", "")
	h := e.handlers(nil)

	_, vietnamese := DeterministicEmailDraft(DraftContext{
		Language: textlang.Vietnamese, Band: convstate.BandFresh, Threaded: true, Topic: "Pricing question",
	})
	chosen := floorReply(ctx, t, h, anchor, `{"language":"vi"}`)
	if chosen.Language == nil || *chosen.Language != crmcontracts.EmailDraftLanguageVi || chosen.Body != vietnamese {
		t.Errorf("chosen vi = %v %q, want vi and the Vietnamese floor %q", chosen.Language, chosen.Body, vietnamese)
	}

	derived := floorReply(ctx, t, h, anchor, "")
	if derived.Language == nil || *derived.Language != crmcontracts.EmailDraftLanguageEn {
		t.Errorf("no choice on an English subject reported %v, want en", derived.Language)
	}
}

func floorReply(ctx context.Context, t *testing.T, h Handlers, anchor ids.ActivityID, body string) crmcontracts.EmailDraft {
	t.Helper()
	rec := httptest.NewRecorder()
	h.DraftEmail(rec, httptest.NewRequest(http.MethodPost, "/v1/activities/x/draft-email",
		strings.NewReader(body)).WithContext(ctx), crmcontracts.Id(anchor.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("draft-email answered %d: %s", rec.Code, rec.Body)
	}
	var draft crmcontracts.EmailDraft
	if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decoding the draft: %v", err)
	}
	return draft
}

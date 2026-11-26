// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A language the product does not write is refused before anything is read,
// so the handler answers it with no store behind it.
func TestADraftInALanguageTheProductDoesNotWriteIsRefused(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	Handlers{}.DraftEmail(rec, httptest.NewRequest(http.MethodPost, "/v1/activities/x/draft-email",
		strings.NewReader(`{"language":"fr"}`)), crmcontracts.Id(ids.NewV7()))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"language"`) {
		t.Fatalf("an unknown language answered %d: %s, want 422 naming language", rec.Code, rec.Body)
	}
}

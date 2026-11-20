// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A token answer, and a refusal of one, are never kept by a cache: OAuth 2.0
// asks the token endpoint for Cache-Control: no-store.
func TestATokenAnswerAndAnOAuthRefusalAreUnstored(t *testing.T) {
	issued := httptest.NewRecorder()
	writeTokenResponse(issued, IssuedPassport{Token: "mrg_example", ExpiresAt: clocktest.Now(t).Add(time.Hour)}, "refresh-example")
	refused := httptest.NewRecorder()
	oauthError(refused, http.StatusBadRequest, "invalid_grant", "the code is spent")

	for name, rec := range map[string]*httptest.ResponseRecorder{"the token answer": issued, "the refusal": refused} {
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s carried Cache-Control %q, want no-store", name, got)
		}
	}
}

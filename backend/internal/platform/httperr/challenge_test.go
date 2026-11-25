// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestEveryUnauthorizedProblemCarriesAChallenge(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	writers := map[string]func(http.ResponseWriter){
		"Unauthorized": func(w http.ResponseWriter) { Unauthorized(w, req, "missing session cookie") },
		"DetailedError": func(w http.ResponseWriter) {
			Write(w, req, &DetailedError{Status: http.StatusUnauthorized, Code: "account_locked", Detail: "locked"})
		},
	}
	for name, write := range writers {
		rec := httptest.NewRecorder()
		write(rec)
		if got := rec.Header().Get("WWW-Authenticate"); got != sessionChallenge {
			t.Errorf("%s: WWW-Authenticate = %q, want %q", name, got, sessionChallenge)
		}
	}
}

func TestAChallengeTheWriterNamedIsKept(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
	Unauthorized(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil), "invalid or missing metrics token")
	if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="metrics"` {
		t.Fatalf("WWW-Authenticate = %q, want the writer's own challenge", got)
	}
}

func TestOnlyAnUnauthorizedProblemChallenges(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, httptest.NewRequest(http.MethodGet, "/v1/deals/x", nil), apperrors.ErrPermissionDenied)
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("a 403 sent WWW-Authenticate %q", got)
	}
}

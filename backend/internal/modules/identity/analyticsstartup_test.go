// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestGetCurrentPrincipalPreparesAnalyticsOnlyWhenRequested(t *testing.T) {
	prepare := crmcontracts.GetCurrentPrincipalParamsPrepareAnalytics
	for _, tt := range []struct {
		name                     string
		authenticated, requested bool
		status, calls            int
	}{
		{"ordinary session", true, false, http.StatusOK, 0},
		{"optional read fails", true, true, http.StatusOK, 1},
		{"no session", false, true, http.StatusUnauthorized, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			h := (Handlers{}).WithAnalyticsStartup(func(ctx context.Context) (*crmcontracts.AnalyticsStartup, error) {
				calls++
				actor, ok := principal.Actor(ctx)
				if !ok || !actor.Permissions.Objects["deal"].Read {
					t.Error("preparation lost the session's authorization")
				}
				return nil, apperrors.ErrPermissionDenied
			})
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			if tt.authenticated {
				req = req.WithContext(withIdentity(t.Context(), Identity{Email: "rep@example.com", SeatType: "full", Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}}}))
			}
			params := crmcontracts.GetCurrentPrincipalParams{}
			if tt.requested {
				params.Prepare = &prepare
			}
			rec := httptest.NewRecorder()
			h.GetCurrentPrincipal(rec, req, params)
			if rec.Code != tt.status || calls != tt.calls {
				t.Fatalf("status %d, reads %d; want %d, %d", rec.Code, calls, tt.status, tt.calls)
			}
			if tt.authenticated {
				var body crmcontracts.MeResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.AnalyticsStartup != nil {
					t.Fatal("failed optional read returned startup data")
				}
				if rec.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatal("session response permits caching")
				}
			}
		})
	}
}

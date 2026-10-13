// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The handler holds no database: a request that reached the read would panic
// rather than pass, so a 403 proves the grant is taken first.
func TestListingImportRunsTakesTheImportRunReadGrant(t *testing.T) {
	h := importHandlers{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/imports", nil)
	ctx := principal.WithActor(req.Context(), principal.Principal{Type: principal.PrincipalHuman})
	h.ListImportRuns(rec, req.WithContext(ctx), crmcontracts.ListImportRunsParams{})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestTheImportRunListLimitStaysInsideTheContractBounds(t *testing.T) {
	for _, tc := range []struct {
		asked *int
		want  int
	}{{nil, 20}, {new(0), 1}, {new(7), 7}, {new(51), 50}} {
		if got := importRunListLimit(tc.asked); got != tc.want {
			t.Errorf("limit %v answered %d, want %d", tc.asked, got, tc.want)
		}
	}
}

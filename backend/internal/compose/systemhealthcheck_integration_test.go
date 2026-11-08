// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// The health page's admission is one recorded check of four grants, any of
// which opens it. A refusal names what would have let the caller in.
func TestSystemHealthAsksOneCheckOfItsFourGrants(t *testing.T) {
	ctx, recorded := auth.RecordChecksForTest(healthTestContext(nil))
	rec := httptest.NewRecorder()
	Server{}.GetSystemHealth(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/admin/system-health", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a human with no grant got %d, want 403", rec.Code)
	}
	want := "job_health.read,data_task.read,embedding_reindex.read,installation_settings.update"
	if got := recorded().Checks; !slices.Equal(got, []string{want}) {
		t.Errorf("the health page asked %v, want [%s]", got, want)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a refusal means for the caller's key, which is not one answer.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A refusal gives the key back so the caller can fix the call and retry. A
// success, or a refusal after a committed write, is never released.
//
// Asserted on the DECISION rather than on the database, because the database
// half is recordClaimOutcome and releaseClaim, each already held.
func TestARefusalReleasesTheKeyAndASuccessDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		committed bool
		release   bool
	}{
		{"a refusal", 422, false, true},
		{"a server error", 500, false, true},
		{"a success", 200, false, false},
		{"a timeout after a commit", 503, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := claimIsReleased(tc.status, tc.committed); got != tc.release {
				t.Errorf("released = %v, want %v", got, tc.release)
			}
		})
	}
}

// A claim that cannot be taken refuses a tool's call, whose caller asked for at
// most once, and still runs a REST request, whose header a client may not mean.
func TestAFailedClaimRefusesOnlyAToolsCall(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, tc := range []struct {
		name       string
		tool, runs bool
	}{{"a tool's call", true, false}, {"a REST request", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ran := false
			mux := chi.NewRouter()
			mux.With(idempotency(pool, nil, nil)).Post("/v1/lead-sources", func(http.ResponseWriter, *http.Request) { ran = true })
			ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()),
				principal.Principal{Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String()})
			if tc.tool {
				ctx = context.WithValue(ctx, admittedByTheToolKey{}, "POST /v1/lead-sources")
			}
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/lead-sources", strings.NewReader(`{}`))
			req.Header.Set(idempotencyKeyHeader, "k1")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if ran != tc.runs {
				t.Fatalf("the handler ran = %t, want %t (status %d)", ran, tc.runs, rec.Code)
			}
		})
	}
}

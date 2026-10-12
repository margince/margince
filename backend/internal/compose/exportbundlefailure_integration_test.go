// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A bundle that fails before its first byte can still be answered. The caller
// gets the opaque 500, not an empty download named like an archive. A closed
// pool fails the read without reaching any database.
func TestAnExportThatFailsBeforeItsFirstByteAnswersFiveHundred(t *testing.T) {
	pool, err := testdb.OwnPool(context.Background(), "postgres://export@127.0.0.1:1/none")
	if err != nil {
		t.Fatalf("building the pool: %v", err)
	}
	pool.Close()
	h := exportBundleHandlers{writer: NewExportWriter(pool), log: slog.New(slog.DiscardHandler)}

	user := ids.NewV7()
	w := httptest.NewRecorder()
	h.DownloadExportBundle(w, exportRequest(principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	}))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type %q, want application/problem+json", got)
	}
	if got := w.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("the 500 still names a download: %q", got)
	}
}

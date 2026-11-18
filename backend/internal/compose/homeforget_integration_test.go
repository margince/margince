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

	"github.com/margince/margince/backend/internal/compose/homecache"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A client may leave after its write commits and before the answer is sent.
// The write stands, so the writer's Home must still be forgotten.
func TestAWriterWhoLeftAfterTheCommitStillHasTheirHomeForgotten(t *testing.T) {
	e := integration.Setup(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	stamp, err := c.Stamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := homecache.Store(ctx, c, e.Rep1, homecache.Worklist, "home", stamp, nil, crmcontracts.Worklist{}); err != nil {
		t.Fatal(err)
	}

	request, leave := context.WithCancel(ctx)
	write := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		leave()
		w.WriteHeader(http.StatusNoContent)
	})
	forgetHomeOnWrite(c, slog.Default())(write).ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(request, http.MethodPost, "/v1/contacts", nil))

	if _, _, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, c, e.Rep1, homecache.Worklist, "home"); err != nil || hit {
		t.Errorf("after the write the kept answer is served = %v (err %v); a departed client left it stale", hit, err)
	}
}

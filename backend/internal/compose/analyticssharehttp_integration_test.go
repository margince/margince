// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// shareRoutes is the handler set as serverassembly.go wires it.
func (e *forecastEnv) shareRoutes() analyticsShareHandlers {
	now := func() time.Time { return time.Now().UTC() }
	return newAnalyticsShareHandlers(
		NewAnalyticsShareStore(now), forecasting.NewStore(InstallationDB(e.Pool)), now)
}

func serveShareRoute(
	ctx context.Context, handler http.HandlerFunc, method, body string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/v1/forecast/shares", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req.WithContext(ctx))
	return rec
}

func TestAnIssuedShareIsListedWithoutItsToken(t *testing.T) {
	e := setupForecast(t)
	_, seat := e.seededIssuer(t, "wire@forecast.test")
	routes := e.shareRoutes()

	created := serveShareRoute(seat, routes.CreateForecastShare, http.MethodPost,
		`{"kind":"live","target":"forecast","scope_kind":"workspace"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("issuing answered %d: %s", created.Code, created.Body)
	}
	var issued map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &issued); err != nil {
		t.Fatalf("reading the issued share: %v", err)
	}
	if token, _ := issued["token"].(string); token == "" {
		t.Fatalf("the issue response carries no token, so nothing was handed out: %s", created.Body)
	}
	if issued["kind"] != "live" || issued["scope_kind"] != "workspace" || issued["target"] != "forecast" {
		t.Errorf("the issue response does not describe the link asked for: %s", created.Body)
	}

	listed := serveShareRoute(seat, routes.ListForecastShares, http.MethodGet, "")
	if listed.Code != http.StatusOK {
		t.Fatalf("listing answered %d: %s", listed.Code, listed.Body)
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil {
		t.Fatalf("reading the list: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0]["id"] != issued["id"] {
		t.Fatalf("the list is %s, want exactly the link just issued (%v)", listed.Body, issued["id"])
	}
	if value, present := page.Data[0]["token"]; present {
		t.Errorf("the list re-disclosed the token %v; it is shown once, at issue", value)
	}
}

func TestASeatThatCannotIssueSharesIsRefusedTheListOnTheWire(t *testing.T) {
	e := setupForecast(t)
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))

	rec := serveShareRoute(reader, e.shareRoutes().ListForecastShares, http.MethodGet, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a forecast reader without create listed shares with %d, want 403", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Errorf("the refusal is %q, want a problem document", got)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
)

func assertReportingEditionTransports(ctx context.Context, t *testing.T, e *forecastEnv, edition crmcontracts.ReportingEdition) {
	t.Helper()
	doc := reportdoc.Document{Blocks: []reportdoc.Block{{Kind: reportdoc.KindStatStrip, Cells: []reportdoc.Cell{{EditionRef: &crmcontracts.ReportEditionReference{EditionId: edition.Id, Metric: "bookings_won"}}}}}}
	encoded := reportingPayload(t, doc)
	server := newServer(e.Pool, slog.New(slog.DiscardHandler), identity.NewHandlers(identity.NewService(e.Pool)), deals.NewHandlers(InstallationDB(e.Pool), DealsInstallation()))
	router := crmcontracts.HandlerFromMuxWithBaseURL(server, chi.NewRouter(), "/v1")
	request := httptest.NewRequest(http.MethodPost, "/v1/analytics/reports/render", bytes.NewReader(encoded)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("assembled HTTP renderer: %d %s", response.Code, response.Body.String())
	}
	var web struct{ Blocks []RenderedBlock }
	if err := json.Unmarshal(response.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}
	raw, err := analyticsReportComposer(e.Pool, analyticsquery.DefaultFloor)(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}
	var tool agents.ComposeAnalyticsReportResult
	if err := json.Unmarshal(raw, &tool); err != nil {
		t.Fatal(err)
	}
	if len(web.Blocks) != 1 || len(tool.Blocks) != 1 {
		t.Fatalf("rendered blocks: web=%d tool=%d", len(web.Blocks), len(tool.Blocks))
	}
	var toolBlock RenderedBlock
	if err := json.Unmarshal(tool.Blocks[0], &toolBlock); err != nil {
		t.Fatal(err)
	}
	for _, block := range []RenderedBlock{web.Blocks[0], toolBlock} {
		if len(block.Values) != 1 || block.Values[0].Value != float64(21600000) || block.Values[0].Coverage == nil {
			t.Fatalf("renderer lost the frozen value or coverage: %+v", block)
		}
	}
}

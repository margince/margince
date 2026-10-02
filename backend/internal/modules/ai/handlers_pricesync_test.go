// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestThePriceSyncRoutesRefuseAnAgentAndAnswer501Unwired(t *testing.T) {
	routes := map[string]func(Handlers, http.ResponseWriter, *http.Request){
		"GET":  func(h Handlers, w http.ResponseWriter, r *http.Request) { h.GetAiPriceSync(w, r) },
		"PUT":  func(h Handlers, w http.ResponseWriter, r *http.Request) { h.ReplaceAiPriceSync(w, r) },
		"POST": func(h Handlers, w http.ResponseWriter, r *http.Request) { h.RefreshAiModelRates(w, r) },
	}
	for method, call := range routes {
		for kind, want := range map[principal.PrincipalType]int{
			principal.PrincipalAgent: http.StatusForbidden, principal.PrincipalHuman: http.StatusNotImplemented,
		} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method, "/v1/ai/price-sync", strings.NewReader(`{"auto_sync":true}`)).
				WithContext(principalCtx(kind))
			call(Handlers{}, w, r)
			if w.Code != want {
				t.Errorf("%s as %s → %d, want %d", method, kind, w.Code, want)
			}
		}
	}
}

func TestTheLastRunTravelsWithItsReport(t *testing.T) {
	ran := time.Date(2026, 10, 2, 6, 0, 0, 0, time.UTC)
	wire := toContractPriceSync(PriceSyncState{AutoSync: true, LastRun: &LastPriceSync{
		RanAt: ran, Trigger: PriceSyncScheduled,
		Report: RateRefreshReport{Providers: []ProviderRefresh{{Provider: "gemini", Outcome: RefreshUpdated, Added: 2, Kept: 1, Models: []string{}, Unlisted: []string{}}}},
	}})
	if wire.LastRun == nil || !wire.LastRun.RanAt.Equal(ran) || string(wire.LastRun.Trigger) != "scheduled" {
		t.Fatalf("last run = %+v", wire.LastRun)
	}
	if got := wire.LastRun.Report.Providers[0]; got.Added != 2 || got.Kept != 1 {
		t.Errorf("line = %+v", got)
	}
	if never := toContractPriceSync(PriceSyncState{AutoSync: true}); never.LastRun != nil {
		t.Error("a sync that never ran reported a last run")
	}
}

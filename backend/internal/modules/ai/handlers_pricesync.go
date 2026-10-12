// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// WithPriceSync wires the engine behind the refresh route and /ai/price-sync.
func (h Handlers) WithPriceSync(sync *PriceSync) Handlers {
	h.priceSync = sync
	return h
}

// priceSyncReady admits a human to a wired engine, answering the request itself otherwise.
func (h Handlers) priceSyncReady(w http.ResponseWriter, r *http.Request) bool {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return false
	}
	if h.priceSync == nil {
		httperr.NotImplemented(w, r, "model price sync")
		return false
	}
	return true
}

// RefreshAiModelRates runs the price sync now and reports what it did per provider.
func (h Handlers) RefreshAiModelRates(w http.ResponseWriter, r *http.Request) {
	if !h.priceSyncReady(w, r) {
		return
	}
	report, err := h.priceSync.Run(r.Context(), PriceSyncManual)
	if err != nil {
		writeRateErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractRefreshReport(report))
}

// GetAiPriceSync implements (GET /ai/price-sync).
func (h Handlers) GetAiPriceSync(w http.ResponseWriter, r *http.Request) {
	if !h.priceSyncReady(w, r) {
		return
	}
	state, err := h.priceSync.State(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractPriceSync(state))
}

// ReplaceAiPriceSync implements (PUT /ai/price-sync).
func (h Handlers) ReplaceAiPriceSync(w http.ResponseWriter, r *http.Request) {
	if !h.priceSyncReady(w, r) {
		return
	}
	// A pointer, because an omitted switch must be refused rather than read as off.
	var req struct {
		AutoSync *bool `json:"auto_sync"`
	}
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.AutoSync == nil {
		httperr.Write(w, r, httperr.Validation("auto_sync", "required", "auto_sync is required"))
		return
	}
	state, err := h.priceSync.SetAutoSync(r.Context(), *req.AutoSync)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractPriceSync(state))
}

func toContractPriceSync(s PriceSyncState) crmcontracts.AiPriceSync {
	out := crmcontracts.AiPriceSync{AutoSync: s.AutoSync}
	if s.LastRun != nil {
		out.LastRun = &crmcontracts.AiPriceSyncRun{
			RanAt: s.LastRun.RanAt, Trigger: crmcontracts.AiPriceSyncRunTrigger(s.LastRun.Trigger),
			Report: toContractRefreshReport(s.LastRun.Report),
		}
	}
	return out
}

func toContractRefreshReport(report RateRefreshReport) crmcontracts.AiModelRateRefreshReport {
	out := crmcontracts.AiModelRateRefreshReport{Providers: make([]crmcontracts.AiModelRateProviderRefresh, 0, len(report.Providers))}
	for _, p := range report.Providers {
		out.Providers = append(out.Providers, crmcontracts.AiModelRateProviderRefresh{
			Provider: p.Provider, Outcome: string(p.Outcome),
			Updated: p.Updated, Unchanged: p.Unchanged, Added: p.Added, Kept: p.Kept,
			Models: p.Models, Unlisted: p.Unlisted,
		})
	}
	return out
}

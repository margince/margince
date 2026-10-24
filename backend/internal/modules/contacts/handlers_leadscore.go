// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The Explain-This-Score surface (AC-S7, ADR-0105/A156): read the
// decomposition behind a lead's score, and enter or withdraw the human
// inputs that feed it.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// explainScoreHistoryLimit bounds one page of the series. The read filters
// every entry's sources through the caller's scope, so an unbounded page
// is unbounded work as well as an unbounded response.
const explainScoreHistoryLimit = 50

// ExplainLeadScore serves GET /leads/{id}/score — the factor
// decomposition behind the lead's number.
func (h Handlers) ExplainLeadScore(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ExplainLeadScoreParams) {
	in := ExplainLeadScoreInput{Limit: explainScoreHistoryLimit}
	if params.History != nil {
		in.History = *params.History
	}
	if params.Cursor != nil {
		in.Cursor = *params.Cursor
	}
	if params.Limit != nil && *params.Limit > 0 && *params.Limit <= explainScoreHistoryLimit {
		in.Limit = *params.Limit
	}
	out, err := h.store.ExplainLeadScore(r.Context(), pathID[ids.LeadKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListLeadManualSignals serves the qualification evidence read model. It
// carries the exact band and provenance rather than asking the client to infer
// them from a score factor whose points are not a unique band key.
func (h Handlers) ListLeadManualSignals(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	data, err := h.store.ListLeadManualSignals(r.Context(), pathID[ids.LeadKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.LeadManualSignalListResponse{Data: data})
}

// SetLeadManualSignal serves PUT /leads/{id}/manual-signals, where a rep
// supplies what capture cannot fetch.
func (h Handlers) SetLeadManualSignal(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.SetLeadManualSignalRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.SetLeadManualSignal(r.Context(), pathID[ids.LeadKind](id), SetLeadManualSignalInput{
		Factor:     string(req.Factor),
		Band:       req.Band,
		SignalKind: string(req.SignalKind),
		Confidence: req.Confidence,
		Reason:     req.Reason,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ClearLeadManualSignal serves DELETE
// /leads/{id}/manual-signals/{factor}.
func (h Handlers) ClearLeadManualSignal(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, factor crmcontracts.LeadManualSignalFactor) {
	if err := h.store.ClearLeadManualSignal(r.Context(), pathID[ids.LeadKind](id), string(factor)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetLeadSignalCatalog serves GET /lead-signal-catalog: the manual scoring
// vocabulary, read from the same catalog the store
// validates against.
func (h Handlers) GetLeadSignalCatalog(w http.ResponseWriter, r *http.Request) {
	catalog := LeadSignalCatalog()
	out := crmcontracts.LeadSignalCatalog{Factors: make([]crmcontracts.LeadSignalCatalogFactor, 0, len(catalog))}
	for _, f := range catalog {
		factor := crmcontracts.LeadSignalCatalogFactor{
			Factor: crmcontracts.LeadManualSignalFactor(f.ID), Bands: make([]crmcontracts.LeadSignalBand, 0, len(f.Bands)),
		}
		if f.CompanyField != "" {
			factor.CompanyField = &f.CompanyField
		}
		for _, b := range f.Bands {
			sizes := b.CompanySizeBands
			if sizes == nil {
				sizes = []string{}
			}
			factor.Bands = append(factor.Bands, crmcontracts.LeadSignalBand{
				Id: b.ID, Points: b.Points, Min: b.Min, Max: b.Max, CompanySizeBands: sizes,
			})
		}
		out.Factors = append(out.Factors, factor)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

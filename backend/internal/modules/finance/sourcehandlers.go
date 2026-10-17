// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// The HTTP transport for managing the accounting source. Wire concerns only;
// the store owns every gate.

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SyncQueue queues one pass of this workspace's sync. The composition
// implements it over the job runner; the api role never reads a source
// in-request beyond the connect check.
type SyncQueue interface {
	QueueSync(ctx context.Context) error
}

// WithSources binds the sources the connect flow offers and the queue its
// syncs go to. Without them the source endpoints answer 501.
func (h Handlers) WithSources(sources Sources, queue SyncQueue) Handlers {
	h.store = h.store.WithSources(sources)
	h.queue = queue
	return h
}

func (h Handlers) wired(w http.ResponseWriter, r *http.Request) bool {
	if h.queue == nil {
		httperr.NotImplemented(w, r, "accounting source management")
		return false
	}
	return true
}

// GetFinanceSource implements GET /finance/source.
func (h Handlers) GetFinanceSource(w http.ResponseWriter, r *http.Request) {
	state, err := h.store.SourceState(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, state)
}

// ConnectFinanceSource implements PUT /finance/source. The first sync is
// queued after the connection commits, so a failed queue leaves a connection
// the six-hourly sweep still picks up.
func (h Handlers) ConnectFinanceSource(w http.ResponseWriter, r *http.Request) {
	if !h.wired(w, r) {
		return
	}
	var req crmcontracts.ConnectFinanceSourceRequest
	if !httperr.DecodeClosed(w, r, &req, nil) {
		return
	}
	state, err := h.store.Connect(r.Context(), req.Provider)
	if err == nil {
		err = h.queue.QueueSync(r.Context())
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, state)
}

// DisconnectFinanceSource implements DELETE /finance/source.
func (h Handlers) DisconnectFinanceSource(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Disconnect(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SyncFinanceSource implements POST /finance/source/sync.
func (h Handlers) SyncFinanceSource(w http.ResponseWriter, r *http.Request) {
	if !h.wired(w, r) {
		return
	}
	err := h.store.RequireConnected(r.Context())
	if err == nil {
		err = h.queue.QueueSync(r.Context())
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted,
		crmcontracts.RefreshAccepted{Status: crmcontracts.RefreshAcceptedStatusEnqueued})
}

// ListFinanceSourceCustomers implements GET /finance/source/customers.
func (h Handlers) ListFinanceSourceCustomers(
	w http.ResponseWriter, r *http.Request, params crmcontracts.ListFinanceSourceCustomersParams,
) {
	customers, page, err := h.store.ListSourceCustomers(r.Context(), params.Cursor, params.Limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK,
		crmcontracts.FinanceSourceCustomerListResponse{Data: customers, Page: page})
}

// LinkFinanceSourceCustomer implements
// PUT /finance/source/customers/{external_id}/link.
func (h Handlers) LinkFinanceSourceCustomer(w http.ResponseWriter, r *http.Request, externalID string) {
	var req crmcontracts.LinkFinanceSourceCustomerRequest
	if !httperr.DecodeClosed(w, r, &req, nil) {
		return
	}
	companyID, err := linkCompany(req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	customer, err := h.store.LinkCustomer(r.Context(), externalID, companyID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, customer)
}

// linkCompany is the company a link request names. An omitted id decodes to
// the zero UUID, which is refused by name rather than looked up as no row.
func linkCompany(req crmcontracts.LinkFinanceSourceCustomerRequest) (ids.CompanyID, error) {
	if err := httperr.RequireBodyID("company_id", ids.UUID(req.CompanyId)); err != nil {
		return ids.CompanyID{}, err
	}
	return ids.From[ids.CompanyKind](ids.UUID(req.CompanyId)), nil
}

// UnlinkFinanceSourceCustomer implements
// DELETE /finance/source/customers/{external_id}/link.
func (h Handlers) UnlinkFinanceSourceCustomer(w http.ResponseWriter, r *http.Request, externalID string) {
	if err := h.store.UnlinkCustomer(r.Context(), externalID); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

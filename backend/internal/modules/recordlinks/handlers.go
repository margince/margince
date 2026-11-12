// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordlinks

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// Handlers is the links' HTTP surface. Every operation is human-only: how two
// records relate is a user's call, and no agent tool exists for it.
type Handlers struct {
	store *Store
}

// NewHandlers binds the routes to the store.
func NewHandlers(store *Store) Handlers {
	return Handlers{store: store}
}

// ListLinkTypes answers every type a link may have, core's first.
func (Handlers) ListLinkTypes(w http.ResponseWriter, _ *http.Request) {
	declared := recordkinds.LinkTypes()
	out := crmcontracts.LinkTypeList{Data: make([]crmcontracts.LinkType, 0, len(declared))}
	for _, t := range declared {
		wire := crmcontracts.LinkType{Name: t.Name, Label: t.Label, From: endsToWire(t.From), To: endsToWire(t.To)}
		if t.Unit != "" {
			unit := string(t.Unit)
			wire.Unit = &unit
		}
		out.Data = append(out.Data, wire)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func endsToWire(ends []string) []crmcontracts.LinkedRecordType {
	out := make([]crmcontracts.LinkedRecordType, len(ends))
	for i, e := range ends {
		out[i] = crmcontracts.LinkedRecordType(e)
	}
	return out
}

// ListRecordLinks answers the links of one record.
func (h Handlers) ListRecordLinks(w http.ResponseWriter, r *http.Request, params crmcontracts.ListRecordLinksParams) {
	links, err := h.store.List(r.Context(), string(params.RecordType), ids.UUID(params.RecordId))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RecordLinkList{Data: links})
}

// CreateRecordLink links two records.
func (h Handlers) CreateRecordLink(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.NewRecordLink
	if !httperr.Decode(w, r, &body) {
		return
	}
	link, err := h.store.Create(r.Context(), NewLink{
		FromType: string(body.FromType), FromID: ids.UUID(body.FromId),
		ToType: string(body.ToType), ToID: ids.UUID(body.ToId), LinkType: body.LinkType,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, link)
}

// DeleteRecordLink removes a link.
func (h Handlers) DeleteRecordLink(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.Delete(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

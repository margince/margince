// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The route: one read, no verbs.
//
// Nothing is answered from here. A pending approval is decided where approvals
// are decided; an undo calls the record's own restore route. This surface
// reports, and the surfaces that own each verb keep it — a second place to
// answer a decision is a second place for the two answers to disagree.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers binds the route to the read.
type Handlers struct {
	svc *Service
	// home serves a reader's receipt over the default window while it holds;
	// nil reads every receipt fresh.
	home *homecache.Cache
}

// NewHandlers binds the route.
func NewHandlers(svc *Service) Handlers { return Handlers{svc: svc} }

// GetMagic answers what the machinery did, needs, could not finish, and is
// watching.
func (h Handlers) GetMagic(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetMagicParams,
) {
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	out, err := h.receipt(r.Context(), params.Since, limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetMagicLineRecords opens one done line: the records it stands for, what
// changed on each, and whether each change can be taken back.
func (h Handlers) GetMagicLineRecords(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID, params crmcontracts.GetMagicLineRecordsParams,
) {
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	out, err := h.svc.LineRecords(r.Context(), ids.UUID(id), params.Since, params.Cursor, limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// WithHomeCache serves a reader's receipt over the default window from c while
// it holds.
func (h Handlers) WithHomeCache(c *homecache.Cache) Handlers {
	h.home = c
	return h
}

// receipt is the read GetMagic answers. A window the caller names is a question
// about the past, read fresh; the default window is what Home opens on.
func (h Handlers) receipt(ctx context.Context, since *time.Time, limit int) (crmcontracts.MagicReceipt, error) {
	reader, cacheable := homecache.ReaderOf(ctx)
	if h.home == nil || !cacheable || since != nil {
		return h.svc.Read(ctx, since, limit)
	}
	key := fmt.Sprintf("limit=%d", limit)
	if out, hit, err := homecache.Lookup[crmcontracts.MagicReceipt](ctx, h.home, reader, homecache.Magic, key); err != nil || hit {
		return out, err
	}
	stamp, err := h.home.Stamp(ctx)
	if err != nil {
		return crmcontracts.MagicReceipt{}, err
	}
	out, err := h.svc.Read(ctx, nil, limit)
	if err != nil {
		return out, err
	}
	if err := homecache.Store(ctx, h.home, reader, homecache.Magic, key, stamp, nil, out); err != nil {
		return crmcontracts.MagicReceipt{}, err
	}
	return out, nil
}

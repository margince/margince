// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the notices transport: one verb, because a notice asks for
// nothing but to be seen. The content itself reaches the reader on the
// Worklist's notices lane, not through a listing of its own.
type Handlers struct {
	store *Store
}

// NewHandlers binds the transport to its store.
func NewHandlers(store *Store) Handlers {
	return Handlers{store: store}
}

// MarkNoticeRead settles one notice for the acting person.
func (h Handlers) MarkNoticeRead(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if err := h.store.MarkRead(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

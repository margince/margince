// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LiveListReader answers the Live Lists whose filter names one column of one
// object. Lists belong to collections, so compose injects it.
type LiveListReader func(ctx context.Context, object, column string) (crmcontracts.CustomFieldLiveLists, error)

// WithLiveLists injects the Live List reader. Without one no field is used by
// any list, which is true of an installation with lists switched off.
func (h Handlers) WithLiveLists(read LiveListReader) Handlers {
	svc := *h.svc
	svc.liveLists = read
	h.svc = &svc
	return h
}

// LiveLists answers the Live Lists that filter on a field: what retiring it
// leaves behind. It asks for the grant that retires a field, since the answer
// is for somebody about to retire one.
func (s *Service) LiveLists(ctx context.Context, id ids.UUID) (crmcontracts.CustomFieldLiveLists, error) {
	none := crmcontracts.CustomFieldLiveLists{Lists: []crmcontracts.CustomFieldLiveList{}}
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return none, err
	}
	var object, column string
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT object, column_name FROM custom_field WHERE id = $1`, id).Scan(&object, &column)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return none, apperrors.ErrNotFound
	}
	if err != nil {
		return none, fmt.Errorf("customfields: reading catalog row: %w", err)
	}
	if s.liveLists == nil {
		return none, nil
	}
	return s.liveLists(ctx, object, column)
}

// ListCustomFieldLiveLists serves GET /custom-fields/{id}/lists, asked before
// a retire is confirmed.
func (h Handlers) ListCustomFieldLiveLists(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	lists, err := h.svc.LiveLists(r.Context(), ids.UUID(id))
	if err != nil {
		writeCustomFieldErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, lists)
}

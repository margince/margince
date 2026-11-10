// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package memberhandoff joins prospect scope and member grants in one transaction.
package memberhandoff

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers applies prospect scope to the assignment transport.
type Handlers struct{ db *database.DB }

// NewHandlers keeps receiver resolution on the workspace-bound transaction.
func NewHandlers(db *database.DB) Handlers { return Handlers{db: db} }

// GetHandoffAssignee reads current versions, so a report snapshot cannot overwrite newer assignments.
func (h Handlers) GetHandoffAssignee(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var result crmcontracts.HandoffAssignee
	err := h.db.Tx(r.Context(), func(tx pgx.Tx) error {
		var err error
		result, err = contacts.HandoffAssignmentTx(r.Context(), tx, ids.UUID(id))
		if err != nil {
			return err
		}
		if string(result.Status) != contacts.HandoffSubmitted {
			return nil
		}
		result.CanReassign, err = contacts.HandoffReassignableTx(r.Context(), tx, ids.UUID(id))
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, result)
}

// ReassignHandoff retains prospect scope while resolving current receiver grants.
func (h Handlers) ReassignHandoff(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.HandoffAssigneeInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	var result crmcontracts.HandoffAssignee
	err := h.db.Tx(r.Context(), func(tx pgx.Tx) error {
		var err error
		result, err = contacts.HandoffAssignmentTx(r.Context(), tx, ids.UUID(id))
		if err != nil {
			return err
		}
		allowed, err := contacts.HandoffReassignableTx(r.Context(), tx, ids.UUID(id))
		if err != nil {
			return err
		}
		if !allowed {
			return apperrors.ErrPermissionDenied
		}
		if result.Version != body.Version {
			return apperrors.ErrVersionSkew
		}
		if string(result.Status) != contacts.HandoffSubmitted {
			return apperrors.ErrConflict
		}
		receiverID := ids.UUID(body.AssignedTo)
		if err := contacts.EnsureHandoffRecipientTx(r.Context(), tx, receiverID); err != nil {
			return err
		}
		members, err := identity.MemberPrincipalsTx(r.Context(), tx, []ids.UUID{receiverID})
		if err != nil {
			return err
		}
		receiver, ok := members[receiverID]
		if !ok {
			return apperrors.ErrInvalidArgument
		}
		if err := contacts.ReassignHandoffTx(r.Context(), tx, ids.UUID(id), body.Version, receiver); err != nil {
			return err
		}
		result, err = contacts.HandoffAssignmentTx(r.Context(), tx, ids.UUID(id))
		result.CanReassign = true
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, result)
}

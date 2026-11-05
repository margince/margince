// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type companyContactShareHandlers struct {
	db     *database.DB
	grants *identity.Service
}

func (h companyContactShareHandlers) ShareCompanyContacts(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ShareCompanyContactsParams) {
	var req crmcontracts.ShareCompanyContactsRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	result, err := ShareCompanyContacts(r.Context(), h.db, h.grants, ids.From[ids.CompanyKind](ids.UUID(id)), req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, result)
}

func ShareCompanyContacts(ctx context.Context, db *database.DB, grants *identity.Service, companyID ids.CompanyID, req crmcontracts.ShareCompanyContactsRequest) (crmcontracts.ShareCompanyContactsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, database.StatementCeiling)
	defer cancel()
	in := identity.CreateGrantInput{
		RecordType: "contact", SubjectType: string(req.SubjectType),
		SubjectID: ids.UUID(req.SubjectId), Access: string(req.Access), Reason: req.Reason, ExpiresAt: req.ExpiresAt,
	}
	out := crmcontracts.ShareCompanyContactsResult{}
	if err := grants.ValidateRecordGrantFields(ctx, in); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return out, err
	}
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, "company", companyID.UUID); err != nil {
			return err
		}
		if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
			return err
		}
		if err := identity.ValidateRecordGrantRecipientTx(ctx, tx, in); err != nil {
			return err
		}
		roster, err := contacts.LockCompanyRosterTx(ctx, tx, companyID)
		if err != nil {
			return err
		}
		for _, contactID := range roster {
			if err := auth.EnsureCanGrant(ctx, tx, "contact", contactID.UUID); err != nil {
				if errors.Is(err, apperrors.ErrPermissionDenied) {
					out.Skipped++
					continue
				}
				return err
			}
			in.RecordID = contactID.UUID
			if err := grants.CreateRecordGrantTx(ctx, tx, in); err != nil {
				return err
			}
			out.Shared++
		}
		return nil
	})
	if err != nil {
		return crmcontracts.ShareCompanyContactsResult{}, err
	}
	return out, nil
}

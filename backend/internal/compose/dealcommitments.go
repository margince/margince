// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the customer committed to that a deal waits on. The deal names its
// company and the claims name contacts, so the two halves meet here: the deal
// read decides whether the caller may see the deal, and the contacts read
// decides which of the account's commitments they may see.

import (
	"context"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealCommitmentsLimit bounds the card. It lists what the deal waits on, and
// past this many the list is a report nobody reads on a deal page.
const dealCommitmentsLimit = 25

// dealCommitmentHandlers serves GET /deals/{id}/commitments.
type dealCommitmentHandlers struct {
	pool     *pgxpool.Pool
	deals    *deals.Store
	contacts *contacts.Store
}

func newDealCommitmentHandlers(pool *pgxpool.Pool) dealCommitmentHandlers {
	db := InstallationDB(pool)
	return dealCommitmentHandlers{
		pool: pool, deals: deals.NewStore(db, DealsInstallation()), contacts: contacts.NewStore(db),
	}
}

// GetDealCommitments implements GET /deals/{id}/commitments.
func (h dealCommitmentHandlers) GetDealCommitments(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.read(r.Context(), ids.From[ids.DealKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// read answers for one deal. A deal with no company, or one whose company the
// caller may not read, has no account to read commitments from, and says so
// as an empty list that is complete only when nothing was masked.
func (h dealCommitmentHandlers) read(ctx context.Context, dealID ids.DealID) (crmcontracts.DealCommitments, error) {
	out := crmcontracts.DealCommitments{Data: []crmcontracts.DealCommitment{}, Complete: true}
	deal, err := h.deals.GetDeal(ctx, dealID, storekit.IncludeArchived)
	if err != nil {
		return out, err
	}
	// The grants the rows need are asked first, so a caller without them is
	// refused on every deal, not only on one whose company they may see.
	for _, object := range []string{"contact", "activity", "relationship"} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return out, err
		}
	}
	if deal.CompanyId == nil {
		out.Complete = deal.MaskedFields == nil || !slices.Contains(*deal.MaskedFields, "company_id")
		return out, nil
	}
	var rows []contacts.CompanyCommitment
	err = database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		// One past the card's length, to know whether more exist.
		rows, out.Complete, err = h.contacts.OpenTheirCommitmentsForCompany(
			ctx, tx, ids.UUID(*deal.CompanyId), dealCommitmentsLimit+1)
		return err
	})
	if err != nil {
		return out, err
	}
	if len(rows) > dealCommitmentsLimit {
		rows, out.HasMore = rows[:dealCommitmentsLimit], true
	}
	for _, row := range rows {
		out.Data = append(out.Data, crmcontracts.DealCommitment{
			Id:               openapi_types.UUID(row.ID),
			ContactId:        openapi_types.UUID(row.ContactID.UUID),
			ContactName:      row.ContactName,
			Body:             row.Body,
			SourceQuote:      row.SourceQuote,
			SourceActivityId: openapi_types.UUID(row.ActivityID),
			SourceKind:       row.SourceKind,
			DueAt:            row.DueAt,
			OccurredAt:       row.OccurredAt,
		})
	}
	return out, nil
}

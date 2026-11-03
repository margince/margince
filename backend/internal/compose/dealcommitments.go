// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the customer committed to that a deal waits on: the promises read out
// of messages filed under the deal. The deal read decides whether the caller
// may see the deal, and the contacts read decides which promises they may see.
// Whether a promise is late, and whether it names another price than the deal,
// is decided here once, so every client shows the same verdict.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/statedprice"
)

// dealCommitmentsLimit bounds the card. It lists what the deal waits on, and
// past this many the list is a report nobody reads on a deal page.
const dealCommitmentsLimit = 25

// dealCommitmentHandlers serves GET /deals/{id}/commitments.
type dealCommitmentHandlers struct {
	pool     *pgxpool.Pool
	deals    *deals.Store
	contacts *contacts.Store
	now      func() time.Time
}

func newDealCommitmentHandlers(pool *pgxpool.Pool, now func() time.Time) dealCommitmentHandlers {
	db := InstallationDB(pool)
	return dealCommitmentHandlers{
		pool: pool, deals: deals.NewStore(db, DealsInstallation()), contacts: contacts.NewStore(db), now: now,
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

// read answers for one deal: the promises read out of messages filed under it.
func (h dealCommitmentHandlers) read(ctx context.Context, dealID ids.DealID) (crmcontracts.DealCommitments, error) {
	out := crmcontracts.DealCommitments{Data: []crmcontracts.DealCommitment{}, Complete: true}
	deal, err := h.deals.GetDeal(ctx, dealID, storekit.IncludeArchived)
	if err != nil {
		return out, err
	}
	var rows []contacts.CompanyCommitment
	now := h.now()
	err = database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		// One past the card's length, to know whether more exist.
		rows, out.Complete, err = h.contacts.OpenTheirCommitmentsForDeal(ctx, tx, dealID.UUID, dealCommitmentsLimit+1)
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
			Id:                openapi_types.UUID(row.ID),
			ContactId:         openapi_types.UUID(row.ContactID.UUID),
			ContactName:       row.ContactName,
			Body:              row.Body,
			SourceQuote:       row.SourceQuote,
			SourceActivityId:  openapi_types.UUID(row.ActivityID),
			SourceKind:        row.SourceKind,
			DueAt:             row.DueAt,
			OccurredAt:        row.OccurredAt,
			OccurredPrecision: crmcontracts.OccurredPrecision(row.Precision),
			Overdue:           deadline.Passed(row.DueAt, now),
			DiffersFromDeal:   differingPrice(row.SourceQuote, deal),
		})
	}
	return out, nil
}

// differingPrice is the total the customer's own words state when it is not
// the deal's value. It reads the verbatim quote, never the model's summary,
// and a deal whose value the reader cannot see is compared with nothing.
func differingPrice(quote string, deal crmcontracts.Deal) *crmcontracts.StatedAmount {
	if deal.AmountMinor == nil || deal.Currency == nil {
		return nil
	}
	stated, ok := statedprice.Differing(quote, *deal.AmountMinor, *deal.Currency)
	if !ok {
		return nil
	}
	return &crmcontracts.StatedAmount{AmountMinor: stated, Currency: *deal.Currency}
}

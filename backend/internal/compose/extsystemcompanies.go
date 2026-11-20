// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of companies. It runs as the unit's service member, through
// the same company store the app reads, so the seat's object grant and row scope
// narrow it. A field the seat's mask withholds is left empty. The read gives the
// facts a unit needs to push or match a company, and no contact, note or custom
// field.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) Companies() extension.Companies { return systemCompanies{rt: r} }

type systemCompanies struct{ rt *callRuntime }

func (c systemCompanies) store() *contacts.Store {
	return contacts.NewStore(InstallationDB(c.rt.deps.pool))
}

func (c systemCompanies) Get(ctx context.Context, actAs extension.UserID, companyID string) (extension.CompanySummary, error) {
	seatCtx, seat, err := c.rt.seatRead(ctx, extension.SystemReadCompanies, actAs)
	if err != nil {
		return extension.CompanySummary{}, err
	}
	id, err := ids.ParseAs[ids.CompanyKind](companyID)
	if err != nil {
		return extension.CompanySummary{}, fmt.Errorf("%w: the company id is not a canonical UUID", extension.ErrInvalid)
	}
	if err := seat.trace("company", companyID, nil); err != nil {
		return extension.CompanySummary{}, err
	}
	var (
		got    crmcontracts.Company
		source map[ids.UUID]outsideID
	)
	err = database.WithWorkspaceTx(seatCtx, c.rt.deps.pool, func(tx pgx.Tx) error {
		var err error
		got, err = c.store().GetCompanyTx(seatCtx, tx, id, storekit.IncludeArchived, contacts.CustomColumns{})
		if err != nil {
			return err
		}
		source, err = outsideIDs(seatCtx, tx, []ids.UUID{id.UUID})
		return err
	})
	if err != nil {
		return extension.CompanySummary{}, seat.seatFault(err)
	}
	return summarizeCompany(seatCtx, got, source[id.UUID]), nil
}

func (c systemCompanies) List(ctx context.Context, actAs extension.UserID, q extension.CompanyPageQuery) (extension.Page[extension.CompanySummary], error) {
	seatCtx, seat, err := c.rt.seatRead(ctx, extension.SystemReadCompanies, actAs)
	if err != nil {
		return extension.Page[extension.CompanySummary]{}, err
	}
	limit, err := mailPageLimit(q.Limit)
	if err != nil {
		return extension.Page[extension.CompanySummary]{}, err
	}
	if err := seat.trace("company", "list", nil); err != nil {
		return extension.Page[extension.CompanySummary]{}, err
	}
	var (
		rows   []crmcontracts.Company
		next   storekit.Page
		source map[ids.UUID]outsideID
	)
	err = database.WithWorkspaceTx(seatCtx, c.rt.deps.pool, func(tx pgx.Tx) error {
		var err error
		rows, next, err = c.store().ListCompaniesTx(seatCtx, tx, contacts.ListCompaniesInput{
			Cursor: optional(q.Cursor), Limit: &limit, IncludeArchived: q.IncludeArchived,
		}, contacts.CustomColumns{})
		if err != nil {
			return err
		}
		listed := make([]ids.UUID, len(rows))
		for i, row := range rows {
			listed[i] = ids.UUID(row.Id)
		}
		source, err = outsideIDs(seatCtx, tx, listed)
		return err
	})
	if err != nil {
		return extension.Page[extension.CompanySummary]{}, seat.seatFault(err)
	}
	page := extension.Page[extension.CompanySummary]{Items: make([]extension.CompanySummary, len(rows)), NextCursor: next.NextCursor}
	for i, row := range rows {
		page.Items[i] = summarizeCompany(seatCtx, row, source[ids.UUID(row.Id)])
	}
	return page, nil
}

// summarizeCompany keeps the facts of the summary, and empties each one the
// seat's field mask withholds on a company. source is the company's id in the
// outside system, which the contract's Company does not carry.
func summarizeCompany(ctx context.Context, c crmcontracts.Company, source outsideID) extension.CompanySummary {
	out := extension.CompanySummary{
		ID: ids.UUID(c.Id).String(), Name: c.DisplayName, LegalName: derefString(c.LegalName),
		SourceSystem: source.system, SourceID: source.id, UpdatedAt: c.UpdatedAt, Archived: c.ArchivedAt != nil,
	}
	if c.Lifecycle != nil {
		out.Lifecycle = string(*c.Lifecycle)
	}
	if c.OwnerId != nil {
		out.OwnerID = extension.UserID(ids.UUID(*c.OwnerId).String())
	}
	own, bound := principal.Actor(ctx)
	if !bound {
		return extension.CompanySummary{ID: out.ID}
	}
	masked := auth.MaskedFields(own, string(recordTypeCompany), false)
	for field, text := range map[string]*string{
		"display_name": &out.Name, "legal_name": &out.LegalName, "lifecycle": &out.Lifecycle,
		"source_system": &out.SourceSystem, "source_id": &out.SourceID,
	} {
		if slices.Contains(masked, field) {
			*text = ""
		}
	}
	if slices.Contains(masked, "owner_id") {
		out.OwnerID = ""
	}
	return out
}

// outsideID is the id a company has in the system it came from.
type outsideID struct{ system, id string }

// outsideIDs reads the outside ids of companies the seat was already admitted
// to, so it widens nothing.
func outsideIDs(ctx context.Context, tx pgx.Tx, companies []ids.UUID) (map[ids.UUID]outsideID, error) {
	rows, err := tx.Query(ctx, `SELECT id, source_system, source_id FROM company WHERE id = ANY($1)`, companies)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]outsideID, len(companies))
	for rows.Next() {
		var (
			id         ids.UUID
			system, ex *string
		)
		if err := rows.Scan(&id, &system, &ex); err != nil {
			return nil, err
		}
		out[id] = outsideID{system: derefString(system), id: derefString(ex)}
	}
	return out, rows.Err()
}

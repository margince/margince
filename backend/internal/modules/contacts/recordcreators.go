// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Who created a contact or company, stamped only where a row leaves the store.
// A before-image reads the bare row, so an update pays for one creator read.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func readContact(ctx context.Context, tx pgx.Tx, id ids.ContactID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Contact, error) {
	out, err := readContactRow(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	return withContactCreator(ctx, tx, out)
}

func readCompany(ctx context.Context, tx pgx.Tx, id ids.CompanyID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Company, error) {
	out, err := readCompanyRow(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	return withCompanyCreator(ctx, tx, out)
}

func withContactCreator(ctx context.Context, tx pgx.Tx, contact crmcontracts.Contact) (crmcontracts.Contact, error) {
	one := []crmcontracts.Contact{contact}
	if err := attachContactCreators(ctx, tx, one); err != nil {
		return crmcontracts.Contact{}, err
	}
	return one[0], nil
}

func withCompanyCreator(ctx context.Context, tx pgx.Tx, company crmcontracts.Company) (crmcontracts.Company, error) {
	one := []crmcontracts.Company{company}
	if err := attachCompanyCreators(ctx, tx, one); err != nil {
		return crmcontracts.Company{}, err
	}
	return one[0], nil
}

func attachContactCreators(ctx context.Context, tx pgx.Tx, contacts []crmcontracts.Contact) error {
	return auditattribution.StampCreators(ctx, tx, contactEntity, contacts,
		func(p crmcontracts.Contact) ids.UUID { return ids.UUID(p.Id) },
		auditattribution.NoneWithheld[crmcontracts.Contact],
		func(p *crmcontracts.Contact, who *crmcontracts.PrincipalAttribution) { p.CreatedBy = who })
}

// attachCompanyCreators withholds nothing, because a row the reader may not
// see never reaches a page.
func attachCompanyCreators(ctx context.Context, tx pgx.Tx, companies []crmcontracts.Company) error {
	return auditattribution.StampCreators(ctx, tx, companyEntity, companies,
		func(o crmcontracts.Company) ids.UUID { return ids.UUID(o.Id) },
		auditattribution.NoneWithheld[crmcontracts.Company],
		func(o *crmcontracts.Company, who *crmcontracts.PrincipalAttribution) { o.CreatedBy = who })
}

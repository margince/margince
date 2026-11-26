// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// read_record's view of what a company or contact page shows beside the
// record's own row. It calls the readers behind GET /companies/{id}/profile-fields,
// /companies/{id}/facts and /contacts/{id}/profile-fields. Their RBAC, row scope
// and live checks apply to the tool as to the page.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func recordSidecars(db *database.DB) agents.RecordSidecarsFor {
	store := contacts.NewStore(db)
	contactPages := newContact360Service(db, store)
	return func(ctx context.Context, recordType datasource.EntityType, id ids.UUID) (agents.RecordSidecars, error) {
		var out agents.RecordSidecars
		var err error
		switch recordType {
		case datasource.EntityCompany:
			company := ids.From[ids.CompanyKind](id)
			if out.CompanyProfileFields, err = store.ListCompanyProfileFields(ctx, company); err == nil {
				out.CompanyFacts, err = store.ListCompanyFacts(ctx, company)
			}
		case datasource.EntityContact:
			out.ContactProfileFields, err = contactPages.ProfileFields(ctx, ids.From[ids.ContactKind](id))
		}
		return out, err
	}
}

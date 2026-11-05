// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/database/storekit/quickfindtest"
)

// The contact and company lists search through the filters their list builds,
// identifier arm included, so the plan is the one production runs.
func TestEachQuickFindReadsItsTrigramIndex(t *testing.T) {
	for _, list := range []struct {
		table   string
		filters listFilters
		index   string
		idIndex string
	}{
		{"contact", contactCommonFilters(ListContactsInput{}), "idx_contact_name_trgm", "idx_contact_email_trgm"},
		{"company", companyCommonFilters(ListCompaniesInput{}), "idx_company_name_trgm", "idx_company_domain_trgm"},
	} {
		t.Run(list.table, func(t *testing.T) {
			quickfindtest.AssertIndexed(t, list.table, list.filters.nameColumn, list.index, list.filters.identifier, list.idIndex)
		})
	}
	t.Run("lead", func(t *testing.T) {
		quickfindtest.AssertIndexed(t, "lead", leadQuickFindExpr, "idx_lead_name_trgm", storekit.LeadEmails, "idx_lead_email_trgm")
	})
}

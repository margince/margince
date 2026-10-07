// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit/quickfindtest"
)

func TestEachQuickFindReadsItsTrigramIndex(t *testing.T) {
	for _, list := range []struct{ table, nameExpr, index string }{
		{"contact", contactNameColumn, "idx_contact_name_trgm"},
		{"company", companyNameColumn, "idx_company_name_trgm"},
		{"lead", leadQuickFindExpr, "idx_lead_name_trgm"},
	} {
		t.Run(list.table, func(t *testing.T) {
			quickfindtest.AssertIndexed(t, list.table, list.nameExpr, list.index)
		})
	}
}
